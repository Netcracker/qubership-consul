Problem Statement
Consul provides no integration with the platform IdP (Infra IdP or Cloud Core IdP) in the open-source edition. All client access today relies on a single shared bootstrap ACL token, with no mechanism to assign scoped permissions to individual users, teams, or services. LDAP and native OIDC authentication are not available in the OSS edition — the OIDC auth method requires a Consul Enterprise license.

Without identity-based access control, operators cannot enforce least-privilege access to Consul's KV store, service catalog, or ACL management API. Any holder of the bootstrap token has unrestricted access to all Consul operations.

Regulated customers require IdP-authenticated access as a deployment prerequisite. Without it, Consul cannot be placed into production in those environments.

Current Pain Points
CPREQ-9673: Etisalat requires LDAP/OAuth authentication for Consul; the deployment is blocked until integration is available.
Business Benefits
Customer unblocking: Etisalat and similar regulated customers cannot deploy Consul without IdP integration; this CR directly unblocks those deployments.
Reduced operational risk: Scoped ACL tokens replace the shared bootstrap token, limiting the blast radius of a credential compromise to a single identity's permissions.
Product competitiveness: Aligns Consul's access-control posture with other cloud-native platform components that already integrate with the platform IdP.
Proposed Solution
Implement a Golang HTTP reverse-proxy component in qubership-consul that sits between clients and the Consul server, extracts Authorization: Bearer <JWT> from each request, maps the JWT to a Consul ACL token via an in-memory key-value store, and injects X-Consul-Token before forwarding.
Follow the role/user declaration approach established by the Kafka AKHQ LDAP proxy (qubership-kafka) for configuring role-to-policy mappings in Helm values.
On a JWT cache miss, call Consul /v1/acl/login to obtain a token and cache the result; on JWT expiry, evict the cache entry and re-validate.
Return HTTP 401 for requests arriving without a JWT or with an invalid JWT; no Consul request is forwarded.
Set up a Consul JWT auth method by generating a ConsulACL CR during deployment from Helm parameters (IdP JWKS URL, claim-to-role mappings, token TTL), reconciled automatically by the ACL Configurator operator.
Make the proxy opt-in via a Helm parameter so existing deployments without IdP integration are unaffected.
Acceptance Criteria
A client presenting a valid JWT receives a proxied Consul response with the scope permitted by the mapped role; the X-Consul-Token header is injected before the request reaches Consul.
A client presenting an expired or invalid JWT receives HTTP 401 from the proxy; no Consul request is forwarded.
A request with no Authorization header receives HTTP 401; no Consul request is forwarded.
A JWT not yet in the cache triggers a /v1/acl/login call; the resulting Consul token is cached and injected into the forwarded request.
An expired JWT is evicted from the in-memory cache; the next request with that JWT triggers fresh validation rather than injecting a stale token.
A ConsulACL CR for the JWT auth method is created automatically during helm install when proxy.enabled: true; no manual Consul API calls are required.
Deploying with proxy.enabled: false (the default) installs Consul without the proxy container; existing K8s service-account authentication via consul-k8s-auth-method is unaffected.
