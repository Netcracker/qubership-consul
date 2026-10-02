# Specification: consul-acl-auth-method

## Overview

Extends the Consul ACL configurator operator with a CR-level `spec.acl.explicitName` flag that controls whether entity names are used verbatim or auto-prefixed, per-rule AuthMethod overrides on binding rules, idempotent binding-rule reconciliation, and complete create/update/delete lifecycle handling including removed-element cleanup.

Naming, per-rule AuthMethod and lifecycle changes are additive: existing `ConsulACL` resources that do not use the new fields keep the prefixed names. The change of the global auth method to the JWT method `applications-k8s-m2m` and of the generated binding-rule selector is **breaking** (see "Global JWT Auth Method" and "Binding-Rule Selector from Claim Mappings").

---

## Terminology

- **ConsulACL** — the Kubernetes custom resource that declares desired Consul ACL policies, roles, and binding rules.
- **ACL configuration** — the JSON value stored in `spec.acl.json`, deserialized into policies, roles, and binding rules at reconcile time.
- **`spec.acl.explicitName`** — an optional boolean field on the `spec.acl` object. When `true`, entity names from the ACL configuration are used verbatim. When absent or `false`, entity names are prefixed automatically.
- **prefixed name** — an entity name produced by concatenating the CR name, CR namespace, and the entity's configured name: `{crName}_{crNamespace}_{name}`.
- **explicit name** — the literal name supplied in the ACL configuration, used as-is without prefixing. Applies when `spec.acl.explicitName: true`.
- **BindName** — the name of the Consul ACL role that a binding rule binds a Kubernetes service account identity to.
- **AuthMethod** — the Consul ACL authentication method under which a binding rule is registered.
- **global AuthMethod** — the auth method name read from the `CONSUL_AUTH_METHOD_NAME` environment variable at operator startup, applied to all binding rules that do not specify a per-rule override. The Helm chart sets it to `applications-k8s-m2m`.
- **`applications-k8s-m2m`** — the global Consul auth method of type `jwt` that the operator creates and updates at startup.
- **JWKS proxy** — a read-only `kubectl proxy` deployment that exposes the Kubernetes API server's JWKS and OpenID configuration to Consul servers.
- **owner list** — the list of namespaces stored in the description of an explicitly named policy as `[consul-acl-owners: ns1, ns2]`.

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

> **Resolved — scope**: `spec.acl.explicitName: true` bypasses prefixing for policies, roles and binding rules of the CR (needed for cross-CR sharing). Shared ownership of policies is handled by the owner list (see "Shared Ownership of Explicit Policies").

> **Open — cross-CR policy references in `policy_names`**: a role may reference a policy defined in a separate `ConsulACL` CR. Confirm that the operator resolves such policies by verbatim name directly from Consul (`getPolicyLinks`) and not only among the policies processed in the same reconcile.

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

> **Resolved — token revocation mechanism**: the operator calls the Consul token API directly (lists tokens by role and deletes them) before deleting the roles. The `remove-tokens` CronJob is not involved.

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

---

### Requirement: Global JWT Auth Method

At startup the operator SHALL create the Consul auth method `applications-k8s-m2m` of type `jwt` if it does not exist, and update it if it does, retrying with exponential backoff until it succeeds. The method SHALL validate tokens against the keys served at `JWKS_URL` and map the claims `/kubernetes.io/namespace` to `namespace` and `/kubernetes.io/serviceaccount/name` to `serviceaccount`.

The `BoundIssuer` and `BoundAudiences` of the method SHALL be taken from the deployment parameters `boundIssuer` and `boundAudiences` when they are set. When they are not set, the operator SHALL detect them from the `issuer` field of the OpenID configuration served by the JWKS proxy (`/.well-known/openid-configuration`) and use it for both values, so that they match the `--service-account-issuer` of the cluster. If detection is needed and the issuer cannot be read, the operator SHALL retry with backoff and SHALL NOT fall back to a fixed value. The method SHALL NOT be updated on start when the resulting configuration is unchanged.

> **Not yet implemented** (task 20.6): `BoundIssuer` and `BoundAudiences` are hard-coded to `https://kubernetes.default.svc.cluster.local` and the method is updated on every start. On clusters with a different issuer all logins through this method are rejected.

#### Scenario: Auth method created on first start

- **WHEN** the operator starts and Consul has no auth method `applications-k8s-m2m`
- **THEN** the operator SHALL create it with type `jwt`, the configured `JWKS_URL` and the claim mappings above

#### Scenario: Consul unavailable at start

- **WHEN** the operator starts and Consul cannot be reached
- **THEN** the operator SHALL keep retrying with increasing backoff and SHALL NOT crash

---

### Requirement: Binding-Rule Selector from Claim Mappings

The selector of a binding rule generated from `ServiceAccountName` SHALL match the claims exposed by the auth method that the rule belongs to.

- For a `jwt` method: `value.namespace == "<crNamespace>" and value.serviceaccount == "<ServiceAccountName>"`.
- For a `kubernetes` method: `serviceaccount.namespace == "<crNamespace>" and serviceaccount.name == "<ServiceAccountName>"`.

> **Not yet implemented** (task 20.5): the operator always generates the `value.*` form. A rule with a per-rule `AuthMethod` of type `kubernetes` therefore never matches a login and the service receives a token without roles.

#### Scenario: Selector for the global JWT method

- **WHEN** a binding-rule entry with `ServiceAccountName: "my-sa"` is reconciled for a CR in namespace `staging` under `applications-k8s-m2m`
- **THEN** the rule selector SHALL be `value.namespace == "staging" and value.serviceaccount == "my-sa"`

#### Scenario: Selector for a per-rule kubernetes method

- **WHEN** a binding-rule entry specifies `AuthMethod` of type `kubernetes` and `ServiceAccountName: "my-sa"`
- **THEN** the rule selector SHALL use `serviceaccount.namespace` and `serviceaccount.name`

---

### Requirement: Upgrade From the Kubernetes Auth Method

Binding rules created by earlier versions under `{fullname}-k8s-auth-method` SHALL be treated as outside the scope of the operator after the upgrade: the operator SHALL NOT update or delete them. The upgrade procedure SHALL be documented, including that client services must log in through `applications-k8s-m2m` and that the old rules must be removed manually.

#### Scenario: CR changed after upgrade

- **WHEN** a CR with an existing rule under the old auth method gets a new role after the operator is upgraded
- **THEN** the operator SHALL create or update the rule only under `applications-k8s-m2m`, and the rule under the old method SHALL remain unchanged

---

### Requirement: JWKS Proxy

The Helm chart SHALL deploy a read-only proxy that exposes only `/openid/v1/jwks` and `/.well-known/openid-configuration` of the Kubernetes API server, under a dedicated ServiceAccount without additional RBAC. The proxy SHALL serve these two paths (`--accept-paths`). All methods other than `GET` SHALL be rejected.

The proxy SHOULD run with more than one replica and a PodDisruptionBudget and SHOULD support the same scheduling and labelling settings as the other components.

> **Not yet implemented** (task 20.7): the deployment has `replicas: 1`, no PodDisruptionBudget, and no affinity, tolerations, nodeSelector, priorityClassName or extra labels.

#### Scenario: JWKS is served

- **WHEN** a Consul server requests `/openid/v1/jwks` from the proxy
- **THEN** the proxy SHALL return the API server response

#### Scenario: Other path is rejected

- **WHEN** a client requests `/api/v1/secrets` from the proxy
- **THEN** the proxy SHALL reject the request

---

### Requirement: Shared Ownership of Explicit Policies

With `spec.acl.explicitName: true` the operator SHALL record the namespace of each owning CR in the owner list of the policy description. The operator SHALL delete a policy only when the CR being deleted or updated is its last owner; otherwise it SHALL remove only the namespace from the owner list. On update, policies that were applied earlier and are absent from the new spec SHALL be released the same way.

#### Scenario: Second CR adds itself as owner

- **WHEN** a CR in `ns2` declares an explicit policy that already exists with `[consul-acl-owners: ns1]`
- **THEN** the operator SHALL update the policy and set the owner list to `ns1, ns2`

#### Scenario: Deleting one of two owners keeps the policy

- **WHEN** the CR in `ns1` is deleted and the policy has owners `ns1, ns2`
- **THEN** the operator SHALL keep the policy and set the owner list to `ns2`

#### Scenario: Deleting the last owner deletes the policy

- **WHEN** the CR in `ns2` is deleted and it is the only remaining owner
- **THEN** the operator SHALL delete the policy

> **Known limitation** (task 21.4): stale policies are identified by parsing the previous `status.policiesStatus` string, which is lost on restore from backup.

---

### Requirement: Shared Ownership of Explicit Roles and Binding Rules

With `spec.acl.explicitName: true` a role or binding rule used by several CRs SHALL NOT be deleted, and the tokens of the role SHALL NOT be revoked, while another CR still declares it. Roles and binding rules removed from a CR spec SHALL be cleaned up in explicit mode under the same rule. Owners SHALL be recorded in the `Description` of the role or binding rule as `[consul-acl-owners: ns1, ns2]` (namespaces only, the same format as for policies); the entity SHALL be deleted, and the tokens of a role revoked, only when the last owner is removed. Entities without the marker SHALL be handled as before.

> **Not yet implemented** (tasks 7.2, 21.5): there is no owner tracking for roles and binding rules. Deleting one CR deletes a shared role and rule and revokes all tokens of the role, including those used through other CRs, and stale cleanup of roles and rules is disabled in explicit mode, so entries removed from a spec stay in Consul.

#### Scenario: Shared role survives deletion of one CR

- **WHEN** two CRs declare the role `shared-reader` with `explicitName: true` and one CR is deleted
- **THEN** the role, its binding rule and the tokens of the role SHALL remain until the other CR is also deleted

---

### Requirement: Network Error Is Propagated From Binding-Rule Processing

A network error returned by `BindingRuleList`, `BindingRuleCreate` or `BindingRuleUpdate` SHALL be returned from `processBindRules`, so that the reconcile ends with `Successful=False` and is requeued after `RECONCILE_PERIOD_SECONDS`. Other errors SHALL be recorded in `status.bindRulesStatus` and SHALL NOT stop processing of the remaining rules.

> **Bug** (task 21.3, bugfix): a variable declared with `:=` inside the loop shadows the outer `err`, so an error from `BindingRuleCreate` or `BindingRuleUpdate` is only written to the status string and the function returns `nil`. The condition becomes `Successful=True` and no requeue happens.

#### Scenario: Consul unavailable while creating a rule

- **WHEN** `BindingRuleCreate` fails with a network error
- **THEN** `processBindRules` SHALL return that error and the reconcile SHALL be requeued

---

### Requirement: Single Active Operator

At most one operator instance SHALL reconcile ConsulACL and ConsulKV resources at a time, including during a rolling update.

> **Not yet implemented** (task 23.1): the ClusterRole already allows `coordination.k8s.io/leases`, but `--leader-elect` is not passed to the operator and defaults to `false`, and the deployment uses `RollingUpdate`.

#### Scenario: Upgrade of the operator

- **WHEN** the operator deployment is updated and the new pod starts before the old pod stops
- **THEN** only one of them SHALL run reconciliation