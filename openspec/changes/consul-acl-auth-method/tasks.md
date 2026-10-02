## 1. ConsulACL — API Type: explicitName field

- [x] 1.1 Add `ExplicitName bool` field to the `ACL` struct in `api/v1alpha1/consulacl_types.go` with `json:"explicitName,omitempty"` tag
- [x] 1.2 Use the project's `controller-gen` tooling to regenerate all generated Kubernetes API artifacts after adding the `ExplicitName` field. Do not edit generated files manually.

> Covers: Role Naming — Default Prefixed, Role Naming — Explicit, BindingRule BindName — Default Prefixed, BindingRule BindName — Explicit, spec.acl.explicitName Is Optional and Backward Compatible

---

## 2. ConsulACL — Role naming logic

- [x] 2.1 Update `convertRoleAdapterToRole` in `consulacl_controller.go` to check `cr.Spec.ACL.ExplicitName`; when `true`, use `roleAdapter.Name` verbatim instead of calling `convertEntityName`
- [x] 2.2 Propagate the `explicitName` flag through `processRoles` so the flag is available when constructing the Consul role name
- [x] 2.3 Write unit tests for `convertRoleAdapterToRole` covering: (a) `ExplicitName: false` produces prefixed name, (b) `ExplicitName: true` produces verbatim name, (c) pre-existing role found by verbatim name triggers update not create

> Covers: Role Naming — Default Prefixed, Role Naming — Explicit

---

## 3. ConsulACL — BindingRule BindName naming logic

- [x] 3.1 Update `convertBindRuleAdapterToBindRule` in `consulacl_controller.go` to check `explicitName`; when `true`, use `bindRuleAdapter.BindName` verbatim instead of calling `convertEntityName`
- [x] 3.2 Propagate the `explicitName` flag through `processBindRules` so the flag is available when constructing `BindName`
- [x] 3.3 Write unit tests for `convertBindRuleAdapterToBindRule` covering: (a) `ExplicitName: false` produces prefixed `BindName`, (b) `ExplicitName: true` produces verbatim `BindName`

> Covers: BindingRule BindName — Default Prefixed, BindingRule BindName — Explicit

---

## 4. ConsulACL — AuthMethod per binding rule

- [x] 4.1 Add `AuthMethod string` field to `ACLBindingRuleAdapter` in `acl_api_provider.go` with `json:"AuthMethod,omitempty"` tag
- [x] 4.2 Update `convertBindRuleAdapterToBindRule` to set `bindingRule.AuthMethod` to `bindRuleAdapter.AuthMethod` when non-empty, falling back to the global `authMethod` variable otherwise
- [x] 4.3 Write unit tests covering: (a) empty `AuthMethod` falls back to global, (b) non-empty `AuthMethod` overrides global, (c) two rules in one CR each use their respective auth methods

> Covers: AuthMethod per Binding Rule — Default, AuthMethod per Binding Rule — Per-Rule Override

---

## 5. ConsulACL — Idempotent binding-rule reconciliation

- [x] 5.1 Update `processBindRules` to call `aclClient.BindingRuleList(applicableAuthMethod, ...)` before creating a rule, using the per-rule `AuthMethod` (or global fallback) as the list scope
- [x] 5.2 Scan the returned list for a matching `BindName`; if found, populate the rule's `ID` and call `BindingRuleUpdate`; if not found, call `BindingRuleCreate`
- [x] 5.3 Write unit tests covering: (a) rule absent → create called, (b) rule present under same auth method → update called with existing ID, (c) rule present under different auth method → not matched, new rule created

> Covers: Idempotent Binding-Rule Reconciliation — Lookup Before Create, Idempotent Binding-Rule Reconciliation — AuthMethod-Scoped Lookup

---

## 6. ConsulACL — Update: handle removed entities

- [x] 6.1 Design and implement a strategy to detect entities removed from `spec.acl.json` on update: fetch the current spec's entity names, compare with Consul entities whose names match the CR's naming pattern (prefixed or explicit), and delete entities no longer declared
- [x] 6.2 Apply removal in order: binding rules first, then roles, then policies — consistent with the deletion order in `deleteAclEntities`
- [x] 6.3 Write unit tests covering: (a) policy removed from spec is deleted from Consul on next reconcile, (b) role removed from spec is deleted, (c) binding rule removed from spec is deleted, (d) entities still in spec are not deleted

> Covers: Apply on Generation Change (removed entity scenario)

---

## 7. ConsulACL — Deletion: token revocation

> **Resolved**: the operator calls the Consul token API directly (`revokeRoleTokens`: `TokenListFiltered` by role, then `TokenDelete`) as part of `deleteAclEntities`, before the roles are removed.

- [x] 7.1 Implement token revocation as part of `deleteACL`, called before finalizer removal
- [x] 7.2 Revoke role tokens only when the last owner of the role is removed (relevant for `explicitName: true` roles shared between several CRs; today tokens of a shared role are revoked when any one CR is deleted)

> Covers: Deletion via Finalizer (token revocation clause)

---

## 8. ConsulACL — Deletion: per-rule AuthMethod binding-rule cleanup

- [x] 8.1 Update `deleteBindingRules` to collect all distinct `AuthMethod` values from the binding-rule entries (both the global AuthMethod and any per-rule overrides)
- [x] 8.2 For each distinct AuthMethod, call `aclClient.BindingRuleList(authMethod, ...)` and remove matching rules
- [x] 8.3 Write unit tests covering: (a) all rules deleted when all use global AuthMethod, (b) rules deleted under both global and override AuthMethod when CR uses mixed methods

> Covers: Deletion with Per-Rule AuthMethod

---

## 9. ConsulKV — API types

- [x] 9.1 Create `api/v1alpha1/consulkv_types.go` with `ConsulKVEntry`, `ConsulKVConfig`, `ConsulKVSpec`, `ConsulKVStatus`, `ConsulKV`, and `ConsulKVList` types matching the structure defined in the spec
- [x] 9.2 Add `+kubebuilder:object:root=true` and `+kubebuilder:subresource:status` markers to `ConsulKV`
- [x] 9.3 Register `ConsulKV` and `ConsulKVList` with `SchemeBuilder.Register` in `groupversion_info.go` (or in the new types file's `init()`)
- [x] 9.4 Regenerate or manually update `zz_generated.deepcopy.go` to include `DeepCopyInto` and `DeepCopyObject` for all new ConsulKV types

> Covers: Resource Structure

---

## 10. ConsulKV — CRD manifest

- [x] 10.1 Generate the ConsulKV CRD using the project's standard `controller-gen` workflow and verify it matches the Go API types.
- [x] 10.2 Copy the generated CRD YAML into `charts/helm/consul-service/crds/consulkv_crd.yaml`

> Covers: Helm Chart Packages the ConsulKV CRD, Resource Structure

---

## 11. ConsulKV — Controller: scaffold and client setup

- [x] 11.1 Create `controllers/consulkv_controller.go` with `ConsulKVReconciler` struct holding `client.Client` and `*runtime.Scheme`
- [x] 11.2 Reuse the existing `makeAclClient()` Consul client (or its parent `consulApi.Client`) for the KV API — `client.KV()` — so host, port, scheme, TLS, and token are shared
- [x] 11.3 Define the finalizer constant as `{apiGroup}/consulkvconfigurator-controller`, consistent with the ACL finalizer pattern
- [x] 11.4 Add `+kubebuilder:rbac` markers for `consulkvs`, `consulkvs/status`, and `consulkvs/finalizers` (get, list, watch, create, update, patch, delete)
- [x] 11.5 Register `ConsulKVReconciler` with the manager in `main.go` alongside the existing `ConsulACLReconciler`

> Covers: Finalizer on Creation, Operator RBAC Covers consulkvs Resources

---

## 12. ConsulKV — Controller: reconcile loop

- [x] 12.1 Implement the `Reconcile` method: fetch the `ConsulKV` instance; return nil if NotFound (do not requeue)
- [x] 12.2 When `DeletionTimestamp` is zero and finalizer is absent: add the finalizer via `CustomResourceUpdater.UpdateWithRetry` and return
- [x] 12.3 When `DeletionTimestamp` is non-zero and finalizer is present: call `deleteKVEntries`, then remove the finalizer via `UpdateWithRetry`
- [x] 12.4 When active: call `applyKVEntries`, then update per-key status via `UpdateStatusWithRetry`
- [x] 12.5 Install a `predicate.GenerationChangedPredicate` (or equivalent `UpdateFunc` checking generation) in `SetupWithManager` so status-only updates do not trigger reconcile

> Covers: Finalizer on Creation, Finalizer Removed After Cleanup, Apply on Generation Change, Generation-Based Reconcile Trigger, Not-Found Does Not Requeue

---

## 13. ConsulKV — Controller: applyKVEntries

- [x] 13.1 Implement `applyKVEntries`: iterate over `spec.kv.entries`; for each entry with a non-empty key, call `kvClient.Put(&consulApi.KVPair{Key: entry.Key, Value: []byte(entry.Value)}, nil)`
- [x] 13.2 For each entry with an empty key, record an error status for that entry and continue (do not abort the loop)
- [x] 13.3 Return per-key status results (success or error message per key) to the caller for status persistence
- [x] 13.4 On network error from any `Put` call, return the error to `Reconcile` so it can requeue after `RECONCILE_PERIOD_SECONDS`

> Covers: KVPut per Entry on Apply, Verbatim Keys — No Automatic Prefix, Idempotent KVPut, Entry Key Must Not Be Empty, Network Error Causes Requeue

---

## 14. ConsulKV — Controller: deleteKVEntries

- [x] 14.1 Implement `deleteKVEntries`: iterate over `spec.kv.entries`; for each entry, call `kvClient.Delete(entry.Key, nil)`
- [x] 14.2 Treat a "key not found" response from Consul as success for that entry and continue processing remaining entries
- [x] 14.3 On network error, return the error to `Reconcile` (deletion will be retried on next reconcile via requeue)

> Covers: KVDelete per Entry on Delete, Absent KV Entry Does Not Block Deletion

---

## 15. ConsulKV — Controller: status

- [x] 15.1 Define the `ConsulKVStatus` shape to record per-key outcomes (map or slice of key+status pairs)
- [x] 15.2 After `applyKVEntries`, write per-key results into `ConsulKVStatus` and persist via `UpdateStatusWithRetry`; on status update failure after retries, requeue after `RECONCILE_PERIOD_SECONDS`

> Covers: Per-Key Status, Status Update Failure Causes Requeue

---

## 16. ConsulKV — Unit tests

- [x] 16.1 Test `applyKVEntries`: (a) all entries written verbatim, (b) empty-key entry skipped with error status, (c) idempotent re-apply succeeds, (d) network error returned
- [x] 16.2 Test `deleteKVEntries`: (a) all entries deleted, (b) absent key treated as success, (c) network error returned
- [x] 16.3 Test reconcile loop: (a) finalizer added on first reconcile, (b) active reconcile calls apply and writes status, (c) deletion reconcile calls delete and removes finalizer, (d) status-only generation change does not trigger KV write, (e) not-found returns without error

> Covers: all ConsulKV requirements

---

## 17. Helm: RBAC for ConsulKV

- [x] 17.1 Verify that the existing `acl-configurator-clusterrole.yaml` wildcard on `consulAclConfigurator.apiGroup` covers `consulkvs`; if not, add explicit rules for `consulkvs`, `consulkvs/status`, `consulkvs/finalizers`

> Covers: Operator RBAC Covers consulkvs Resources

---

## 18. Integration tests

- [x] 18.1 Write an integration test for `ConsulACL` with `explicitName: true`: apply CR, verify Consul role and binding-rule names are verbatim, update CR to remove one entity, verify it is deleted from Consul
- [x] 18.2 Write an integration test for `ConsulACL` with per-rule `AuthMethod`: apply CR, verify binding rule is registered under the overridden auth method
- [x] 18.3 Write an integration test for `ConsulACL` delete: apply and then delete a CR, verify all policies, roles, and binding rules are removed from Consul
- [x] 18.4 Write an integration test for `ConsulKV` apply: apply a CR with multiple entries, verify all keys exist in Consul verbatim; re-apply, verify idempotency
- [x] 18.5 Write an integration test for `ConsulKV` delete: apply then delete a CR, verify all keys are removed from Consul
- [x] 18.6 Write an integration test for `ConsulKV` partial failure: apply a CR with one empty-key entry and two valid entries, verify valid keys are written and the error entry is recorded in `.status`

> Covers: all specification acceptance criteria
 
---

## Acceptance Criteria

- [x] 19.1 Verify that the generated ConsulKV CRD and Helm chart deploy successfully via ArgoCD without requiring manual changes.
  <!-- Static verification (all passed): (1) operator builds cleanly; (2) consulkv_crd.yaml is
       valid YAML in crds/ with no Helm templating, correct group/kind/scope/version/schema;
       (3) config/rbac/role.yaml regenerated — consulkvs, consulkvs/status, consulkvs/finalizers
       now present alongside consulacls rules; (4) Helm ClusterRole wildcard on
       consulAclConfigurator.apiGroup covers consulkvs at runtime; (5) deepcopy complete for
       both ConsulKV and ConsulACL types. Live ArgoCD deploy must be confirmed in target cluster. -->


---

## 20. Global JWT auth method and JWKS proxy

- [x] 20.1 Create/update the global JWT auth method `applications-k8s-m2m` at operator startup (`EnsureApplicationsAuthMethodWithRetry`, exponential backoff) with `ClaimMappings` `/kubernetes.io/namespace → namespace` and `/kubernetes.io/serviceaccount/name → serviceaccount`
- [x] 20.2 Generate binding-rule selectors from claim mappings: `value.namespace == "<ns>" and value.serviceaccount == "<sa>"`
- [x] 20.3 Add JWKS proxy deployment, service and ServiceAccount to the Helm chart; set `JWKS_URL` for the operator
- [x] 20.4 Set `CONSUL_AUTH_METHOD_NAME` of the operator to `applications-k8s-m2m`
- [x] 20.5 Generate the selector depending on the auth-method type (`AuthMethodRead`: `kubernetes` → `serviceaccount.*`, `jwt` → `value.*`) so that a per-rule `AuthMethod` of type `kubernetes` keeps working
- [x] 20.6 `BoundIssuer` and `BoundAudiences`: by default detect them automatically from the `issuer` field of `/.well-known/openid-configuration` served by the JWKS proxy (same host as `JWKS_URL`), using the issuer for both values; allow overriding them in `values.yaml` (`boundIssuer`, `boundAudiences`, passed to the operator as environment variables) — explicit values take precedence over detection. If detection is needed and the request fails, retry with the existing backoff instead of falling back to a hard-coded value. Do not call `AuthMethodUpdate` when the resulting config is unchanged. Add unit tests (issuer detected, override used, request fails, config unchanged)
- [x] 20.7 JWKS proxy availability: more than one replica and a PodDisruptionBudget; support affinity, tolerations, nodeSelector, priorityClassName, extra labels and resources overrides; optional NetworkPolicy restricting access to Consul servers
- [x] 20.8 Document the upgrade path and clean-up of binding rules left under `{fullname}-k8s-auth-method`; update `docs/public/acl-configurator.md` (the `AuthMethod` default and the `Selector` description, which also has namespace and service account name swapped), `connect-inject` login settings and `backup-daemon/scripts/restore.py` where they still reference the old methods
- [x] 20.9 Unit tests for `EnsureApplicationsAuthMethod` (create, update, unchanged)
- [x] 20.10 Document the JWT auth method settings:
  - `docs/public/installation.md`: add `consulAclConfigurator.boundIssuer` and `consulAclConfigurator.boundAudiences` to the parameters table (optional, empty by default, explicit values take precedence over detection);
  - `docs/public/acl-configurator.md`: describe the global JWT auth method `applications-k8s-m2m` (claim mappings, JWKS proxy) and how `BoundIssuer`/`BoundAudiences` are determined: detected automatically from the `issuer` of `/.well-known/openid-configuration`, audience assumed equal to the issuer (the default `--api-audiences`), overridable in `values.yaml` for clusters where it differs; how to check the issuer manually (`kubectl get --raw /.well-known/openid-configuration`); that the operator retries until the issuer can be read.
- [x] 20.11 **[bugfix]** Anchor both alternatives of the JWKS proxy `--accept-paths` (`^(?:/openid/v1/jwks|/\.well-known/openid-configuration)$`): the old expression `^a|b$` let `/openid/v1/jwks/...` and `.../.well-known/openid-configuration` through; remove the duplicated `allowPrivilegeEscalation`/`capabilities` keys from the proxy container `securityContext`
- [x] 20.12 **[bugfix]** Remove the second `tolerations` block at the end of the operator Deployment: with `consulAclConfigurator.tolerations` set, the pod spec had the key twice

> Covers: Global JWT Auth Method, JWKS Proxy, Binding-Rule Selector from Claim Mappings

---

## 21. ConsulACL — explicit policies, ownership and reconcile robustness

- [x] 21.1 Track owners of explicitly named policies in the policy description (`[consul-acl-owners: ns1, ns2]`); delete the policy only when the last owner is removed
- [x] 21.2 Clean up stale explicit policies on update using the previous `policiesStatus`
- [x] 21.3 **[bugfix]** Fix variable shadowing of `err` in `processBindRules` (`existingRules, _, err :=` hides the outer `err`) so that network errors from `BindingRuleCreate`/`BindingRuleUpdate` are returned and the request is requeued; add a unit test "network error in create → error returned"
- [ ] 21.3a Enable `govet` `shadow` in the shared golangci config (`netcracker/.github`, `config/linters`); a local `.github/linters/.golangci.yml` would replace the shared config. The operator code currently has no findings in non-strict mode
- [x] 21.4 Replace parsing of the human-readable status string in `parsePolicyNamesFromStatus` with a structured source (a status field such as `appliedPolicies`, or the owner marker in the policy description in Consul) so that cleanup survives restore from backup and status format changes; add tests for `parsePolicyNamesFromStatus` and `removeStaleExplicitPolicies`
- [x] 21.5 Extend the owner mechanism to roles and binding rules: store the owner list in their `Description` in the same format as for policies (`[consul-acl-owners: ns1, ns2]`, namespaces only; the operator already selects CRs by name, so the CR name is not stored). Create/update adds the namespace; deleting a CR or removing the entity from its spec removes the namespace; the role or rule is deleted, and the role tokens revoked, only when the last owner is removed. Entities without a marker (created before the upgrade) keep the current behaviour. This also enables stale clean-up of roles and rules in explicit mode
- [x] 21.6 **[bugfix]** Fix `StatusHolder.GetStatus()` for `innerErrorHandlingItem` (missing `continue` duplicates the message)
- [x] 21.7 Remove the unused `AuthMethodsStatus` field from `ConsulACLStatus` and from both CRDs
- [x] 21.8 **[bugfix]** Keep the first network error in `processPolicies`, `processRoles` and `processBindRules`: only the error of the last entity was checked, so a network error of an earlier entity was lost when a later call succeeded; add a test "network error on the first role, second succeeds → error returned"
- [x] 21.9 Do not release an entity declared by another ConsulACL resource of the same namespace (the owner list stores namespaces only); applies to deletion and stale clean-up of policies, roles and binding rules; an unparsable sibling configuration fails the operation so that it is retried

> Covers: Explicit Policies, Shared Ownership of Explicit Entities, Network Error Handling

---

## 22. ConsulKV — ownership, purge and batch consistency

- [x] 22.1 Track ownership of keys with the `Flags` reference counter (create → `Flags=1`, additional owner → `Flags+1`, externally created key (`Flags=0`) is not owned)
- [x] 22.2 Write and delete in transactional batches of at most 64 operations (Consul limit) using CAS with retries
- [x] 22.3 Support `purgeOnDelete` (recursive delete of the declared keys on CR deletion) and `operatorNamespace`
- [ ] 22.4 **[bugfix]** `purgeOnDelete`: delete the exact declared key and the tree `<key>/` (append `/` when missing; a key without `/` is accepted), regardless of other users of those keys, so that `config/app` never removes `config/application/...` or `config/app-gateway/...`; add tests for the prefix collision and for keys shared with another CR
- [ ] 22.5 **[bugfix]** Duplicate keys within one CR: do not fail the transaction; the last entry for a key wins, earlier duplicates get status `skipped (duplicate key)` in `status.entries`, all other keys are written normally
- [ ] 22.6 **[bugfix]** Partial batch failure (more than 64 keys, a later batch fails): keys of the batches that were committed are recorded in `status.entries` with `Owned=true`, so on the next reconcile their `Flags` are not incremented again; only the keys of the failed batch get an error status and are retried; `generalStatus`/`Successful` becomes success only after all batches are written. The same for removal of keys: keys already released are not decremented again. Add a test "second batch fails" and correct the "batch is atomic" comment
- [ ] 22.7 Tests for `purgeOnDelete`

> Covers: Key Ownership, Purge on Delete, Duplicate Keys, Batched Writes

---

## 23. Deployment and housekeeping

- [ ] 23.1 Enable leader election for the operator (`args: ["--leader-elect"]`; the RBAC for `coordination.k8s.io/leases` is already in the ClusterRole) or set `strategy: Recreate`, so that two operator pods never reconcile in parallel during a rolling update
- [x] 23.2 **[bugfix]** Revert the accidental change of `statusWritingEnabled` in `values.yaml` back to `true` (integration-test parameter, unrelated to this change)
- [ ] 23.3 **[bugfix]** Align the CRD version annotation: ConsulACL CRD uses `crd.netcracker.com/version: 0.0.19`, ConsulKV CRD and kustomize bases use `crd/version: 0.0.18`; use one key and bump the version of the ConsulKV CRD
