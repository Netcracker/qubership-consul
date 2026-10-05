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

#### Scenario: Issuer detected from the cluster

- **WHEN** `boundIssuer` is not set and the OpenID configuration of the cluster has `issuer: https://oidc.example`
- **THEN** the operator SHALL set `BoundIssuer` to `https://oidc.example` and, if `boundAudiences` is not set, `BoundAudiences` to `["https://oidc.example"]`

#### Scenario: Explicit issuer and audiences

- **WHEN** `boundIssuer` and `boundAudiences` are set in the deployment parameters
- **THEN** the operator SHALL use them and SHALL NOT request the OpenID configuration

#### Scenario: Issuer cannot be read

- **WHEN** `boundIssuer` is not set and the JWKS proxy does not return the OpenID configuration
- **THEN** the operator SHALL NOT create or update the method and SHALL retry with backoff

#### Scenario: Configuration unchanged

- **WHEN** the operator starts and the method in Consul already has the desired configuration
- **THEN** the operator SHALL NOT update the method

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

- For any other type, or when the auth method does not exist yet, the `jwt` form is used.

An explicit `Selector` in the binding-rule entry SHALL be passed to Consul unchanged.

#### Scenario: Selector for the global JWT method

- **WHEN** a binding-rule entry with `ServiceAccountName: "my-sa"` is reconciled for a CR in namespace `staging` under `applications-k8s-m2m`
- **THEN** the rule selector SHALL be `value.namespace == "staging" and value.serviceaccount == "my-sa"`

#### Scenario: Selector for a per-rule kubernetes method

- **WHEN** a binding-rule entry specifies `AuthMethod` of type `kubernetes` and `ServiceAccountName: "my-sa"`
- **THEN** the rule selector SHALL use `serviceaccount.namespace` and `serviceaccount.name`

---

### Requirement: Upgrade From the Kubernetes Auth Method

The global auth method of binding rules SHALL be configurable (`consulAclConfigurator.authMethod`, default `applications-k8s-m2m`). The global auth methods of earlier versions SHALL be configurable as legacy methods (`consulAclConfigurator.legacyAuthMethods`, empty by default, so the rules under the old method are kept until the cleanup is enabled).

On every reconcile and on deletion of a CR the operator SHALL delete the rules of the CR left under each existing legacy method: `BindType` `role` and a `BindName` with the prefix `{crName}_{crNamespace}_`. On reconcile, a rule that the CR declares with that legacy method as its per-rule `AuthMethod` SHALL be kept. A legacy method equal to the current global method SHALL be ignored, and a legacy method that does not exist in Consul SHALL be skipped. Rules with another `BindType` or name (for example the `service` rules of `server-acl-init`) SHALL NOT be touched. The upgrade procedure SHALL be documented, including that client services must log in through the current global method.

#### Scenario: CR reconciled after upgrade

- **WHEN** a CR `my-service` in `ns` has the rule `my-service_ns_reader` under `consul-k8s-auth-method` from an earlier version, `legacyAuthMethods` is `[consul-k8s-auth-method]` and the operator reconciles it with the global method `applications-k8s-m2m`
- **THEN** the operator SHALL create the rule under `applications-k8s-m2m` and delete the rule under `consul-k8s-auth-method`

#### Scenario: Old rules kept by default

- **WHEN** `legacyAuthMethods` is not set
- **THEN** the operator SHALL NOT delete rules under `consul-k8s-auth-method`

#### Scenario: Service mesh rules untouched

- **WHEN** the legacy method has a rule with `BindType` `service` created by `server-acl-init`
- **THEN** the operator SHALL NOT delete it

---

### Requirement: JWKS Proxy

The Helm chart SHALL deploy a read-only proxy that exposes only `/openid/v1/jwks` and `/.well-known/openid-configuration` of the Kubernetes API server, under a dedicated ServiceAccount without additional RBAC. The proxy SHALL serve exactly these two paths (`--accept-paths` anchored at both ends). All methods other than `GET` SHALL be rejected.

The proxy SHALL run with 2 replicas by default, spread across nodes with a preferred anti-affinity, and with a PodDisruptionBudget when there is more than one replica. Replicas, resources, affinity, tolerations, nodeSelector, priorityClassName and extra labels SHALL be configurable in `consulAclConfigurator.jwksProxy`. An optional NetworkPolicy, disabled by default, SHALL allow ingress only from the Consul server pods of the release and from the operator.

#### Scenario: Proxy survives a node drain

- **WHEN** a node with one of the two proxy pods is drained
- **THEN** the PodDisruptionBudget SHALL keep the other pod running

#### Scenario: Path with a valid prefix is rejected

- **WHEN** a client requests `/openid/v1/jwks/extra` from the proxy
- **THEN** the proxy SHALL reject the request

#### Scenario: JWKS is served

- **WHEN** a Consul server requests `/openid/v1/jwks` from the proxy
- **THEN** the proxy SHALL return the API server response

#### Scenario: Other path is rejected

- **WHEN** a client requests `/api/v1/secrets` from the proxy
- **THEN** the proxy SHALL reject the request

---

### Requirement: Shared Ownership of Explicit Policies

With `spec.acl.explicitName: true` the operator SHALL record the namespace of each owning CR in the owner list of the policy description. The operator SHALL delete a policy only when the CR being deleted or updated is its last owner; otherwise it SHALL remove only the namespace from the owner list. On update, policies that were applied earlier and are absent from the new spec SHALL be released the same way.

Policies applied earlier SHALL be identified by the owner list in Consul, not by the CR status: a policy is released on update when the namespace of the CR is in its owner list and neither the CR nor another ConsulACL resource of the same namespace managed by the operator declares it. A policy without the owner marker SHALL NOT be released on update.

The operator SHALL NOT remove a namespace from an owner list while another ConsulACL resource of that namespace managed by the operator, and not being deleted, still declares the entity. If the configuration of such a resource can not be parsed, the operator SHALL fail the operation and retry it.

#### Scenario: Second CR adds itself as owner

- **WHEN** a CR in `ns2` declares an explicit policy that already exists with `[consul-acl-owners: ns1]`
- **THEN** the operator SHALL update the policy and set the owner list to `ns1, ns2`

#### Scenario: Deleting one of two owners keeps the policy

- **WHEN** the CR in `ns1` is deleted and the policy has owners `ns1, ns2`
- **THEN** the operator SHALL keep the policy and set the owner list to `ns2`

#### Scenario: Deleting the last owner deletes the policy

- **WHEN** the CR in `ns2` is deleted and it is the only remaining owner
- **THEN** the operator SHALL delete the policy

#### Scenario: Stale policy found after restore from backup

- **WHEN** the CR status was lost and the CR in `ns1` no longer declares a policy whose owner list contains `ns1`
- **THEN** the operator SHALL release the policy on the next reconcile

#### Scenario: Policy declared by another CR of the same namespace

- **WHEN** two CRs in `ns1` declare the explicit policy `shared-policy` and one of them is deleted
- **THEN** the operator SHALL keep the policy and its owner list unchanged

---

### Requirement: Shared Ownership of Explicit Roles and Binding Rules

With `spec.acl.explicitName: true` a role or binding rule used by several CRs SHALL NOT be deleted, and the tokens of the role SHALL NOT be revoked, while another CR still declares it. Roles and binding rules removed from a CR spec SHALL be cleaned up in explicit mode under the same rule. Owners SHALL be recorded in the `Description` of the role or binding rule as `[consul-acl-owners: ns1, ns2]` (namespaces only, the same format as for policies); the entity SHALL be deleted, and the tokens of a role revoked, only when the last owner is removed. Entities without the marker SHALL be handled as before.

Stale roles and binding rules SHALL be identified in the same way as stale policies (owner list in Consul, other resources of the namespace taken into account).

#### Scenario: Shared role survives deletion of one CR

- **WHEN** two CRs declare the role `shared-reader` with `explicitName: true` and one CR is deleted
- **THEN** the role, its binding rule and the tokens of the role SHALL remain until the other CR is also deleted

#### Scenario: Role removed from the spec of its last owner

- **WHEN** the role `old-reader` with owner list `ns1` is removed from the spec of the CR in `ns1`
- **THEN** the operator SHALL revoke the tokens of the role and delete the role

#### Scenario: Role created before the upgrade

- **WHEN** a CR declaring a role whose description has no owner marker is deleted
- **THEN** the operator SHALL delete the role and revoke its tokens as before

---

### Requirement: Network Error Is Propagated From Binding-Rule Processing

A network error returned by `BindingRuleList`, `BindingRuleCreate` or `BindingRuleUpdate` SHALL be returned from `processBindRules`, so that the reconcile ends with `Successful=False` and is requeued after `RECONCILE_PERIOD_SECONDS`. Other errors SHALL be recorded in `status.bindRulesStatus` and SHALL NOT stop processing of the remaining rules.

The same SHALL hold for policies and roles. A network error of any entity SHALL be returned even if the calls for later entities of the same type succeed.

#### Scenario: Consul unavailable while creating a rule

- **WHEN** `BindingRuleCreate` fails with a network error
- **THEN** `processBindRules` SHALL return that error and the reconcile SHALL be requeued

#### Scenario: Network error followed by a successful call

- **WHEN** the create call for the first role fails with a network error and the call for the second role succeeds
- **THEN** `processRoles` SHALL return the network error

---

### Requirement: Single Active Operator

At most one operator instance SHALL reconcile ConsulACL and ConsulKV resources at a time, including during a rolling update.

The operator SHALL run with leader election (`--leader-elect`, a Lease in its own namespace). The reconcilers and the creation of the global auth method SHALL run only in the leader. The leader SHALL release the Lease on shutdown, so that the new pod of a rolling update takes over without waiting for the Lease to expire.

#### Scenario: Upgrade of the operator

- **WHEN** the operator deployment is updated and the new pod starts before the old pod stops
- **THEN** only one of them SHALL run reconciliation

#### Scenario: Auth method written by the leader only

- **WHEN** two operator pods run at the same time
- **THEN** only the pod holding the Lease SHALL create or update `applications-k8s-m2m`