## Why

Consul OSS has no integration with the platform IdP (Infra IdP or Cloud Core IdP). LDAP and native OIDC authentication are not available in the OSS edition: the OIDC auth method requires a Consul Enterprise license. All client access today relies on a single shared bootstrap ACL token, with no way to give scoped permissions to individual users, teams or services. Any holder of the bootstrap token has unrestricted access to KV, the service catalog and the ACL API.

Regulated customers require IdP-authenticated access before Consul can go to production (CPREQ-9673: Etisalat requires LDAP/OAuth authentication for Consul; the deployment is blocked until the integration is available).

## What Changes

- Add a new component, **Consul IdP proxy**: a Go HTTP reverse proxy deployed as a separate Kubernetes Deployment and Service in front of the Consul HTTP API.
  - It takes `Authorization: Bearer <JWT>` from every request, validates the JWT against the IdP JWKS, and exchanges it for a Consul ACL token through `/v1/acl/login` on the IdP JWT auth method.
  - It keeps the JWT → Consul token mapping in an in-memory cache and evicts an entry when the JWT or the Consul token expires.
  - It injects `X-Consul-Token` before forwarding and drops any token sent by the client.
  - It returns HTTP 401 for requests without a JWT or with an invalid or expired JWT. Such requests are never forwarded to Consul.
- Add the Helm section `proxy` (opt-in, `proxy.enabled: false` by default).
  - IdP settings in the style of Kafka OAuth: external issuer and optional internal URL (as `kafka.idpWhitelist`), audiences, `clockSkew`, the claim path of the roles (as `kafka.tokenRolesPath`), and the token TTL.
  - A role declaration that follows the AKHQ model of qubership-kafka: named `roles` with Consul ACL rules, `usersconfig.groups` / `usersconfig.users` that map IdP groups and users to these roles (as `akhq.ldap.usersconfig`), and `defaultRole` (as AKHQ `default-group`).
- Provision a Consul JWT auth method for the IdP during `helm install`/`helm upgrade`, together with the policies, roles and binding rules generated from the Helm role mappings, as a `ConsulACL` resource reconciled by the ACL Configurator operator. No manual Consul API calls are needed. How the auth method itself is declared is an open question (see design.md, Open Questions).
- Build and publish the proxy image in CI, and document the feature.

The existing Consul server Service and port 8500/8501 are unchanged. Consul clients, `connect-inject`, `server-acl-init`, the backup daemon and services that log in through `applications-k8s-m2m` or the Kubernetes auth methods keep working as before.

**BREAKING:** No. The feature is opt-in. With `proxy.enabled: false` the rendered chart is the same as before.

## Capabilities

### New Capabilities

- `consul-idp-proxy`: IdP JWT authentication in front of the Consul HTTP API. Covers the reverse proxy behaviour (401 handling, login, token cache, token injection), the opt-in Helm deployment, and the automatic provisioning of the IdP auth method and the role mappings.

### Modified Capabilities

- `consul-acl-auth-method`: possibly modified, depending on how the open question about the IdP auth method is resolved (option A in design.md adds auth-method declaration to `ConsulACL`). There is no delta spec until the question is decided.

## Non-goals

- Browser SSO for the Consul UI (redirect to the IdP, session cookies). The proxy serves API clients that send a Bearer token.
- LDAP bind authentication. The IdP is the LDAP/OAuth integration point; the proxy accepts only JWTs issued by it.
- Replacing the bootstrap token or the existing auth methods (`applications-k8s-m2m`, `{fullname}-k8s-auth-method`).
- Shared or persistent token cache across proxy replicas.
- Consul Enterprise features (namespaces, admin partitions, OIDC auth method).

## Impact

- **New component** `consul-idp-proxy/`: Go module, Dockerfile, unit tests, CI image build.
- **Helm chart `consul-service`**: new `proxy` values section; Deployment, Service, ServiceAccount, PodDisruptionBudget, optional NetworkPolicy and Ingress/Route for the proxy; the generated `ConsulACL` resource; `values.schema.json`.
- **ACL Configurator operator**: reconciles the generated `ConsulACL` resource. Option A of the open question also extends the `ConsulACL` API and the operator.
- **Consul ACL state**: a new JWT auth method for the IdP, plus the policies, roles and binding rules for the declared roles.
- **Documentation**: `docs/public/` (installation, security, a new IdP proxy page) and `charts/helm/consul-service/README.md`.
- **Integration tests**: robot tests with a mock IdP (JWKS + signed JWTs).