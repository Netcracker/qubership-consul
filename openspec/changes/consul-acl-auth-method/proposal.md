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

**BREAKING:** None. All changes are additive. Existing ConsulACL resources continue to behave as before when explicit names are not specified.

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
- **RBAC** — grants the operator permissions required to reconcile ConsulKV resources.
- **Deployment Workflows** — enables applications to provision Consul ACL resources and Consul KV entries declaratively through Kubernetes manifests while remaining compatible with existing deployments.

The implementation relies only on existing Kubernetes and Consul APIs and does not introduce additional external dependencies.
