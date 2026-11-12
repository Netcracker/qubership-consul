## 1. Proxy module skeleton

- [ ] 1.1 Create the Go module `consul-idp-proxy/` (`cmd/`, `internal/config`, `internal/jwt`, `internal/cache`, `internal/proxy`) with configuration from env/flags: upstream address and scheme, Consul CA path, auth method name, JWKS URL, issuer, audiences, algorithms, leeway, cache size, sweep interval, timeouts, listen address
- [ ] 1.2 Add `/healthz` (liveness) and `/readyz` (readiness requires a loaded JWKS) endpoints
- [ ] 1.3 Add structured logging that never prints the JWT or the Consul `SecretID` (hash prefixes only)
- [ ] 1.4 Add a Dockerfile (distroless, non-root) and the CI workflow that builds and publishes `ghcr.io/netcracker/qubership-consul-idp-proxy`

> Covers: Separate Deployment

---

## 2. JWT validation

- [ ] 2.1 Implement the JWKS fetcher with an in-memory key set, periodic refresh, refresh on an unknown `kid`, and keeping the last good key set on fetch failure
- [ ] 2.2 Implement validation: signature, algorithm allowlist (reject `none`), `exp`/`nbf` with leeway, `iss`, `aud`
- [ ] 2.3 Unit tests: missing header, non-Bearer, expired, not yet valid, wrong key, wrong `iss`, wrong `aud`, `alg: none`, key rotation (new `kid`)

> Covers: Reject Requests Without a JWT, Reject Invalid or Expired JWTs

---

## 3. Login and token cache

- [ ] 3.1 Implement the Consul login client (`POST /v1/acl/login`) and the logout client (`POST /v1/acl/logout`), with TLS and the Consul CA
- [ ] 3.2 Implement the cache: key `sha256(JWT)`, value `{AccessorID, SecretID, ExpiresAt = min(jwt.exp, token.ExpirationTime)}`, expiry check on lookup, LRU bound, background sweeper, best-effort logout on eviction
- [ ] 3.3 Deduplicate concurrent logins per key with `singleflight`
- [ ] 3.4 Map login errors to responses: no binding rule matched → 403, JWT rejected by Consul → 401, Consul unavailable or 5xx → 502/503; no cache entry on error
- [ ] 3.5 Unit tests: miss → one login; hit → no login; concurrent miss → one login; expired JWT entry removed and request rejected with 401; Consul token expiry → new login; LRU eviction; logout on eviction

> Covers: Login on Cache Miss, Use Cached Token on Cache Hit, Evict Expired Entries

---

## 4. Reverse proxy

- [ ] 4.1 Implement the handler on `httputil.ReverseProxy`: validate → cache/login → strip `Authorization`, `X-Consul-Token` and the `token` query parameter → set `X-Consul-Token` → forward
- [ ] 4.2 Remove the cache entry and retry once with a fresh login on an upstream 403 `ACL not found`; pass other upstream 403 responses through
- [ ] 4.3 Support blocking queries and streaming (flush interval, upstream timeout greater than the maximum Consul wait)
- [ ] 4.4 Unit tests with a fake Consul upstream: injected header, removed client tokens, path/query/body passed through, `X-Consul-Index` returned, nothing forwarded on 401/403

> Covers: Token Injection and Client Token Removal, Transparent Proxying

---

## 5. IdP auth method provisioning (blocked by design.md Open Questions, item 1)

- [ ] 5.1 Decide between option A (an `auth_methods` entry in `ConsulACL`) and option B (an operator-managed second global method), and update design.md and proposal.md
- [ ] 5.2 (A) Add the `auth_methods` adapter to `ACLConfig`, with create/update/delete in the reconciler, ownership and finalizer cleanup, restriction to allowed namespaces, unit tests, and a delta spec `specs/consul-acl-auth-method/spec.md`
- [ ] 5.3 (B) Add env/Helm parameters for the IdP method to the operator, with create/update at startup like `applications-k8s-m2m`, cleanup when the proxy is disabled, and unit tests
- [ ] 5.4 Make sure binding rules that reference a not-yet-created IdP method converge through the existing retryable-error path (test)

> Covers: Automatic Provisioning of the IdP Auth Method and Role Mappings

---

## 6. Helm chart

- [ ] 6.1 Add the `proxy` section to `values.yaml` (`enabled: false`, image, replicas, resources, affinity, tolerations, nodeSelector, priorityClassName, extraLabels, securityContext, `idp.*`, `cache.*`, `roles`, `service`, `ingress`, `disruptionBudget`, `networkPolicy`) and to `values.schema.json`
- [ ] 6.2 Add templates `idp-proxy-deployment.yaml`, `idp-proxy-service.yaml`, `idp-proxy-serviceaccount.yaml`, `idp-proxy-disruptionbudget.yaml`, the optional NetworkPolicy and Ingress/Route, all guarded by `proxy.enabled`
- [ ] 6.3 Add `idp-proxy-consulacl.yaml` (AKHQ model, design.md decision 8): `proxy.roles` → policies and roles `idp_<name>`; each `(group, role)` pair of `proxy.usersconfig.groups` → a binding rule with `"<group>" in list.groups`; each `(user, role)` pair of `proxy.usersconfig.users` → a binding rule with `value.username == "<user>"`; `proxy.defaultRole` → a binding rule with an empty selector; all with `AuthMethod: <authMethodName>`, `BindType: role`, and `explicitName: true`
- [ ] 6.4 Resolve the IdP addresses in a helper, as `kafka.idpWhitelist`: external `issuer` for `BoundIssuer`, and `jwksUrl`, or else `internalUrl`, or else `issuer` for the JWKS URL. Convert `groupsClaimPath` (`resource_access.account.roles`) to a JSON pointer for `ListClaimMappings`
- [ ] 6.5 Fail the template on a role that `usersconfig` or `defaultRole` references but `roles` does not declare, and on names or values that break Consul naming or selector quoting
- [ ] 6.6 Helm template tests: default render has no proxy resources; enabled render contains the expected Deployment/Service/ConsulACL; a group with two roles gives two binding rules; the selectors are escaped correctly; an undeclared role fails the render

- [ ] 6.7 Fail the render when `proxy.enabled: true` and `consulAclConfigurator.enabled: false` or Consul ACLs are disabled, with a test for each case

> Covers: Opt-in Deployment, Separate Deployment, Automatic Provisioning of the IdP Auth Method and Role Mappings, Deployment Preconditions

---

## 7. Integration tests

- [ ] 7.1 Use the real IdP as the Kafka ACL tests do (`OAuthLibrary`: register a client with `registrationToken`, `client_credentials` grant). Add `global.secrets.integrationTests.idp.{username,password,registrationToken}` and `IDENTITY_PROVIDER_URL` to the integration-tests secret and Deployment, and exclude the proxy tests through `tags_exclusion.py` when they are not set
- [ ] 7.2 Add a local JWT signing helper (own key pair and JWKS) for the negative cases that the real IdP cannot produce: wrong key, `alg: none`, wrong `iss`/`aud`, short `exp`
- [ ] 7.3 Robot tests: no header → 401; expired/invalid JWT → 401 and no Consul request; valid JWT → scoped response; read-only role cannot write; JWT expiry → entry evicted and request rejected; role mapping change applied after upgrade
- [ ] 7.4 Regression: with `proxy.enabled: false`, the existing ACL and auth-method tests pass unchanged

> Covers: all requirements

---

## 8. Documentation

- [ ] 8.1 New page `docs/public/idp-proxy.md`: architecture, IdP configuration, role mapping examples, client usage (`curl -H "Authorization: Bearer ..."`), response codes, limitations (IdP-side revocation, direct access to port 8500)
- [ ] 8.2 Update `docs/public/installation.md`, `docs/public/security.md` and `charts/helm/consul-service/README.md` with the `proxy.*` parameters
- [ ] 8.3 Add proxy failure cases (401 storm, JWKS unreachable, login 403) to `docs/public/troubleshooting.md`
