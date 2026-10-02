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

> _Superseded by Decision 7: the flag is the CR-level field `spec.acl.explicitName`. The per-rule `AuthMethod` part of this decision still applies._

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
  KV { Entries []{Key, Value}, PurgeOnDelete bool, OperatorNamespace string }
}
ConsulKVStatus {
  Entries []{Key, Status, Owned}, GeneralStatus, ManagedBy string, Conditions []metav1.Condition
}
```

**Reconciliation:**
- Apply: transactional batches of at most 64 operations (`KV().Txn` with `KVCAS`), with ownership tracked in `Flags` (see Decision 9).
- Delete: per key, decrement `Flags` or delete the key at `Flags<=1`; with `purgeOnDelete` a recursive delete (see Decision 10).
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

### 7. Naming flag lives at `spec.acl.explicitName`, not per entity

**Decision (supersedes Decision 1):** the flag is a single CR-level field `spec.acl.explicitName` (typed in `ACL`), not `ExplicitName`/`ExplicitBindName` inside the JSON blob. It applies to policies, roles and binding rules of that CR.

**Rationale:** the cross-CR sharing pattern requires verbatim names for policies as well, and one switch per CR is simpler to reason about than three per-entity flags. The field is optional (`omitempty`), so existing CRs keep the prefixed naming.

### 8. Global JWT auth method with a JWKS proxy

**Decision:** the operator creates (and on every start updates) a global auth method `applications-k8s-m2m` of type `jwt`. Consul servers validate Kubernetes service-account tokens against the API server's public keys, fetched from `JWKS_URL`. Because the API server endpoints require authentication, the chart deploys `kubectl proxy` (`acl-configurator-jwks-proxy`) that exposes only `/openid/v1/jwks` and `/.well-known/openid-configuration`, read-only, under its own ServiceAccount without extra RBAC. `ClaimMappings` map `/kubernetes.io/namespace` → `namespace` and `/kubernetes.io/serviceaccount/name` → `serviceaccount`; therefore binding-rule selectors use `value.namespace` and `value.serviceaccount`.

**Rationale:** a `jwt` method does not need a reviewer token with `TokenReview` permissions in Consul and works for services outside the Consul datacenter's Kubernetes cluster as long as they present a service-account JWT.

**Trade-offs and known limitations (tracked in tasks 20.x):**
- `BoundIssuer`/`BoundAudiences` are hard-coded to `https://kubernetes.default.svc.cluster.local`. Clusters with a different `--service-account-issuer` (OpenShift, EKS, GKE, AKS, custom cluster domain) reject every login until these values are detected from the cluster (task 20.6: by default the `issuer` of `/.well-known/openid-configuration`, overridable in `values.yaml`; audience is assumed to equal the issuer, which is the default `--api-audiences`).
- The selector format depends on the method type. It is currently always `value.*`, so a per-rule `AuthMethod` of type `kubernetes` yields a rule that never matches.
- The proxy is a single replica without PodDisruptionBudget and without the scheduling knobs available for the other components; while it is down, key lookups for unknown `kid` (key rotation, Consul restart) fail and new logins are rejected.
- `EnsureApplicationsAuthMethod` overwrites the method on every start, so manual corrections are reverted.

### 9. Ownership of shared entities

**Decision:** for `explicitName: true` the operator cannot identify its own entities by name prefix, so shared ownership is tracked in the entity itself:
- **Policies:** the namespaces of the owning CRs are stored in the description as `[consul-acl-owners: ns1, ns2]`. A policy is deleted when its last owner is removed.
- **ConsulKV keys:** the `Flags` field is a reference counter. A new key is created with `Flags=1`; another CR using the same key increments the counter; removal decrements it and deletes the key at `Flags<=1`. A key created outside the operator (`Flags=0`) is written but never owned and never deleted.
- **Stale policies on update** are found by parsing the previous `policiesStatus` string.

**Limitations (tasks 21.x, 22.x):**
- Roles and binding rules have no owner tracking. Deleting one CR deletes a shared role/rule and revokes all tokens of the role, including tokens used by other CRs; stale clean-up of roles and rules is disabled in explicit mode, so entries removed from the spec stay in Consul.
- The status string is a human-readable value, not a reliable store: it is lost on restore from backup and its format is not a contract. A structured source is needed.
- Both batch operations are atomic per batch of 64 operations, not as a whole. A failure after the first batch leaves `Flags` out of sync with `status` (counter incremented twice on retry, or decremented twice on removal, which can delete a key still used by another CR).

### 10. `purgeOnDelete` for ConsulKV

**Decision:** with `purgeOnDelete: true` the controller removes the declared keys on CR deletion with a recursive delete (`DeleteTree`) instead of the ref-count decrement.

**Known risk:** Consul `recurse` matches a string prefix, not a path hierarchy. A key `config/app` also removes `config/application/...` and `config/app-gateway/...`. Planned fix (task 22.4): purge the exact key and the tree `<key>/` (the slash is appended, a key without it is not rejected). Purge is intentionally unconditional: it removes the whole path even if other resources use it.

### 11. Operator concurrency

**Decision:** the ClusterRole grants `coordination.k8s.io/leases`, but leader election is not enabled (`--leader-elect` defaults to `false`). With the default `RollingUpdate` strategy two operators run in parallel during an upgrade; ACL reconciliation (lookup-before-create, owner list in description) is not safe in that case. Task 23.1 enables leader election or switches to `Recreate`.

---

## Risks / Trade-offs

| Risk | Severity | Mitigation |
|---|---|---|
| Binding-rule list is scoped to a single auth method; per-rule auth method override requires a separate list call per distinct auth method value | Low | Each override value triggers its own `BindingRuleList` call; result is cached within the reconcile loop for that invocation |
| Duplicate Consul ACL resources if an operator pod is killed between `BindingRuleCreate` and status write (pre-existing gap) | Medium | Idempotent lookup-before-create (Decision 2) eliminates this for new reconciles; stale duplicates from before the fix require manual cleanup |
| `ConsulKV` controller shares the Consul token with the ACL controller; the bootstrap token must have KV write permissions | Medium | Document requirement; the bootstrap token in standard Consul deployments already has full permissions. Teams using scoped bootstrap tokens must extend it |
| ExplicitName flag in JSON is invisible to Kubernetes admission (no schema validation) | Low | Invalid configurations surface as Consul API errors reflected in `.status`; acceptable given the existing pattern |
| CRDs in `crds/` are not updated on `helm upgrade` (Helm limitation) | Low | Documented in Helm's own docs; operators must run `kubectl apply -f crds/` on upgrade when the CRD schema changes |
| Switching the global auth method to `applications-k8s-m2m` (JWT) leaves binding rules under the old `-k8s-auth-method` that the operator no longer updates or deletes | High | Mark as breaking; document migration and manual clean-up; clients must log in through the new method (task 20.8) |
| Hard-coded JWT `BoundIssuer`/`BoundAudiences` reject all logins on clusters with another service-account issuer | High | Detect from the cluster OpenID configuration, overridable in values (task 20.6) |
| Network error in `processBindRules` swallowed by a shadowed `err`: condition `Successful=True`, no requeue | High | Fix the shadowing, add test, enable `govet shadow` (task 21.3) |
| `purgeOnDelete` deletes by raw string prefix and removes sibling keys such as `config/application/...` | High | Purge `<key>` and `<key>/` only (task 22.4) |
| Shared role/rule deleted and role tokens revoked when one of several CRs is deleted (`explicitName: true`) | Medium | Owner tracking for roles and rules (tasks 7.2, 21.5) |
| Partial failure across KV batches desynchronises `Flags` and status | Medium | Per-batch results (task 22.6) |
| Duplicate key in one ConsulKV makes every transaction of the batch fail with a CAS conflict | Medium | Last entry wins, earlier duplicates marked skipped (task 22.5) |
| JWKS proxy is a single replica and a single point of failure for new logins | Medium | Replicas and PDB (task 20.7) |
| Two operator pods run in parallel during a rolling update | Medium | Leader election or `Recreate` (task 23.1) |

---

## Migration Plan

1. **No action required for existing `ConsulACL` resources.** The new fields are optional with `omitempty`. Existing resources that do not include `ExplicitName`, `ExplicitBindName`, or per-rule `AuthMethod` continue to behave as before — `convertEntityName` is called when the flag is absent or false.

2. **Binding-rule idempotency fix** (Decision 2) changes behavior for resources that already have binding rules: the first reconcile after upgrade will attempt a list and may find existing rules (previously created with duplicates). The reconciler will update the first matching rule and skip re-creation. Operators should verify binding-rule counts in Consul after upgrade and clean up any pre-existing duplicates.

3. **ConsulKV CRD** is a new resource type — no existing objects to migrate.

3a. **Global auth method change (breaking).** After the upgrade the operator creates `applications-k8s-m2m` and creates binding rules under it. Rules created earlier under `{fullname}-k8s-auth-method` stay in Consul untouched and keep serving clients that still log in through the old method, but later CR changes are applied only to the new rules. Steps: (a) upgrade the operator, (b) switch client services to log in through `applications-k8s-m2m` with a service-account JWT, (c) remove the old binding rules manually, (d) review `connect-inject` and `backup-daemon` settings that still reference the old methods.

4. **Helm upgrade**: the new CRD YAML in `crds/` is not applied automatically on `helm upgrade`. Operators must apply `consulkv_crd.yaml` before upgrading to the new chart version when `ConsulKV` resources are intended to be used.

---

## Open Questions

1. **Explicit role name and cross-namespace sharing** — _Partially resolved_: sharing is intentional (see Decision 9). Owner tracking exists for policies and ConsulKV keys; it is still missing for roles and binding rules (tasks 7.2, 21.5).

2. **ConsulKV value encoding**: should the `spec.value` field support arbitrary binary data (base64-encoded) or only UTF-8 strings? The Consul KV API accepts `[]byte`, so base64 is possible without extra dependencies.

3. **ConsulKV path ownership** — _Resolved_: ownership is tracked with the `Flags` reference counter (Decision 9); keys created outside the operator are written but not owned.

4. **Binding-rule deletion with per-rule auth method** — _Resolved_: `deleteBindingRules` iterates over every distinct auth method referenced in the config (task 8). Rules left under an auth method that was removed from the config, including the pre-upgrade `-k8s-auth-method`, are not found and need manual clean-up.

5. **ConsulACL status for binding rules** — _Resolved for ConsulKV_: `ConsulKVStatus` has per-key `entries`, `generalStatus`, `managedBy` and `conditions`. ConsulACL keeps free-form status strings; making them structured is required for reliable stale clean-up (task 21.4).
