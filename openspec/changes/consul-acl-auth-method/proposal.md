## Why

The current Consul ACL configurator enforces a fixed naming convention for ACL policies, roles, and binding rules. This prevents reusing predefined (global) ACL resources, limits integration with existing Consul deployments, and makes it difficult to share ACL configurations across services.

In addition, there is currently no declarative mechanism for provisioning and managing Consul KV entries through Kubernetes resources. This change introduces more flexible ACL management and declarative Consul KV support while remaining fully backward compatible with existing ConsulACL resources.

## What Changes

- Allow explicit names for Consul ACL Roles instead of always generating names from the ConsulACL resource.
- Allow explicit BindName values for Consul ACL BindingRules.
- Support specifying the AuthMethod used by each BindingRule.
- Improve the ACL reconciliation lifecycle to fully support creation, update, and deletion of managed ACL resources.
- Introduce a new ConsulKV custom resource for declarative management of Consul KV entries.
- Add a ConsulKV controller responsible for provisioning, updating, and removing managed KV entries.
- Update deployment manifests, CRDs, and RBAC configuration to support the new ConsulKV resource.
- Introduce a global JWT auth method (`applications-k8s-m2m`) created by the operator at startup, together with a JWKS proxy deployment that exposes the Kubernetes API server's public signing keys to Consul servers.
- Generate binding-rule selectors from JWT claim mappings (`value.namespace`, `value.serviceaccount`) instead of the Kubernetes auth method fields (`serviceaccount.namespace`, `serviceaccount.name`).
- Revoke Consul tokens issued for a role when the role is deleted.
- Track shared ownership of explicitly named ACL policies (owner list in the policy description) and of ConsulKV keys (reference counter in the key `Flags`), so shared entities are removed only when the last owner goes away.
- Support `purgeOnDelete` and `operatorNamespace` on ConsulKV.

**BREAKING:** Yes, for the binding-rule contract of ConsulACL resources.

- The global auth method used for binding rules (`CONSUL_AUTH_METHOD_NAME`) changes from `{fullname}-k8s-auth-method` (type `kubernetes`) to `applications-k8s-m2m` (type `jwt`).
- The generated binding-rule selector changes from `serviceaccount.namespace==... and serviceaccount.name==...` to `value.namespace == ... and value.serviceaccount == ...`.
- Consequences on upgrade: binding rules are looked up per auth method, so a new rule is created under `applications-k8s-m2m` and the existing rule under the old method is **not** removed by the operator (the old method is not part of the cleanup set). Client services must log in through `applications-k8s-m2m` with a Kubernetes service-account JWT to get permissions from changes made after the upgrade. Rules under the old method have to be cleaned up manually.
- A per-rule `AuthMethod` of type `kubernetes` combined with the generated `value.*` selector does not match any login and results in a token without roles.
- Everything that is not related to the global auth method (explicit names, per-rule `AuthMethod`, ConsulKV) is additive, and existing ConsulACL resources keep the `{crName}_{crNamespace}_{name}` naming when `explicitName` is not set.

## Bug Fixes

Defects found in review of the changes above. They are fixed as bugfixes within this change and do not extend its scope (tasks marked `[bugfix]`):

- Network errors from binding-rule create/update are swallowed because of a shadowed `err` in `processBindRules`: the CR gets `Successful=True` and is not requeued (21.3).
- In `processPolicies`, `processRoles` and `processBindRules` only the error of the last entity is checked, so a network error of an earlier entity is lost when a later call succeeds (21.8).
- `purgeOnDelete` deletes by raw string prefix, so with `config/app` the keys `config/application/...` are removed too (22.4).
- A failure in a later KV batch leaves `Flags` out of sync with `status`; a key shared by two CRs can be deleted or never released (22.6).
- A duplicate key in one ConsulKV makes the whole transaction fail on every retry; the last entry should win and the others be skipped (22.5).
- `StatusHolder.GetStatus()` duplicates the message for `innerErrorHandlingItem` (21.6).
- `statusWritingEnabled` in `values.yaml` was changed unintentionally and is reverted (23.2); the CRD version annotation keys are inconsistent (23.3).

Review items that add behaviour or harden the design (JWT issuer detection and override, owner tracking for roles and binding rules, structured status, JWKS proxy availability, leader election) are tracked as regular tasks, not as bug fixes.

## Capabilities

### New Capabilities

- `consul-acl-auth-method`: Extends Consul ACL management with explicit Role names, explicit BindingRule names, configurable AuthMethods, and complete lifecycle reconciliation of managed ACL resources.

- `consul-kv`: Declarative management of Consul KV entries through Kubernetes custom resources.

### Modified Capabilities

None.

## Impact

This change affects the following components:

- **Consul ACL Operator** — extends ACL reconciliation to support explicit resource naming, configurable authentication methods, and full lifecycle management of managed ACL resources.
- **ConsulKV API** — introduces a new Kubernetes custom resource and controller for declarative Consul KV management.
- **Helm Chart and CRDs** — adds installation and lifecycle management for the ConsulKV CRD and its controller.
- **RBAC** — grants the operator permissions required to reconcile ConsulKV resources and to use leases for leader election.
- **JWKS proxy** — new `kubectl proxy` deployment/service/ServiceAccount in the Helm chart that serves `/openid/v1/jwks` and `/.well-known/openid-configuration` to Consul servers.
- **Client services and tooling** — services that authenticate through the old `-k8s-auth-method` must move to `applications-k8s-m2m`; the migration is documented in `docs/public/acl-configurator.md`. `connect-inject` login settings and `backup-daemon` restore logic were reviewed and keep the Kubernetes auth methods managed by `server-acl-init` (service mesh and component logins).
- **Deployment Workflows** — enables applications to provision Consul ACL resources and Consul KV entries declaratively through Kubernetes manifests while remaining compatible with existing deployments.

The implementation relies only on existing Kubernetes and Consul APIs and does not introduce additional external dependencies.
