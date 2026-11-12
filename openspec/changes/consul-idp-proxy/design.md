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

- signature against the IdP JWKS, cached in memory, refreshed periodically and on an unknown `kid`;
- an algorithm allowlist (default `RS256`, `ES256`), never `none`;
- `exp` and `nbf` with the leeway `proxy.idp.clockSkew` (seconds, same meaning as `kafka.oauth.clockSkew`; also set as `ClockSkewLeeway` of the auth method);
- `iss` == `proxy.idp.issuer`, and `aud` containing one of `proxy.idp.audiences`.

**IdP addresses (as in Kafka `idpWhitelist`):** `proxy.idp.issuer` is the external URL and must equal the `iss` claim. `proxy.idp.internalUrl` is optional. When it is set, the proxy and Consul use it for network calls to the IdP (`<internalUrl>/.well-known/openid-configuration` → `jwks_uri`, with the host replaced by the internal one), while `iss` is still checked against the external issuer. `proxy.idp.jwksUrl` overrides discovery entirely. `proxy.idp.caCert` gives the CA for HTTPS calls to the IdP (`JWKSCACert` of the auth method). Consul gets the resolved `JWKSURL`, not `OIDCDiscoveryURL`, because discovery in Consul checks the issuer against the discovery URL and fails when the internal and external URLs differ.

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

**Reference:** qubership-kafka has no separate proxy: AKHQ authenticates LDAP users itself (Micronaut). What this change takes from it is the declaration model of `kafka-service` values and of the AKHQ security configuration (`docs/public/security/akhq.md`):

- permission sets are declared once, by name (AKHQ `roles` / `groups`);
- external identities are mapped to them in `akhq.ldap.usersconfig`: `groups[].name` (LDAP group) → `groups[]` (AKHQ groups), and `users[].username` → `groups[]`;
- `default-group` applies to every authenticated user.

From Kafka broker OAuth it also takes `kafka.tokenRolesPath` (default `resource_access.account.roles`, where the platform IdP puts the roles), `kafka.oauth.clockSkew`, and the external/internal issuer pair of `kafka.idpWhitelist`.

**Decision:** The same model, with Consul policies as the permission sets. The values are rendered into one `ConsulACL` resource with `spec.acl.explicitName: true`:

```yaml
proxy:
  enabled: false
  replicas: 2
  idp:
    issuer: ""                 # external IdP URL, must equal the iss claim
    internalUrl: ""            # optional internal IdP URL for JWKS calls (as the internal URL of kafka.idpWhitelist)
    jwksUrl: ""                # optional, overrides discovery
    caCert: ""                 # optional CA of the IdP HTTPS endpoint
    audiences: []
    algorithms: ["RS256", "ES256"]
    clockSkew: 10              # seconds, as kafka.oauth.clockSkew
    tokenTTL: 1h               # MaxTokenTTL of the auth method
    authMethodName: platform-idp
    usernameClaim: preferred_username       # -> value.username
    groupsClaimPath: resource_access.account.roles  # -> list.groups, as kafka.tokenRolesPath
  cache:
    maxEntries: 10000
  # Consul permission sets, as AKHQ roles/groups: name -> Consul ACL rules
  roles:
    kv-reader:
      description: "Read access to KV"
      rules: |
        key_prefix "" { policy = "read" }
    catalog-admin:
      rules: |
        service_prefix "" { policy = "write" }
        node_prefix "" { policy = "read" }
  # Role for every valid IdP identity, as AKHQ default-group. Empty: identities without a mapping get 403.
  defaultRole: ""
  # IdP identities -> Consul roles, as akhq.ldap.usersconfig
  usersconfig:
    groups:
      - name: consul-readers   # a value of the groupsClaimPath claim
        roles: [kv-reader]
    users:
      - username: alice        # a value of the usernameClaim claim
        roles: [kv-reader, catalog-admin]
```

Rendering:

- each entry of `roles` produces a policy `idp_<name>` (`Rules`) and a Consul role `idp_<name>` (`policy_names: [idp_<name>]`);
- each `(group, role)` pair of `usersconfig.groups` produces a binding rule with `BindName: idp_<role>` and `Selector: "<group>" in list.groups`. A Consul binding rule binds one role, so a group with N roles gives N rules;
- each `(user, role)` pair of `usersconfig.users` produces a binding rule with `Selector: value.username == "<user>"`;
- `defaultRole` produces a binding rule with an empty selector, which matches every login;
- all rules use `AuthMethod: <authMethodName>` and `BindType: role`;
- the auth method maps `usernameClaim` in `ClaimMappings` to `username`, and maps `groupsClaimPath`, converted to a JSON pointer (`/resource_access/account/roles`), in `ListClaimMappings` to `groups`;
- the template fails when `usersconfig` or `defaultRole` references a role that is not declared in `roles`, or when a name breaks Consul naming or selector quoting;
- explicit selectors are already passed to Consul verbatim (`convertBindRuleAdapterToBindRule`), so the operator needs no change for the binding rules.

**Difference from AKHQ:** AKHQ reads LDAP groups through a manager bind. Here the groups come only from the JWT claims issued by the IdP, so the proxy has no LDAP connection and no LDAP credentials. The LDAP/AD integration stays on the IdP side.

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
- **Clock skew** between the IdP, the proxy and Consul. A configurable leeway `proxy.idp.clockSkew` (default 10s, as in Kafka).

## Migration Plan

Opt-in. Existing installations are unchanged until `proxy.enabled: true`. Enabling it adds resources and does not modify existing ones. Disabling it removes the proxy and the generated `ConsulACL` resource; the operator finalizer removes the policies, roles and binding rules it created.

## Open Questions

1. **How the IdP auth method is declared** (blocking for tasks section 5):
   - **A. Extend `ConsulACL`** with an `auth_methods` entry in `spec.acl.json` (`Name`, `Type: jwt`, `Config`: `JWKSURL`, `JWKSCACert`, `BoundIssuer`, `BoundAudiences`, `ClaimMappings`, `ListClaimMappings`, `JWTSupportedAlgs`; `MaxTokenTTL`). Create, update and delete follow the existing lifecycle and ownership. This matches the ticket wording ("ConsulACL CR for the JWT auth method"), but it needs a delta spec for `consul-acl-auth-method` and RBAC thinking: any namespace that may create `ConsulACL` could then create auth methods, which may need restricting to the operator namespace.
   - **B. Operator-managed second global method.** The operator creates and updates `proxy.idp.authMethodName` at startup from env, the same way it handles `applications-k8s-m2m`. The generated `ConsulACL` carries only policies, roles and binding rules. This is simpler and has no API change, but the method lives outside the CR lifecycle (it is not removed when the proxy is disabled unless the cleanup is handled separately). It also deviates from the ticket, which says the auth method (JWKS URL, token TTL) is set up by generating a `ConsulACL` CR. With B, acceptance criterion "A ConsulACL CR for the JWT auth method is created automatically" is met only for the roles and binding rules, so the deviation has to be agreed.
2. Should a valid JWT with no mapped role (and an empty `defaultRole`) get 403 (proposed) or 401?
3. The default of `groupsClaimPath`: `resource_access.account.roles` (the platform IdP, as `kafka.tokenRolesPath`) or `groups` (a generic OIDC IdP)? Do we need several group claims at once? The ticket names two IdPs, Infra IdP and Cloud Core IdP (one per installation): check whether they put roles and groups at the same claim path, and document the value for each if not.
4. TLS on the proxy listener: reuse the chart cert-manager integration (as for the backup daemon), or terminate at Ingress only?
5. Several trusted issuers, as in the `kafka.idpWhitelist` list? A Consul `jwt` auth method has one `BoundIssuer`, so each issuer would need its own auth method and its own set of binding rules. The proposal is one issuer in this change. Needing Infra IdP and Cloud Core IdP at the same time is this case.
6. The customer (CPREQ-9673) asks for "LDAP/OAuth". The change covers it through OAuth, on the assumption that the customer LDAP/AD is connected to the IdP and the LDAP groups arrive in JWT claims. To be confirmed with the customer; if LDAP is not connected to the IdP, that is a separate task on the IdP side, not in the proxy.
