# Specification: consul-acl-auth-method

## Overview

Extends the Consul ACL configurator operator with a CR-level `spec.acl.explicitName` flag that controls whether entity names are used verbatim or auto-prefixed, per-rule AuthMethod overrides on binding rules, idempotent binding-rule reconciliation, and complete create/update/delete lifecycle handling including removed-element cleanup.

All changes are additive. Existing `ConsulACL` resources that do not use the new fields continue to behave as before.

---

## Terminology

- **ConsulACL** — the Kubernetes custom resource that declares desired Consul ACL policies, roles, and binding rules.
- **ACL configuration** — the JSON value stored in `spec.acl.json`, deserialized into policies, roles, and binding rules at reconcile time.
- **`spec.acl.explicitName`** — an optional boolean field on the `spec.acl` object. When `true`, entity names from the ACL configuration are used verbatim. When absent or `false`, entity names are prefixed automatically.
- **prefixed name** — an entity name produced by concatenating the CR name, CR namespace, and the entity's configured name: `{crName}_{crNamespace}_{name}`.
- **explicit name** — the literal name supplied in the ACL configuration, used as-is without prefixing. Applies when `spec.acl.explicitName: true`.
- **BindName** — the name of the Consul ACL role that a binding rule binds a Kubernetes service account identity to.
- **AuthMethod** — the Consul ACL authentication method under which a binding rule is registered.
- **global AuthMethod** — the auth method name read from the `CONSUL_AUTH_METHOD_NAME` environment variable at operator startup, applied to all binding rules that do not specify a per-rule override.

---

## ADDED Requirements

### Requirement: Role Naming — Default Prefixed

When `spec.acl.explicitName` is absent or `false`, the operator SHALL use the prefixed name `{crName}_{crNamespace}_{name}` as the Consul role name.

#### Scenario: Role created with prefixed name

- **WHEN** a `ConsulACL` resource with name `myapp` in namespace `staging` is reconciled with `spec.acl.explicitName` absent, and a role entry has `Name: "reader"`
- **THEN** the operator SHALL create or update a Consul role named `myapp_staging_reader`

---

### Requirement: Role Naming — Explicit

When `spec.acl.explicitName: true`, the operator SHALL use the literal `Name` value from each role entry as the Consul role name, bypassing the prefixed-name convention.

> **TODO — Explicit naming scope for policies**: The Jira problem statement names only roles and binding rules as targets for explicit naming. However, the Jira examples show policies with verbatim names when `spec.acl.explicitName: true` is set, which the cross-CR sharing pattern requires. Confirm whether `spec.acl.explicitName: true` also bypasses prefixing for policies before finalizing implementation.

> **TODO — Cross-CR policy references in `policy_names`**: The second Jira example has a role whose `policy_names` list references policies defined in a separate `ConsulACL` CR. The operator currently resolves `policy_names` only against policies processed in the same reconcile cycle. Confirm whether the operator must also look up policies by verbatim name directly from Consul to support cross-CR references.

#### Scenario: Role created with explicit name

- **WHEN** a `ConsulACL` resource with `spec.acl.explicitName: true` is reconciled and a role entry has `Name: "staging_myservice"`
- **THEN** the operator SHALL create or update a Consul role named exactly `staging_myservice`

#### Scenario: Explicit name references a pre-existing Consul role

- **WHEN** a `ConsulACL` resource has `spec.acl.explicitName: true`, a role entry specifies a name, and a Consul role with that exact name already exists in Consul
- **THEN** the operator SHALL update the existing role rather than create a new one

---

### Requirement: BindingRule BindName — Default Prefixed

When `spec.acl.explicitName` is absent or `false`, the operator SHALL use the prefixed name `{crName}_{crNamespace}_{bindName}` as the Consul binding rule's `BindName`.

#### Scenario: BindingRule created with prefixed BindName

- **WHEN** a `ConsulACL` resource with name `myapp` in namespace `staging` is reconciled with `spec.acl.explicitName` absent, and a binding-rule entry has `BindName: "reader"`
- **THEN** the operator SHALL create or update a Consul binding rule whose `BindName` is `myapp_staging_reader`

---

### Requirement: BindingRule BindName — Explicit

When `spec.acl.explicitName: true`, the operator SHALL use the literal `BindName` value from each binding-rule entry, bypassing the prefixed-name convention.

#### Scenario: BindingRule created with explicit BindName

- **WHEN** a `ConsulACL` resource with `spec.acl.explicitName: true` is reconciled and a binding-rule entry has `BindName: "${serviceaccount.namespace}_${serviceaccount.name}"`
- **THEN** the operator SHALL create or update a Consul binding rule whose `BindName` is exactly `${serviceaccount.namespace}_${serviceaccount.name}`

---

### Requirement: AuthMethod per Binding Rule — Default

When a binding-rule entry does not supply an `AuthMethod` value, the operator SHALL register the binding rule under the global AuthMethod.

#### Scenario: BindingRule uses global AuthMethod

- **WHEN** a binding-rule entry has no `AuthMethod` field set, and the operator's `CONSUL_AUTH_METHOD_NAME` is `cluster-k8s-auth-method`
- **THEN** the binding rule SHALL be created or updated with `AuthMethod` set to `cluster-k8s-auth-method`

---

### Requirement: AuthMethod per Binding Rule — Per-Rule Override

When a binding-rule entry supplies a non-empty `AuthMethod` value, the operator SHALL register that binding rule under the specified auth method, overriding the global AuthMethod for that rule only.

#### Scenario: BindingRule overrides AuthMethod

- **WHEN** a binding-rule entry has `AuthMethod: "new_auth_method"` and the global AuthMethod is `cluster-k8s-auth-method`
- **THEN** the binding rule SHALL be created or updated with `AuthMethod` set to `new_auth_method`

#### Scenario: Multiple binding rules with different AuthMethods

- **WHEN** a single `ConsulACL` resource declares two binding-rule entries, one with no `AuthMethod` override and one with `AuthMethod: "new_auth_method"`
- **THEN** the first binding rule SHALL be registered under the global AuthMethod, and the second SHALL be registered under `new_auth_method`

---

### Requirement: Idempotent Binding-Rule Reconciliation — Lookup Before Create

Before creating a binding rule, the operator SHALL query the Consul ACL API to list existing binding rules under the applicable AuthMethod and check whether a rule with a matching `BindName` already exists.

#### Scenario: Binding rule does not exist — create

- **WHEN** reconciliation processes a binding-rule entry and no existing Consul binding rule has a matching `BindName` under the applicable AuthMethod
- **THEN** the operator SHALL create a new binding rule in Consul

#### Scenario: Binding rule already exists — update

- **WHEN** reconciliation processes a binding-rule entry and an existing Consul binding rule with a matching `BindName` exists under the applicable AuthMethod
- **THEN** the operator SHALL update the existing binding rule rather than create a new one

---

### Requirement: Idempotent Binding-Rule Reconciliation — AuthMethod-Scoped Lookup

When a binding-rule entry specifies a per-rule `AuthMethod` override, the operator SHALL perform the lookup against that specific AuthMethod, not the global AuthMethod.

#### Scenario: Lookup scoped to per-rule AuthMethod

- **WHEN** a binding-rule entry has `AuthMethod: "custom-auth"` and an existing Consul binding rule with a matching `BindName` is registered under `custom-auth`
- **THEN** the operator SHALL find that rule and update it rather than create a duplicate

---

### Requirement: Finalizer on Creation

When a `ConsulACL` resource is created and does not yet carry the operator's finalizer, the operator SHALL add the finalizer before performing any Consul API calls.

#### Scenario: Finalizer added on first reconcile

- **WHEN** a `ConsulACL` resource is created and its finalizer list does not contain the operator's finalizer
- **THEN** the operator SHALL add the finalizer to the resource and persist the update before proceeding

---

### Requirement: Apply on Generation Change

The operator SHALL reconcile the desired ACL state against Consul on every change that increments `metadata.generation`. It SHALL NOT re-reconcile when only `status` fields change.

The reconciliation SHALL handle elements removed from the spec: ACL entities that were present in the previous spec but are absent from the updated spec SHALL be deleted from Consul.

#### Scenario: Reconcile triggered by spec change

- **WHEN** a `ConsulACL` resource's `spec.acl.json` is updated, causing `metadata.generation` to increment
- **THEN** the operator SHALL reconcile all policies, roles, and binding rules against the updated configuration

#### Scenario: Status-only update does not trigger reconcile

- **WHEN** the operator writes a status update to a `ConsulACL` resource and no spec fields change
- **THEN** the operator SHALL NOT trigger an additional reconcile cycle for that update

#### Scenario: Removed entity deleted from Consul on update

- **WHEN** a `ConsulACL` resource is updated and an entity (policy, role, or binding rule) that was present in the previous spec is absent from the updated spec
- **THEN** the operator SHALL delete that entity from Consul

---

### Requirement: Apply Order

During reconciliation, the operator SHALL process entities in the following order: policies first, then roles (which may reference processed policies by ID), then binding rules.

#### Scenario: Roles reference policies processed in the same reconcile

- **WHEN** a `ConsulACL` resource declares both policies and roles that reference those policies
- **THEN** the operator SHALL ensure all policies are created or updated before resolving policy links for roles

---

### Requirement: Deletion via Finalizer

When a `ConsulACL` resource has a non-zero `DeletionTimestamp` and the operator's finalizer is present, the operator SHALL delete all managed Consul entities, revoke or reject any associated Consul tokens, and then remove the finalizer.

> **TODO — Token revocation mechanism**: The Jira requires deletion to "revoke/reject any corresponding tokens." It is not specified whether the operator must call the Consul token-revocation API directly, or whether this is the responsibility of the existing `remove-tokens` CronJob. Confirm before implementing.

#### Scenario: Delete removes Consul entities in reverse order

- **WHEN** a `ConsulACL` resource is deleted
- **THEN** the operator SHALL delete binding rules first, then roles, then policies from Consul, revoke or reject any associated Consul tokens, and then remove its finalizer from the resource

#### Scenario: Finalizer removed after successful Consul cleanup

- **WHEN** all Consul ACL entities for a `ConsulACL` resource have been successfully deleted
- **THEN** the operator SHALL remove its finalizer from the resource, allowing Kubernetes to complete the deletion

---

### Requirement: Deletion with Per-Rule AuthMethod

When deleting binding rules for a `ConsulACL` resource that contains binding-rule entries with per-rule `AuthMethod` values, the operator SHALL query binding rules under each distinct AuthMethod referenced in the configuration.

#### Scenario: Binding rules with different AuthMethods all removed on delete

- **WHEN** a `ConsulACL` resource being deleted contains binding rules registered under both the global AuthMethod and a per-rule override AuthMethod
- **THEN** the operator SHALL query and remove binding rules under each applicable AuthMethod

---

### Requirement: Per-Entity Status After Apply

After each reconcile, the operator SHALL update `status.policiesStatus`, `status.rolesStatus`, and `status.bindRulesStatus` to reflect the outcome for each managed entity.

#### Scenario: Successful reconcile reflects created/updated status

- **WHEN** a `ConsulACL` resource is reconciled and all Consul API calls succeed
- **THEN** the operator SHALL write a status string per entity type indicating whether each entity was created or updated

#### Scenario: Partial failure reflected in status

- **WHEN** reconciliation of a `ConsulACL` resource succeeds for policies but fails for one role due to a Consul API error
- **THEN** the operator SHALL record an error entry in `status.rolesStatus` for the failing role while recording success entries for all other entities

---

### Requirement: Network Error Handling

When a Consul API call fails due to a network error, the operator SHALL requeue the reconcile request after the configured `RECONCILE_PERIOD_SECONDS` interval. It SHALL NOT clear the existing status.

#### Scenario: Network error causes requeue

- **WHEN** the operator encounters a network error during a Consul API call
- **THEN** the operator SHALL log the error and requeue the request after `RECONCILE_PERIOD_SECONDS` seconds without updating status

---

### Requirement: Backward Compatibility — Existing Resources Unaffected

A `ConsulACL` resource that does not include `spec.acl.explicitName` SHALL be reconciled using the same prefixed-name behavior as before this change.

#### Scenario: Legacy resource reconciled without modification

- **WHEN** a `ConsulACL` resource created before this change is reconciled after an operator upgrade
- **THEN** the operator SHALL apply the prefixed-name convention to all roles and binding rules, producing identical Consul entities to those produced before the upgrade

---

### Requirement: spec.acl.explicitName Is Optional and Backward Compatible

The `spec.acl.explicitName` field is a new optional field added to `spec.acl`. A `ConsulACL` resource that does not include `spec.acl.explicitName` SHALL be treated as if the field were `false`, preserving the existing prefixed-name behavior. The addition of this optional field SHALL NOT invalidate any existing `ConsulACL` resources.

#### Scenario: Resource without explicitName uses prefixed naming after upgrade

- **WHEN** a `ConsulACL` resource that does not contain `spec.acl.explicitName` is reconciled after an operator upgrade
- **THEN** the operator SHALL apply the prefixed-name convention to all roles and binding rules, producing identical Consul entities to those produced before the upgrade

---

### Requirement: Role Name Required

A role entry that has no `Name` value SHALL be skipped during reconciliation, and an error entry SHALL be recorded in `status.rolesStatus`.

#### Scenario: Role with missing name is skipped

- **WHEN** a `ConsulACL` resource is reconciled and a role entry has an empty `Name` field
- **THEN** the operator SHALL skip that role, record `"Some roles have not got a name"` in `status.rolesStatus`, and continue processing remaining roles

---

### Requirement: BindName Required

A binding-rule entry that has no `BindName` value SHALL be skipped during reconciliation, and an error entry SHALL be recorded in `status.bindRulesStatus`.

#### Scenario: BindingRule with missing BindName is skipped

- **WHEN** a `ConsulACL` resource is reconciled and a binding-rule entry has an empty `BindName` field
- **THEN** the operator SHALL skip that binding rule, record `"Some binding rules have not got a name"` in `status.bindRulesStatus`, and continue processing remaining binding rules