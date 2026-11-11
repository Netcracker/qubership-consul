# Design: Consul IdP Proxy

## Context

Consul OSS supports the `kubernetes` and `jwt` auth methods. The `oidc` auth method, which would integrate with an IdP directly, is Enterprise only. The `jwt` auth method can validate a JWT issued by any IdP that publishes a JWKS, but only a client that calls `/v1/acl/login` itself and then sends the returned `SecretID` in `X-Consul-Token` gets the scoped token. Regular API clients (curl, scripts, CI tools, other platform components) authenticate to the IdP and carry a Bearer token. They do not do the Consul login dance.

The chart already has the building blocks:

- The ACL Configurator operator reconciles `ConsulACL` resources (policies, roles, binding rules in `spec.acl.json`, `spec.acl.explicitName`, per-rule `AuthMethod`, explicit `Selector` passed to Consul verbatim).
- The operator creates the global JWT auth method `applications-k8s-m2m` at startup from env/Helm parameters. `ConsulACL` cannot declare an auth method today.
- A binding rule whose `AuthMethod` does not exist yet fails with a retryable error, and the resource is reconciled again later.

The proxy closes the gap: the client sends the IdP JWT, and the proxy logs in to Consul on the client's behalf and injects the scoped token.

---

## Goals

- Clients authenticate with an IdP JWT and get exactly the permissions of the roles mapped to their IdP identity (groups, users).
- Requests without a valid JWT never reach Consul.
- The Consul login happens once per JWT, not once per request.
- Everything is configured through Helm values. No manual Consul API calls.
- Opt-in, with no effect on existing deployments.

## Non-Goals

See proposal.md: UI SSO, LDAP bind, replacing existing auth methods, shared cache, Enterprise features.

---

## Architecture

```
 client ──Bearer JWT──▶ {fullname}-idp-proxy Service ──▶ idp-proxy Deployment (N replicas)
                                                             │  1. validate JWT (JWKS of the IdP)
                                                             │  2. cache lookup sha256(JWT)
                                                             │  3. miss → POST /v1/acl/login {AuthMethod, BearerToken}
                                                             │  4. strip client tokens, set X-Consul-Token
                                                             ▼
                                              {fullname}-server Service :8500/:8501 (unchanged)
                                                             ▲
              Consul clients, connect-inject, backup daemon, operators ── direct, as before
```

---

## Technical Decisions

### 1. Separate Deployment and Service

**Decision:** The proxy runs as its own Deployment `{fullname}-idp-proxy` with Service `{fullname}-idp-proxy` (default port 8080, or 8443 with TLS). It does not run as a sidecar in the server StatefulSet.

**Rationale:** It scales independently of the Consul servers, an upgrade does not roll the server StatefulSet, and its failure domain is separate. The server Service stays as it is, so internal clients are not affected.

**Alternative considered:** A sidecar in the server pod. Rejected: every proxy change would roll the StatefulSet, and replicas would be tied to the Raft cluster size.

### 2. Dedicated entry point, existing port untouched

**Decision:** The rule "no JWT → 401" applies only to the proxy Service. Port 8500/8501 of the server Service keeps accepting `X-Consul-Token` directly. External exposure (Ingress/Route) points at the proxy Service when `proxy.enabled: true`.

**Rationale:** Consul agents, `connect-inject`, `server-acl-init`, the backup daemon and services that use `applications-k8s-m2m` work with Consul tokens and have no IdP JWT. This also covers the acceptance criterion that K8s service-account authentication stays as it is.

**Consequence:** Least privilege for humans is enforced only when the server Service is not reachable from outside the cluster. Restricting direct access to the server Service is the operator's choice and is documented, not enforced by the chart.

### 3. Reverse proxy implementation

**Decision:** A new Go module `consul-idp-proxy/` in the repository root, based on `net/http/httputil.ReverseProxy`. The upstream is `{fullname}-server:{8500|8501}`; with TLS, the Consul CA is mounted from the existing CA secret. Streaming and blocking queries (`?index=`, `?wait=`) are passed through. Upstream timeouts are configurable, and the default must exceed the Consul maximum blocking-query wait.

### 4. JWT validation in the proxy before login

**Decision:** The proxy validates the JWT locally before it calls Consul:

- signature against the IdP JWKS (`proxy.idp.jwksUrl`), cached in memory, refreshed periodically and on an unknown `kid`;
- an algorithm allowlist (default `RS256`, `ES256`), never `none`;
- `exp` and `nbf` with a small clock-skew tolerance;
- `iss` == `proxy.idp.issuer`, and `aud` containing one of `proxy.idp.audiences`.

A validation failure returns 401 with `WWW-Authenticate: Bearer error="invalid_token"` and calls neither `/v1/acl/login` nor any other Consul endpoint.

**Rationale:** Garbage and expired tokens never load Consul, `exp` is known for cache eviction, and the 401 is deterministic. Consul validates the JWT again on login against the same JWKS, which is defence in depth.

### 5. Login and response codes

**Decision:** On a cache miss the proxy calls `POST /v1/acl/login` with `{"AuthMethod": "<idp auth method>", "BearerToken": "<JWT>"}`.

| Situation | Response |
| --- | --- |
| No `Authorization` header, or not `Bearer` | 401, not forwarded |
| JWT fails local validation (signature, `exp`, `iss`, `aud`) | 401, not forwarded |
| Consul login returns 403 because no binding rule matches the identity | 403, not forwarded |
| Consul login returns 401/403 for any other reason (JWT rejected by Consul) | 401, not forwarded |
| Consul unavailable or login 5xx | 502/503, not forwarded |
| Upstream request returns 403 (ACL denies the operation) | passed through to the client |

> **To confirm:** a 403 when the JWT is valid but no role is mapped. The ticket only requires 401 for invalid JWTs.

Concurrent requests with the same uncached JWT share one login (`singleflight`).

### 6. Token cache

**Decision:** An in-memory cache per replica:

- key `sha256(JWT)`, so the raw JWT is never stored or logged;
- value `{AccessorID, SecretID, ExpiresAt}`, where `ExpiresAt = min(jwt.exp, consulToken.ExpirationTime)`;
- a lookup whose `ExpiresAt` has passed is treated as a miss: the entry is removed and the request goes through validation again (the JWT is then rejected with 401 if it expired);
- a periodic sweeper removes expired entries;
- bounded size (`proxy.cache.maxEntries`, LRU eviction) to protect memory against a flood of distinct JWTs;
- on eviction the proxy calls `/v1/acl/logout` with the cached `SecretID` (best effort), so that unused login tokens do not pile up in Consul;
- if the upstream answers 403 `ACL not found` for a cached token (the token was revoked, for example its role was deleted), the entry is removed and the request is retried once with a fresh login.

**Rationale:** The token TTL comes from `MaxTokenTTL` of the auth method (`proxy.idp.tokenTTL`), so Consul expires tokens even if the proxy pod dies before logout. A per-replica cache means at most one login per JWT per replica, which is acceptable.

### 7. Header and query sanitizing

**Decision:** Before forwarding, the proxy removes `X-Consul-Token`, the `token` query parameter, and the `Authorization` header from the client request, and then sets `X-Consul-Token` to the cached `SecretID`.

**Rationale:** Without this, a client with a valid low-privilege JWT could pass a stolen or bootstrap token and bypass the mapping. The Consul API accepts the token in any of these three places.

### 8. Helm role declaration (AKHQ-like)

**Decision:** Roles are declared in values and rendered into one `ConsulACL` resource with `spec.acl.explicitName: true`:

```yaml
proxy:
  enabled: false
  replicas: 2
  idp:
    jwksUrl: ""             # https://idp.example.com/.well-known/jwks.json
    issuer: ""
    audiences: []
    algorithms: ["RS256", "ES256"]
    tokenTTL: 1h            # MaxTokenTTL of the auth method
    authMethodName: platform-idp
    claimMappings:          # claim -> value.<name> for binding-rule selectors
      preferred_username: username
    listClaimMappings:      # claim -> list.<name>
      groups: groups
  cache:
    maxEntries: 10000
  roles:
    - name: consul-kv-reader
      description: "Read access to KV"
      rules: |
        key_prefix "" { policy = "read" }
      groups: ["consul-readers"]
      users: ["alice"]
```

Rendering:

- each role produces a policy `idp_<name>` (`Rules`), a role `idp_<name>` (`policy_names`), and one binding rule per group (`Selector: "\"<group>\" in list.groups"`) and per user (`Selector: "value.username == \"<user>\""`), all with `AuthMethod: <authMethodName>` and `BindType: role`;
- explicit selectors are already passed to Consul verbatim (`convertBindRuleAdapterToBindRule`), so the operator needs no change for the bind rules;
- the format of `roles` must be aligned with the AKHQ LDAP proxy values of qubership-kafka before implementation (see Open Questions).

### 9. Provisioning of the IdP JWT auth method

The auth method must exist before its binding rules are reconciled. Binding rules that reference a missing method fail with a retryable error, so the order converges by itself.

The way the auth method is declared is **not decided yet**. See Open Questions, item 1.

---

## Risks / Trade-offs

- **Direct access to port 8500 bypasses the proxy.** Mitigation: document that external exposure goes through the proxy only, and offer an optional NetworkPolicy for the server Service. Not enforced by default, because internal clients need it.
- **A revoked IdP session stays valid until the JWT expires.** The cache and Consul cannot see IdP-side revocation. Mitigation: short JWT lifetime on the IdP side, and `tokenTTL` no longer than the JWT lifetime.
- **JWKS endpoint unavailable.** New JWTs cannot be validated (401), while cached tokens keep working until they expire. The last good JWKS stays in memory.
- **Login token accumulation** if logout fails. Bounded by `MaxTokenTTL`.
- **Memory.** Bounded by `cache.maxEntries`.
- **Clock skew** between the IdP, the proxy and Consul. A configurable leeway (default 30s).

## Migration Plan

Opt-in. Existing installations are unchanged until `proxy.enabled: true`. Enabling it adds resources and does not modify existing ones. Disabling it removes the proxy and the generated `ConsulACL` resource; the operator finalizer removes the policies, roles and binding rules it created.

## Open Questions

1. **How the IdP auth method is declared** (blocking for tasks section 5):
   - **A. Extend `ConsulACL`** with an `auth_methods` entry in `spec.acl.json` (`Name`, `Type: jwt`, `Config`: `JWKSURL`, `JWKSCACert`, `BoundIssuer`, `BoundAudiences`, `ClaimMappings`, `ListClaimMappings`, `JWTSupportedAlgs`; `MaxTokenTTL`). Create, update and delete follow the existing lifecycle and ownership. This matches the ticket wording ("ConsulACL CR for the JWT auth method"), but it needs a delta spec for `consul-acl-auth-method` and RBAC thinking: any namespace that may create `ConsulACL` could then create auth methods, which may need restricting to the operator namespace.
   - **B. Operator-managed second global method.** The operator creates and updates `proxy.idp.authMethodName` at startup from env, the same way it handles `applications-k8s-m2m`. The generated `ConsulACL` carries only policies, roles and binding rules. This is simpler and has no API change, but the method lives outside the CR lifecycle (it is not removed when the proxy is disabled unless the cleanup is handled separately).
2. Should a valid JWT with no mapped role get 403 (proposed) or 401?
3. The exact values format of the AKHQ LDAP proxy in qubership-kafka, to keep `proxy.roles` compatible.
4. TLS on the proxy listener: reuse the chart cert-manager integration (as for the backup daemon), or terminate at Ingress only?
5. Do the IdP JWKS endpoints need a custom CA (`proxy.idp.caCert`)?
