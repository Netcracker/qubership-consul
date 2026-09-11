# Design: Consul ACL Auth Method & ConsulKV

## Context

The Consul ACL configurator operator manages Consul ACL policies, roles, and binding rules through the `ConsulACL` custom resource. The spec embeds ACL configuration as a JSON blob (`spec.acl.json`) parsed into `ACLConfig` at reconcile time. The reconciler (`ConsulACLReconciler`) follows a standard controller-runtime pattern: add finalizer on creation, call `applyACL` on every generation change, call `deleteACL` on deletion, and persist per-entity status strings via `UpdateStatusWithRetry`.

Two structural limitations exist today:

1. **Naming is unconditional.** Every entity name is built by `convertEntityName(name, crName, crNamespace)` → `{crName}_{crNamespace}_{name}`. There is no way to reference a pre-existing Consul role or binding-rule bind target by its exact name.
2. **Binding rules lack idempotent lifecycle management.** `processBindRules` always calls `BindingRuleCreate`; there is no lookup-by-name before create/update, and per-rule auth method override is absent (`authMethod` is a process-global variable read from `CONSUL_AUTH_METHOD_NAME`).

Additionally, there is no declarative path for Consul KV entries — teams currently manage them through sidecar scripts or init containers.

---

## Goals

- Allow roles to carry an optional explicit name that bypasses the `{crName}_{crNamespace}_` prefix.
- Allow binding rules to carry an optional explicit bind name and an optional per-rule auth method override.
- Fix the binding-rule reconciliation so it is idempotent: look up an existing rule by `BindName` before deciding create vs. update.
- Introduce a `ConsulKV` CRD and its controller for declarative management of Consul KV entries, following the same patterns as `ConsulACLReconciler`.
- Ship all changes without breaking existing `ConsulACL` deployments.

## Non-Goals

- Per-policy explicit naming (not requested in the proposal).
- Consul Enterprise namespace awareness.
- Consul KV watches or push-to-CR sync (read-back into Kubernetes).
- Migration of existing Consul-managed binding rules to the new lookup path.
- Any changes to the REST ACL configurator sidecar.

---

## Technical Decisions

### 1. Explicit naming via opt-in flag fields — no new CRD version

**Decision:** Extend `ACLRoleAdapter` and `ACLBindingRuleAdapter` (defined in `acl_api_provider.go`) with optional `ExplicitName bool` and `ExplicitBindName bool` flag fields. When `true`, `convertEntityName` is skipped and the literal name from the struct is used. For `ACLBindingRuleAdapter`, add an optional `AuthMethod string` field; when non-empty it overrides the global `authMethod` variable.

**Rationale:** These structs are the internal representation deserialized from `spec.acl.json`. Extending them with optional fields is fully backward compatible (omitempty JSON tags) and requires no CRD schema change, no new API version, and no conversion webhook. The `ConsulACL` CRD spec field `json` is an opaque string in the CRD schema — its internal structure is not validated by Kubernetes.

**Alternative considered:** Adding typed fields directly on `ConsulACLSpec` as first-class Kubernetes fields (requiring a CRD version bump to `v1beta1`). Rejected: the JSON blob approach is already the established pattern in this codebase; extending it is consistent and defers the complexity of a conversion webhook.

### 2. Idempotent binding-rule reconciliation via list-and-match

**Decision:** In `processBindRules`, before calling `BindingRuleCreate`, call `aclClient.BindingRuleList(authMethod, ...)` and scan for a matching `BindName`. If found, populate the rule's `ID` from the existing entry and call `BindingRuleUpdate`. This mirrors the existing pattern already used for policies and roles (`readPolicy` / `readRole` before create/update).

**Rationale:** Policies and roles already follow the lookup-first pattern. Applying the same approach to binding rules removes the TODO and makes the reconciler consistent. It also avoids Consul-side duplicates when the operator pod restarts between create and status-write.

**Note on auth method override:** When a per-rule `AuthMethod` is specified, the `BindingRuleList` call must use that auth method value (not the global) so the lookup finds rules registered under the correct auth method.

### 3. ConsulKV as a new CRD and controller — same package, same patterns

**Decision:** Add a `ConsulKV` CRD in `api/v1alpha1/consulkv_types.go` and a `ConsulKVReconciler` in `controllers/consulkv_controller.go`. Register it in `main.go` alongside `ConsulACLReconciler`. Use the identical lifecycle: finalizer on creation, apply on generation change, delete on `DeletionTimestamp`.

**Spec shape:**
```
ConsulKVSpec {
  Path  string  // KV path in Consul
  Value string  // value to write
}
ConsulKVStatus {
  GeneralStatus string
}
```

**Reconciliation:**
- Apply: `PUT /v1/kv/{path}` via `consul.KV().Put(...)`.
- Delete: `DELETE /v1/kv/{path}` via `consul.KV().Delete(...)`.
- Finalizer string: `{group}/consulkvconfigurator-controller` (consistent with the existing ACL finalizer pattern).

**Rationale:** Using the same `api/v1alpha1` package, the same `CustomResourceUpdater` pattern, and the same `makeAclClient` parent Consul client follows established conventions. The KV API (`consul.KV()`) is already available on the same Consul client used for ACL operations. No new dependencies.

**Alternative considered:** A separate operator binary. Rejected: unnecessary operational complexity; the existing deployment is a single pod with co-located containers, and the operator already manages one controller type — adding a second controller to the same manager is the idiomatic controller-runtime approach.

### 4. RBAC — extend the existing ClusterRole

**Decision:** Add `consulkvs`, `consulkvs/status`, and `consulkvs/finalizers` verbs to the existing `//+kubebuilder:rbac:groups=...` marker in `consulkv_controller.go`. The Helm ClusterRole template (`acl-configurator-clusterrole.yaml`) uses a wildcard (`*`) on `consulAclConfigurator.apiGroup`, so it covers `consulkvs` automatically without modification.

**Rationale:** No new ServiceAccount or ClusterRoleBinding is needed. The existing ClusterRole already grants full access to the configured API group via wildcard — new resource types in the same group are covered without Helm changes.

### 5. CRD installation via Helm `crds/` directory

**Decision:** Add `consulkv_crd.yaml` to `charts/helm/consul-service/crds/` alongside the existing `consul_acl_configurator_crd.yaml`. Helm installs CRDs on `helm install` and leaves them on `helm uninstall` (standard Helm CRD lifecycle). No Helm hooks or Job-based CRD installation.

**Rationale:** This is the pattern already used for `ConsulACL`. Consul CRDs managed by connect-inject follow the same approach. Keeping it consistent avoids a two-class CRD installation model.

### 6. No changes to ConsulACL CRD schema

**Decision:** The `ConsulACL` CRD schema (`spec.acl.json`) remains `type: string`. The new fields live inside the JSON blob, not in the Kubernetes schema.

**Rationale:** Kubernetes does not validate the JSON blob's structure. Adding the new fields only requires updating the Go structs and controller logic, not the CRD YAML. This avoids a CRD version bump and a conversion webhook entirely.

---

## Risks / Trade-offs

| Risk | Severity | Mitigation |
|---|---|---|
| Binding-rule list is scoped to a single auth method; per-rule auth method override requires a separate list call per distinct auth method value | Low | Each override value triggers its own `BindingRuleList` call; result is cached within the reconcile loop for that invocation |
| Duplicate Consul ACL resources if an operator pod is killed between `BindingRuleCreate` and status write (pre-existing gap) | Medium | Idempotent lookup-before-create (Decision 2) eliminates this for new reconciles; stale duplicates from before the fix require manual cleanup |
| `ConsulKV` controller shares the Consul token with the ACL controller; the bootstrap token must have KV write permissions | Medium | Document requirement; the bootstrap token in standard Consul deployments already has full permissions. Teams using scoped bootstrap tokens must extend it |
| ExplicitName flag in JSON is invisible to Kubernetes admission (no schema validation) | Low | Invalid configurations surface as Consul API errors reflected in `.status`; acceptable given the existing pattern |
| CRDs in `crds/` are not updated on `helm upgrade` (Helm limitation) | Low | Documented in Helm's own docs; operators must run `kubectl apply -f crds/` on upgrade when the CRD schema changes |

---

## Migration Plan

1. **No action required for existing `ConsulACL` resources.** The new fields are optional with `omitempty`. Existing resources that do not include `ExplicitName`, `ExplicitBindName`, or per-rule `AuthMethod` continue to behave as before — `convertEntityName` is called when the flag is absent or false.

2. **Binding-rule idempotency fix** (Decision 2) changes behavior for resources that already have binding rules: the first reconcile after upgrade will attempt a list and may find existing rules (previously created with duplicates). The reconciler will update the first matching rule and skip re-creation. Operators should verify binding-rule counts in Consul after upgrade and clean up any pre-existing duplicates.

3. **ConsulKV CRD** is a new resource type — no existing objects to migrate.

4. **Helm upgrade**: the new CRD YAML in `crds/` is not applied automatically on `helm upgrade`. Operators must apply `consulkv_crd.yaml` before upgrading to the new chart version when `ConsulKV` resources are intended to be used.

---

## Open Questions

1. **Explicit role name and cross-namespace sharing**: when `ExplicitName: true` is set on a role, and the same role name appears in two different `ConsulACL` resources, both reconcilers will call `RoleUpdate` on the same Consul entity. Is this intentional (shared role ownership) or a misconfiguration that should be detected and rejected?

2. **ConsulKV value encoding**: should the `spec.value` field support arbitrary binary data (base64-encoded) or only UTF-8 strings? The Consul KV API accepts `[]byte`, so base64 is possible without extra dependencies.

3. **ConsulKV path ownership**: if two `ConsulKV` resources target the same Consul path, the last writer wins. Should the controller detect this (e.g., by writing the resource UID as metadata) and refuse to overwrite?

4. **Binding-rule deletion with per-rule auth method**: when a `ConsulACL` is deleted, `deleteBindingRules` calls `BindingRuleList(authMethod, ...)` using the global auth method. If some rules were created with a per-rule override, those rules will not be found in the global-scoped list and will be leaked. Should deletion iterate over all auth methods referenced in the config, or should the per-rule auth method value be recorded in status?

5. **ConsulACL status for binding rules**: the current `BindRulesStatus` is a free-form string serialized from `StatusHolder`. Should the `ConsulKV` status follow the same pattern (single `GeneralStatus` string) or adopt a structured `Conditions` array aligned with Kubernetes conventions?
