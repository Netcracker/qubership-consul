# Specification: consul-idp-proxy

## Overview

Adds IdP-based authentication in front of the Consul HTTP API for Consul OSS. A reverse proxy validates the IdP JWT of each request, exchanges it for a Consul ACL token through the IdP JWT auth method, caches the token, and injects it as `X-Consul-Token`. The proxy is opt-in and deployed separately from the Consul servers.

---

## Terminology

- **IdP proxy** — the `{fullname}-idp-proxy` Deployment and Service.
- **IdP JWT** — a JWT issued by the platform IdP and sent as `Authorization: Bearer <JWT>`.
- **IdP auth method** — the Consul auth method of type `jwt` named by `proxy.idp.authMethodName`, which validates IdP JWTs.
- **token cache** — the in-memory map in each proxy replica from `sha256(IdP JWT)` to the Consul token obtained by login.
- **cache entry expiry** — `min(exp of the JWT, ExpirationTime of the Consul token)`.

---

## ADDED Requirements

### Requirement: Reject Requests Without a JWT

The IdP proxy SHALL respond with HTTP 401 to any request that has no `Authorization` header or whose `Authorization` header does not use the `Bearer` scheme, and SHALL NOT send any request to Consul for it.

#### Scenario: No Authorization header

- **WHEN** a client sends `GET /v1/kv/app/config` to the IdP proxy without an `Authorization` header
- **THEN** the proxy SHALL respond with HTTP 401 and a `WWW-Authenticate: Bearer` header
- **AND** no request SHALL be sent to Consul

#### Scenario: Non-Bearer scheme

- **WHEN** a client sends a request with `Authorization: Basic dXNlcjpwYXNz`
- **THEN** the proxy SHALL respond with HTTP 401 and SHALL NOT send any request to Consul

---

### Requirement: Reject Invalid or Expired JWTs

The IdP proxy SHALL validate the IdP JWT before any call to Consul: the signature against the IdP JWKS, the signing algorithm against the configured allowlist, `exp` and `nbf` (with the configured clock-skew leeway), `iss` against `proxy.idp.issuer`, and `aud` against `proxy.idp.audiences`. When validation fails, the proxy SHALL respond with HTTP 401 and SHALL NOT call `/v1/acl/login` or forward the request.

#### Scenario: Expired JWT

- **WHEN** a client sends a request with a correctly signed JWT whose `exp` is in the past beyond the leeway
- **THEN** the proxy SHALL respond with HTTP 401 with `WWW-Authenticate: Bearer error="invalid_token"`
- **AND** no request SHALL be sent to Consul

#### Scenario: Wrong signature

- **WHEN** a client sends a JWT signed by a key that is not in the IdP JWKS
- **THEN** the proxy SHALL respond with HTTP 401 and SHALL NOT send any request to Consul

#### Scenario: Wrong issuer or audience

- **WHEN** a client sends a correctly signed, unexpired JWT whose `iss` differs from `proxy.idp.issuer`, or whose `aud` contains none of `proxy.idp.audiences`
- **THEN** the proxy SHALL respond with HTTP 401 and SHALL NOT send any request to Consul

#### Scenario: Unsupported algorithm

- **WHEN** a client sends a JWT with `alg: none`, or with an algorithm that is not in `proxy.idp.algorithms`
- **THEN** the proxy SHALL respond with HTTP 401 and SHALL NOT send any request to Consul

---

### Requirement: Login on Cache Miss

When a valid IdP JWT has no entry in the token cache, the IdP proxy SHALL call Consul `POST /v1/acl/login` with the IdP auth method and the JWT, SHALL store the returned token in the token cache, and SHALL forward the request with that token. Concurrent requests with the same uncached JWT SHALL result in a single login call.

#### Scenario: First request with a valid JWT

- **WHEN** a client sends its first request with a valid IdP JWT that maps to a Consul role
- **THEN** the proxy SHALL call `/v1/acl/login` once with `AuthMethod` set to `proxy.idp.authMethodName`
- **AND** the proxy SHALL store the returned `SecretID` in the token cache under `sha256(JWT)`
- **AND** the proxy SHALL forward the request to Consul with `X-Consul-Token` set to that `SecretID`

#### Scenario: Concurrent first requests

- **WHEN** ten requests with the same uncached valid JWT arrive at one replica at the same time
- **THEN** the proxy SHALL call `/v1/acl/login` exactly once and SHALL forward all ten requests with the same token

#### Scenario: Valid JWT without a mapped role

- **WHEN** a valid IdP JWT does not match any binding rule of the IdP auth method, and Consul login is denied
- **THEN** the proxy SHALL respond with HTTP 403, SHALL NOT forward the request, and SHALL NOT create a cache entry

#### Scenario: Consul unavailable during login

- **WHEN** the login call fails because Consul is unreachable or returns a 5xx status
- **THEN** the proxy SHALL respond with HTTP 502 or 503 and SHALL NOT create a cache entry

---

### Requirement: Use Cached Token on Cache Hit

When a valid IdP JWT has an unexpired entry in the token cache, the IdP proxy SHALL forward the request with the cached token and SHALL NOT call `/v1/acl/login`.

#### Scenario: Repeated request

- **WHEN** a client sends a second request with the same valid JWT before the cache entry expiry
- **THEN** the proxy SHALL forward the request with the cached `X-Consul-Token`
- **AND** the proxy SHALL NOT call `/v1/acl/login`

#### Scenario: Cached token revoked in Consul

- **WHEN** a forwarded request with a cached token gets HTTP 403 `ACL not found` from Consul
- **THEN** the proxy SHALL remove the cache entry, log in again once, and retry the request with the new token

---

### Requirement: Evict Expired Entries

The IdP proxy SHALL treat a cache entry whose expiry has passed as absent: it SHALL remove the entry, and SHALL validate the JWT again before any further use. It SHALL never inject a token from an expired entry. The proxy SHALL also remove expired entries in the background, and SHALL call `/v1/acl/logout` for removed tokens on a best-effort basis.

#### Scenario: JWT expired after it was cached

- **WHEN** a JWT was cached, its `exp` has passed, and the client sends a request with the same JWT
- **THEN** the proxy SHALL remove the cache entry
- **AND** the proxy SHALL respond with HTTP 401, because validation fails on `exp`
- **AND** the proxy SHALL NOT forward the request with the previously cached token

#### Scenario: Consul token expired before the JWT

- **WHEN** the Consul token of a cache entry reached its `ExpirationTime` while the JWT is still valid
- **THEN** the next request with that JWT SHALL trigger a new `/v1/acl/login` call, and the new token SHALL be cached and injected

#### Scenario: Background cleanup

- **WHEN** a cache entry expires and no request uses it
- **THEN** the proxy SHALL remove it within one sweep interval and SHALL attempt `/v1/acl/logout` with its `SecretID`

#### Scenario: Cache size limit

- **WHEN** the token cache holds `proxy.cache.maxEntries` entries and a new JWT is logged in
- **THEN** the proxy SHALL evict the least recently used entry before inserting the new one

---

### Requirement: Token Injection and Client Token Removal

Before forwarding, the IdP proxy SHALL remove the `X-Consul-Token` header, the `token` query parameter and the `Authorization` header of the client request, and SHALL set `X-Consul-Token` to the token obtained for the IdP JWT. The IdP JWT and the Consul `SecretID` SHALL NOT be written to logs.

#### Scenario: Client sends its own Consul token

- **WHEN** a client sends a valid IdP JWT together with `X-Consul-Token: <bootstrap token>` or `?token=<bootstrap token>`
- **THEN** the request forwarded to Consul SHALL carry only the token obtained for the JWT
- **AND** the operation SHALL be authorized with the permissions of the mapped role only

#### Scenario: Scoped permissions apply

- **WHEN** a JWT mapped to a role with `key_prefix "" { policy = "read" }` is used for `PUT /v1/kv/app/config`
- **THEN** Consul SHALL deny the write, and the proxy SHALL pass the Consul 403 response through to the client

---

### Requirement: Transparent Proxying

For authorized requests, the IdP proxy SHALL forward the method, path, query (without `token`), body and the remaining headers to the Consul HTTP API unchanged, and SHALL return the Consul response status, headers and body to the client, including blocking queries and streamed responses.

#### Scenario: Blocking query

- **WHEN** a client sends `GET /v1/kv/app?index=42&wait=5m` with a valid JWT
- **THEN** the proxy SHALL keep the upstream request open until Consul responds or the wait elapses, and SHALL return the `X-Consul-Index` header to the client

---

### Requirement: Automatic Provisioning of the IdP Auth Method and Role Mappings

When `proxy.enabled: true`, the Helm chart SHALL provision, without manual Consul API calls, the IdP auth method from `proxy.idp` (JWKS URL, issuer, audiences, algorithms, claim mappings, `tokenTTL` as `MaxTokenTTL`), and a `ConsulACL` resource reconciled by the ACL Configurator operator. For each entry of `proxy.roles`, the resource SHALL contain a policy with the entry rules, a role referencing that policy, and one binding rule under the IdP auth method per listed group and per listed user.

> **Open:** the mechanism that creates the auth method itself (a `ConsulACL` entry or an operator-managed global method) is decided in design.md, Open Questions, item 1.

#### Scenario: Install with the proxy enabled

- **WHEN** the chart is installed with `proxy.enabled: true` and `proxy.roles` contains `consul-kv-reader` with `groups: ["consul-readers"]` and `users: ["alice"]`
- **THEN** after reconciliation Consul SHALL contain the IdP auth method of type `jwt`, the policy and role for `consul-kv-reader`, a binding rule with selector `"consul-readers" in list.groups`, and a binding rule with selector `value.username == "alice"`, both with `BindType: role`

#### Scenario: Role mapping changed on upgrade

- **WHEN** a group is removed from a role in `proxy.roles` and `helm upgrade` is run
- **THEN** the operator SHALL delete the binding rule of that group, and new logins of members of that group only SHALL NOT get the role

#### Scenario: Proxy disabled after being enabled

- **WHEN** the chart is upgraded from `proxy.enabled: true` to `proxy.enabled: false`
- **THEN** the proxy Deployment, the Service and the generated `ConsulACL` resource SHALL be removed, and the operator SHALL remove the policies, roles and binding rules created for `proxy.roles`

---

### Requirement: Opt-in Deployment

The Helm chart SHALL deploy the IdP proxy only when `proxy.enabled: true`. The default SHALL be `false`. When disabled, the rendered chart SHALL contain no proxy resources and no generated `ConsulACL` resource, and the existing auth methods and the Consul server Service SHALL be unaffected.

#### Scenario: Default install

- **WHEN** the chart is installed without setting `proxy.enabled`
- **THEN** no `{fullname}-idp-proxy` Deployment, Service or generated `ConsulACL` resource SHALL be created
- **AND** service-account login through `applications-k8s-m2m` and `{fullname}-k8s-auth-method` SHALL work as before

#### Scenario: Proxy enabled, internal clients

- **WHEN** the chart is installed with `proxy.enabled: true`
- **THEN** the `{fullname}-server` Service SHALL keep its ports and SHALL keep accepting requests with `X-Consul-Token`
- **AND** Consul clients, `connect-inject`, the backup daemon and the ACL Configurator SHALL keep connecting to it directly

---

### Requirement: Separate Deployment

The IdP proxy SHALL run as a Deployment `{fullname}-idp-proxy` with its own Service, ServiceAccount, configurable replicas, resources, affinity, tolerations, node selector and priority class, and a PodDisruptionBudget when replicas > 1. It SHALL expose liveness and readiness endpoints. Readiness SHALL require a loaded JWKS.

#### Scenario: Readiness without JWKS

- **WHEN** the proxy starts and the IdP JWKS URL is unreachable
- **THEN** the pod SHALL NOT become ready until the JWKS is loaded

#### Scenario: Rolling update of the proxy

- **WHEN** the proxy image is changed with `helm upgrade`
- **THEN** only the proxy Deployment SHALL be rolled, and the Consul server StatefulSet SHALL NOT be restarted