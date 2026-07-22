## 1. ConsulACL — API Type: explicitName field

- [x] 1.1 Add `ExplicitName bool` field to the `ACL` struct in `api/v1alpha1/consulacl_types.go` with `json:"explicitName,omitempty"` tag
- [x] 1.2 Use the project's `controller-gen` tooling to regenerate all generated Kubernetes API artifacts after adding the `ExplicitName` field. Do not edit generated files manually.

> Covers: Role Naming — Default Prefixed, Role Naming — Explicit, BindingRule BindName — Default Prefixed, BindingRule BindName — Explicit, spec.acl.explicitName Is Optional and Backward Compatible

---

## 2. ConsulACL — Role naming logic

- [ ] 2.1 Update `convertRoleAdapterToRole` in `consulacl_controller.go` to check `cr.Spec.ACL.ExplicitName`; when `true`, use `roleAdapter.Name` verbatim instead of calling `convertEntityName`
- [ ] 2.2 Propagate the `explicitName` flag through `processRoles` so the flag is available when constructing the Consul role name
- [ ] 2.3 Write unit tests for `convertRoleAdapterToRole` covering: (a) `ExplicitName: false` produces prefixed name, (b) `ExplicitName: true` produces verbatim name, (c) pre-existing role found by verbatim name triggers update not create

> Covers: Role Naming — Default Prefixed, Role Naming — Explicit

---

## 3. ConsulACL — BindingRule BindName naming logic

- [ ] 3.1 Update `convertBindRuleAdapterToBindRule` in `consulacl_controller.go` to check `explicitName`; when `true`, use `bindRuleAdapter.BindName` verbatim instead of calling `convertEntityName`
- [ ] 3.2 Propagate the `explicitName` flag through `processBindRules` so the flag is available when constructing `BindName`
- [ ] 3.3 Write unit tests for `convertBindRuleAdapterToBindRule` covering: (a) `ExplicitName: false` produces prefixed `BindName`, (b) `ExplicitName: true` produces verbatim `BindName`

> Covers: BindingRule BindName — Default Prefixed, BindingRule BindName — Explicit

---

## 4. ConsulACL — AuthMethod per binding rule

- [ ] 4.1 Add `AuthMethod string` field to `ACLBindingRuleAdapter` in `acl_api_provider.go` with `json:"AuthMethod,omitempty"` tag
- [ ] 4.2 Update `convertBindRuleAdapterToBindRule` to set `bindingRule.AuthMethod` to `bindRuleAdapter.AuthMethod` when non-empty, falling back to the global `authMethod` variable otherwise
- [ ] 4.3 Write unit tests covering: (a) empty `AuthMethod` falls back to global, (b) non-empty `AuthMethod` overrides global, (c) two rules in one CR each use their respective auth methods

> Covers: AuthMethod per Binding Rule — Default, AuthMethod per Binding Rule — Per-Rule Override

---

## 5. ConsulACL — Idempotent binding-rule reconciliation

- [ ] 5.1 Update `processBindRules` to call `aclClient.BindingRuleList(applicableAuthMethod, ...)` before creating a rule, using the per-rule `AuthMethod` (or global fallback) as the list scope
- [ ] 5.2 Scan the returned list for a matching `BindName`; if found, populate the rule's `ID` and call `BindingRuleUpdate`; if not found, call `BindingRuleCreate`
- [ ] 5.3 Write unit tests covering: (a) rule absent → create called, (b) rule present under same auth method → update called with existing ID, (c) rule present under different auth method → not matched, new rule created

> Covers: Idempotent Binding-Rule Reconciliation — Lookup Before Create, Idempotent Binding-Rule Reconciliation — AuthMethod-Scoped Lookup

---

## 6. ConsulACL — Update: handle removed entities

- [ ] 6.1 Design and implement a strategy to detect entities removed from `spec.acl.json` on update: fetch the current spec's entity names, compare with Consul entities whose names match the CR's naming pattern (prefixed or explicit), and delete entities no longer declared
- [ ] 6.2 Apply removal in order: binding rules first, then roles, then policies — consistent with the deletion order in `deleteAclEntities`
- [ ] 6.3 Write unit tests covering: (a) policy removed from spec is deleted from Consul on next reconcile, (b) role removed from spec is deleted, (c) binding rule removed from spec is deleted, (d) entities still in spec are not deleted

> Covers: Apply on Generation Change (removed entity scenario)

---

## 7. ConsulACL — Deletion: token revocation

> **BLOCKED — awaiting clarification**: The Jira requires "revoke/reject any corresponding tokens" on delete, but the mechanism (operator calling Consul token API directly vs. delegating to the existing `remove-tokens` CronJob) is unspecified. Do not implement until the token revocation TODO in the spec is resolved.

- [ ] 7.1 _(Blocked)_ Once mechanism is confirmed: implement token revocation/rejection as part of `deleteACL`, called before finalizer removal

> Covers: Deletion via Finalizer (token revocation clause)

---

## 8. ConsulACL — Deletion: per-rule AuthMethod binding-rule cleanup

- [ ] 8.1 Update `deleteBindingRules` to collect all distinct `AuthMethod` values from the binding-rule entries (both the global AuthMethod and any per-rule overrides)
- [ ] 8.2 For each distinct AuthMethod, call `aclClient.BindingRuleList(authMethod, ...)` and remove matching rules
- [ ] 8.3 Write unit tests covering: (a) all rules deleted when all use global AuthMethod, (b) rules deleted under both global and override AuthMethod when CR uses mixed methods

> Covers: Deletion with Per-Rule AuthMethod

---

## 9. ConsulKV — API types

- [ ] 9.1 Create `api/v1alpha1/consulkv_types.go` with `ConsulKVEntry`, `ConsulKVConfig`, `ConsulKVSpec`, `ConsulKVStatus`, `ConsulKV`, and `ConsulKVList` types matching the structure defined in the spec
- [ ] 9.2 Add `+kubebuilder:object:root=true` and `+kubebuilder:subresource:status` markers to `ConsulKV`
- [ ] 9.3 Register `ConsulKV` and `ConsulKVList` with `SchemeBuilder.Register` in `groupversion_info.go` (or in the new types file's `init()`)
- [ ] 9.4 Regenerate or manually update `zz_generated.deepcopy.go` to include `DeepCopyInto` and `DeepCopyObject` for all new ConsulKV types

> Covers: Resource Structure

---

## 10. ConsulKV — CRD manifest

- [ ] 10.1 Generate the ConsulKV CRD using the project's standard `controller-gen` workflow and verify it matches the Go API types.
- [ ] 10.2 Copy the generated CRD YAML into `charts/helm/consul-service/crds/consulkv_crd.yaml`

> Covers: Helm Chart Packages the ConsulKV CRD, Resource Structure

---

## 11. ConsulKV — Controller: scaffold and client setup

- [ ] 11.1 Create `controllers/consulkv_controller.go` with `ConsulKVReconciler` struct holding `client.Client` and `*runtime.Scheme`
- [ ] 11.2 Reuse the existing `makeAclClient()` Consul client (or its parent `consulApi.Client`) for the KV API — `client.KV()` — so host, port, scheme, TLS, and token are shared
- [ ] 11.3 Define the finalizer constant as `{apiGroup}/consulkvconfigurator-controller`, consistent with the ACL finalizer pattern
- [ ] 11.4 Add `+kubebuilder:rbac` markers for `consulkvs`, `consulkvs/status`, and `consulkvs/finalizers` (get, list, watch, create, update, patch, delete)
- [ ] 11.5 Register `ConsulKVReconciler` with the manager in `main.go` alongside the existing `ConsulACLReconciler`

> Covers: Finalizer on Creation, Operator RBAC Covers consulkvs Resources

---

## 12. ConsulKV — Controller: reconcile loop

- [ ] 12.1 Implement the `Reconcile` method: fetch the `ConsulKV` instance; return nil if NotFound (do not requeue)
- [ ] 12.2 When `DeletionTimestamp` is zero and finalizer is absent: add the finalizer via `CustomResourceUpdater.UpdateWithRetry` and return
- [ ] 12.3 When `DeletionTimestamp` is non-zero and finalizer is present: call `deleteKVEntries`, then remove the finalizer via `UpdateWithRetry`
- [ ] 12.4 When active: call `applyKVEntries`, then update per-key status via `UpdateStatusWithRetry`
- [ ] 12.5 Install a `predicate.GenerationChangedPredicate` (or equivalent `UpdateFunc` checking generation) in `SetupWithManager` so status-only updates do not trigger reconcile

> Covers: Finalizer on Creation, Finalizer Removed After Cleanup, Apply on Generation Change, Generation-Based Reconcile Trigger, Not-Found Does Not Requeue

---

## 13. ConsulKV — Controller: applyKVEntries

- [ ] 13.1 Implement `applyKVEntries`: iterate over `spec.kv.entries`; for each entry with a non-empty key, call `kvClient.Put(&consulApi.KVPair{Key: entry.Key, Value: []byte(entry.Value)}, nil)`
- [ ] 13.2 For each entry with an empty key, record an error status for that entry and continue (do not abort the loop)
- [ ] 13.3 Return per-key status results (success or error message per key) to the caller for status persistence
- [ ] 13.4 On network error from any `Put` call, return the error to `Reconcile` so it can requeue after `RECONCILE_PERIOD_SECONDS`

> Covers: KVPut per Entry on Apply, Verbatim Keys — No Automatic Prefix, Idempotent KVPut, Entry Key Must Not Be Empty, Network Error Causes Requeue

---

## 14. ConsulKV — Controller: deleteKVEntries

- [ ] 14.1 Implement `deleteKVEntries`: iterate over `spec.kv.entries`; for each entry, call `kvClient.Delete(entry.Key, nil)`
- [ ] 14.2 Treat a "key not found" response from Consul as success for that entry and continue processing remaining entries
- [ ] 14.3 On network error, return the error to `Reconcile` (deletion will be retried on next reconcile via requeue)

> Covers: KVDelete per Entry on Delete, Absent KV Entry Does Not Block Deletion

---

## 15. ConsulKV — Controller: status

- [ ] 15.1 Define the `ConsulKVStatus` shape to record per-key outcomes (map or slice of key+status pairs)
- [ ] 15.2 After `applyKVEntries`, write per-key results into `ConsulKVStatus` and persist via `UpdateStatusWithRetry`; on status update failure after retries, requeue after `RECONCILE_PERIOD_SECONDS`

> Covers: Per-Key Status, Status Update Failure Causes Requeue

---

## 16. ConsulKV — Unit tests

- [ ] 16.1 Test `applyKVEntries`: (a) all entries written verbatim, (b) empty-key entry skipped with error status, (c) idempotent re-apply succeeds, (d) network error returned
- [ ] 16.2 Test `deleteKVEntries`: (a) all entries deleted, (b) absent key treated as success, (c) network error returned
- [ ] 16.3 Test reconcile loop: (a) finalizer added on first reconcile, (b) active reconcile calls apply and writes status, (c) deletion reconcile calls delete and removes finalizer, (d) status-only generation change does not trigger KV write, (e) not-found returns without error

> Covers: all ConsulKV requirements

---

## 17. Helm: RBAC for ConsulKV

- [ ] 17.1 Verify that the existing `acl-configurator-clusterrole.yaml` wildcard on `consulAclConfigurator.apiGroup` covers `consulkvs`; if not, add explicit rules for `consulkvs`, `consulkvs/status`, `consulkvs/finalizers`

> Covers: Operator RBAC Covers consulkvs Resources

---

## 18. Integration tests

- [ ] 18.1 Write an integration test for `ConsulACL` with `explicitName: true`: apply CR, verify Consul role and binding-rule names are verbatim, update CR to remove one entity, verify it is deleted from Consul
- [ ] 18.2 Write an integration test for `ConsulACL` with per-rule `AuthMethod`: apply CR, verify binding rule is registered under the overridden auth method
- [ ] 18.3 Write an integration test for `ConsulACL` delete: apply and then delete a CR, verify all policies, roles, and binding rules are removed from Consul
- [ ] 18.4 Write an integration test for `ConsulKV` apply: apply a CR with multiple entries, verify all keys exist in Consul verbatim; re-apply, verify idempotency
- [ ] 18.5 Write an integration test for `ConsulKV` delete: apply then delete a CR, verify all keys are removed from Consul
- [ ] 18.6 Write an integration test for `ConsulKV` partial failure: apply a CR with one empty-key entry and two valid entries, verify valid keys are written and the error entry is recorded in `.status`

> Covers: all specification acceptance criteria
 
---

## Acceptance Criteria

- [ ] 19.1 Verify that the generated ConsulKV CRD and Helm chart deploy successfully via ArgoCD without requiring manual changes.
