# Specification: consul-kv

## Overview

Introduces the `ConsulKV` custom resource for declarative management of Consul KV entries. Each resource declares a list of key-value entries under `spec.kv.entries`; the controller writes each entry to the Consul KV store on apply and deletes each entry on removal.

The controller follows the same reconciliation patterns as the existing `ConsulACLReconciler`: finalizer on creation, apply on every generation change, delete on `DeletionTimestamp`. It reuses the same Consul client configuration (host, port, scheme, TLS, and token).

---

## Terminology

- **ConsulKV** — the Kubernetes custom resource that declares one or more desired Consul KV entries.
- **entry** — a single `{key, value}` pair in `spec.kv.entries`.
- **key** — the Consul KV store path for an entry, specified as `spec.kv.entries[*].key`. Used verbatim; no prefix is added.
- **value** — the optional string value for an entry, specified as `spec.kv.entries[*].value`. May be absent or an empty string.
- **finalizer** — a controller-owned string added to `metadata.finalizers` to ensure the controller deletes all Consul KV entries before the Kubernetes resource is removed.

---

## ADDED Requirements

### Requirement: Resource Structure

A `ConsulKV` resource SHALL declare its desired KV entries under `spec.kv.entries` as a list. Each entry SHALL have a `key` field. The `value` field is optional and MAY be absent or an empty string.

The resource type structure is:

```go
type ConsulKVEntry struct {
    Key   string `json:"key"`
    Value string `json:"value,omitempty"`
}

type ConsulKVConfig struct {
    Entries           []ConsulKVEntry `json:"entries"`
    PurgeOnDelete     bool            `json:"purgeOnDelete,omitempty"`
    OperatorNamespace string          `json:"operatorNamespace,omitempty"`
}

type ConsulKVSpec struct {
    KV ConsulKVConfig `json:"kv"`
}
```

#### Scenario: Valid ConsulKV resource with multiple entries accepted

- **WHEN** a `ConsulKV` resource is created with `spec.kv.entries` containing one or more entries, each with a non-empty `key`
- **THEN** the controller SHALL proceed to reconcile the resource and write all declared entries to the Consul KV store

#### Scenario: Entry with no value is accepted

- **WHEN** a `ConsulKV` resource includes an entry with a non-empty `key` and no `value` field
- **THEN** the controller SHALL write the key to the Consul KV store with an empty value

---

### Requirement: Per-Key Status

A `ConsulKV` resource SHALL expose a `.status` field that records the outcome for each individual key after each reconcile. The controller SHALL update this status after each reconcile cycle using optimistic-locking retry consistent with the `CustomResourceUpdater` pattern.

> **TODO — ArgoCD status compatibility**: The Jira requires that CR statuses "should be recognized and supported by ArgoCD." The specific `.status` field structure, health-check annotations, or CRD schema conventions required for ArgoCD compatibility are not specified. Confirm the required status shape before finalizing the CRD schema.

#### Scenario: Status records outcome per key

- **WHEN** the controller reconciles a `ConsulKV` resource with three entries
- **THEN** the controller SHALL write a status entry for each of the three keys indicating the outcome of the KV write for that key

#### Scenario: Conflicting status update is retried

- **WHEN** the controller attempts to update per-key status and receives a conflict error from the Kubernetes API
- **THEN** the controller SHALL re-fetch the `ConsulKV` resource and retry the status update

---

### Requirement: Finalizer on Creation

When a `ConsulKV` resource is created and does not yet carry the controller's finalizer, the controller SHALL add the finalizer before performing any Consul API calls.

#### Scenario: Finalizer added on first reconcile

- **WHEN** a `ConsulKV` resource is created and its `metadata.finalizers` does not contain the controller's finalizer
- **THEN** the controller SHALL add the finalizer to the resource and persist the update before writing any entries to the Consul KV store

---

### Requirement: Finalizer Removed After Cleanup

The controller SHALL remove its finalizer from a `ConsulKV` resource only after all Consul KV entries declared in `spec.kv.entries` have been successfully deleted or confirmed absent.

#### Scenario: Finalizer removed after all KV entries deleted

- **WHEN** all Consul KV entries for a `ConsulKV` resource have been deleted
- **THEN** the controller SHALL remove its finalizer, allowing Kubernetes to complete the resource deletion

---

### Requirement: KVPut per Entry on Apply

When a `ConsulKV` resource is reconciled and carries no `DeletionTimestamp`, the controller SHALL call KVPut for each entry in `spec.kv.entries`, writing each entry's value to its key in the Consul KV store.

#### Scenario: All declared entries written to Consul on apply

- **WHEN** a `ConsulKV` resource is first reconciled with three entries in `spec.kv.entries`
- **THEN** the controller SHALL call KVPut for each of the three entries, creating or overwriting each key in the Consul KV store

#### Scenario: Entry with empty value written as empty key

- **WHEN** a `ConsulKV` resource is reconciled and an entry has `key: "config/app/feature"` with no `value`
- **THEN** the controller SHALL call KVPut for that key with an empty value

---

### Requirement: Verbatim Keys — No Automatic Prefix

The controller SHALL use each `key` exactly as declared in `spec.kv.entries`. The controller SHALL NOT prepend the CR name, CR namespace, or any other automatic prefix to any key.

#### Scenario: Key written verbatim

- **WHEN** a `ConsulKV` resource with name `myapp` in namespace `staging` is reconciled and an entry has `key: "config/staging/myapp/setting"`
- **THEN** the controller SHALL write the entry to the Consul KV path `config/staging/myapp/setting` with no modification

#### Scenario: Automatic prefix not applied

- **WHEN** a `ConsulKV` resource with name `myapp` in namespace `staging` is reconciled and an entry has `key: "logging/app/LOG_LEVEL"`
- **THEN** the controller SHALL NOT write to `myapp_staging_logging/app/LOG_LEVEL` or any prefixed variant; it SHALL write only to `logging/app/LOG_LEVEL`

---

### Requirement: Idempotent KVPut

The controller's KVPut operation SHALL be idempotent. Re-applying a `ConsulKV` resource whose entries already exist in Consul with the same values SHALL succeed without error.

#### Scenario: Re-apply of unchanged resource succeeds

- **WHEN** a `ConsulKV` resource is reconciled and the Consul KV store already contains all declared entries with identical values
- **THEN** the controller SHALL call KVPut for each entry without error and update the per-key status to reflect success

---

### Requirement: Apply on Generation Change

The controller SHALL call KVPut for all entries in `spec.kv.entries` on every change to `spec` that increments `metadata.generation`. It SHALL NOT write to Consul when only `status` fields change.

#### Scenario: Updated entry value written to Consul

- **WHEN** a `ConsulKV` resource's `spec.kv.entries` is modified, causing `metadata.generation` to increment
- **THEN** the controller SHALL call KVPut for all current entries, writing the updated values to the Consul KV store

#### Scenario: Status-only update does not trigger KV write

- **WHEN** the controller writes a status update to a `ConsulKV` resource and no spec fields change
- **THEN** the controller SHALL NOT perform additional KVPut calls

---

### Requirement: KVDelete per Entry on Delete

When a `ConsulKV` resource has a non-zero `DeletionTimestamp` and the controller's finalizer is present, the controller SHALL call KVDelete for each entry in `spec.kv.entries` before removing the finalizer.

#### Scenario: All declared entries deleted from Consul on resource deletion

- **WHEN** a `ConsulKV` resource is deleted by a user and contains three entries in `spec.kv.entries`
- **THEN** the controller SHALL call KVDelete for each of the three keys, removing them from the Consul KV store, and then remove its finalizer from the resource

---

### Requirement: Absent KV Entry Does Not Block Deletion

If a Consul KV entry for a declared key does not exist when the controller attempts to delete it, the controller SHALL treat this as a success for that key and continue processing remaining entries.

#### Scenario: Deletion succeeds when a KV entry is already absent

- **WHEN** a `ConsulKV` resource is deleted and one of its declared keys does not exist in the Consul KV store
- **THEN** the controller SHALL proceed without error, delete any remaining keys that do exist, and remove its finalizer

---

### Requirement: Generation-Based Reconcile Trigger

The controller SHALL trigger reconciliation only when `metadata.generation` changes. It SHALL NOT reconcile in response to status-subresource updates.

#### Scenario: Generation change triggers reconcile

- **WHEN** a `ConsulKV` resource's spec is modified and `metadata.generation` increments
- **THEN** the controller SHALL start a reconcile cycle for that resource

#### Scenario: Status update does not trigger redundant reconcile

- **WHEN** the controller updates the per-key status on a `ConsulKV` resource
- **THEN** the controller SHALL NOT trigger an additional reconcile cycle for that update

---

### Requirement: Not-Found Does Not Requeue

When a reconcile request arrives for a `ConsulKV` resource that no longer exists in Kubernetes, the controller SHALL return without error and SHALL NOT requeue.

#### Scenario: Deleted resource does not cause reconcile error

- **WHEN** a reconcile request is processed for a `ConsulKV` resource that has already been removed from Kubernetes
- **THEN** the controller SHALL return successfully without requeueing

---

### Requirement: Network Error Causes Requeue

When a Consul API call fails due to a network error, the controller SHALL requeue the reconcile request after the configured `RECONCILE_PERIOD_SECONDS` interval.

#### Scenario: Network error during KVPut causes requeue

- **WHEN** the controller encounters a network error while calling KVPut for an entry
- **THEN** the controller SHALL log the error and requeue the reconcile request after `RECONCILE_PERIOD_SECONDS` seconds

---

### Requirement: Status Update Failure Causes Requeue

When all KVPut calls succeed but the subsequent Kubernetes status update fails after exhausting retries, the controller SHALL requeue the reconcile request after the configured `RECONCILE_PERIOD_SECONDS` interval.

#### Scenario: Status update failure causes requeue

- **WHEN** all KVPut calls succeed but the controller fails to update the per-key status after exhausting retries
- **THEN** the controller SHALL log the error and requeue the reconcile request after `RECONCILE_PERIOD_SECONDS` seconds

---

### Requirement: Helm Chart Packages the ConsulKV CRD

The `ConsulKV` CRD SHALL be packaged and installed by the Helm chart alongside the existing `ConsulACL` CRD.

#### Scenario: ConsulKV CRD installed on helm install

- **WHEN** the Helm chart is installed
- **THEN** the `ConsulKV` CRD SHALL be present in the cluster

---

### Requirement: Operator RBAC Covers consulkvs Resources

The operator's ClusterRole SHALL grant the necessary permissions on `consulkvs`, `consulkvs/status`, and `consulkvs/finalizers` resources so the controller can reconcile `ConsulKV` resources.

#### Scenario: Operator can get, list, watch, create, update, patch, and delete ConsulKV resources

- **WHEN** the Helm chart is installed and the operator ClusterRole is applied
- **THEN** the operator's service account SHALL have get, list, watch, create, update, patch, and delete permissions on `consulkvs` resources in the configured API group

---

### Requirement: Entry Key Must Not Be Empty

An entry in `spec.kv.entries` with an empty `key` SHALL be skipped during reconciliation. The controller SHALL record the validation failure in `.status` for that entry and SHALL continue processing remaining entries.

#### Scenario: Entry with empty key is skipped

- **WHEN** a `ConsulKV` resource is reconciled and one entry in `spec.kv.entries` has an empty `key` field
- **THEN** the controller SHALL skip that entry, record an error in `.status` for it, and continue calling KVPut for all other valid entries

---

### Requirement: Key Ownership

The controller SHALL track ownership of each key in the Consul `Flags` field as a reference counter.

- A key that does not exist SHALL be created with `Flags=1`; the CR owns it.
- A key that exists with `Flags=0` (created outside the operator) SHALL have its value written, `Flags` SHALL stay `0`, and the CR SHALL NOT own it (status `synced (not owned: pre-existing key)`).
- A key that exists with `Flags>0` and is not yet owned by this CR SHALL have `Flags` incremented.
- A key already owned by this CR SHALL have only its value updated.
- On deletion of the CR, or when a key is removed from the spec, the controller SHALL decrement `Flags` for each owned key and SHALL delete the key when `Flags<=1`. Keys that are not owned SHALL be left in Consul.

> This refines "KVDelete per Entry on Delete" and "Finalizer Removed After Cleanup": on deletion the controller releases owned keys instead of deleting every declared key unconditionally.

#### Scenario: Key shared by two CRs survives deletion of one

- **WHEN** two `ConsulKV` resources declare `config/shared/url` and one is deleted
- **THEN** the key SHALL remain in Consul with `Flags=1` until the other resource is deleted

#### Scenario: Externally created key is not deleted

- **WHEN** a key created outside the operator is declared in a `ConsulKV` and the resource is deleted
- **THEN** the key SHALL remain in Consul

---

### Requirement: Batched Transactions

The controller SHALL write and delete keys in Consul transactions of at most 64 operations using check-and-set, retrying a batch on a CAS conflict. A batch is atomic; the operation as a whole is not.

When a later batch fails after earlier batches committed, the controller SHALL record the outcome of the committed batches in `status` so that a retry neither increments `Flags` a second time for keys already written, nor decrements it a second time for keys already released.

> **Bug** (task 22.6, bugfix): on a failure all entries are given an `error` status without `Owned=true`, although earlier batches are committed, so the next reconcile increments `Flags` again and the key is never deleted. When removing keys, a failed later batch returns no result, so the next reconcile decrements the already released keys again and may delete a key still used by another CR.

#### Scenario: Second batch fails on apply

- **WHEN** a CR with 100 keys is applied and the batch with keys 65–100 fails after the batch with keys 1–64 committed
- **THEN** keys 1–64 SHALL be recorded as owned in `status` and the next reconcile SHALL NOT increment their `Flags`

---

### Requirement: Duplicate Keys Are Rejected

A `ConsulKV` resource SHALL NOT contain the same `key` twice. The controller SHALL report `duplicate key "<key>"` in `status` and SHALL NOT include the duplicate in the transaction. The CRD SHOULD reject such a manifest with a validation rule.

> **Bug** (task 22.5, bugfix): a duplicate produces two `KVCAS` operations with the same `ModifyIndex` in one transaction, the second one fails, and every retry fails the same way until `max retries exceeded`, so none of the up to 64 keys in that batch is written.

#### Scenario: Same key declared twice

- **WHEN** `spec.kv.entries` contains `config/app/url` twice
- **THEN** the controller SHALL report a clear duplicate-key error and SHALL still write the other valid entries

---

### Requirement: Purge on Delete

When `spec.kv.purgeOnDelete` is `true`, the controller SHALL remove the declared keys on deletion of the CR with a recursive delete. Because Consul `recurse` matches a string prefix, a key used with `purgeOnDelete` SHALL end with `/`, and the controller SHALL delete a tree only for keys with `Owned=true` and `Flags<=1`. The controller SHALL NOT purge a prefix under which keys of other CRs exist.

> **Bug** (task 22.4, bugfix): there is no validation, and `deleteKVTree` deletes by prefix for every entry in `status`, including keys that are not owned and keys with `Flags>1`. With `key: "config/app"`, `config/application/...` and `config/app-gateway/...` are deleted too, and the reference counting is bypassed. There are no tests for `purgeOnDelete`.

#### Scenario: Prefix collision

- **WHEN** a CR with `purgeOnDelete: true` and `key: "config/app/"` is deleted and `config/application/x` exists
- **THEN** `config/application/x` SHALL remain in Consul

#### Scenario: Key without trailing slash

- **WHEN** a CR with `purgeOnDelete: true` declares `key: "config/app"`
- **THEN** the controller or the CRD validation SHALL reject it

---

### Requirement: Operator Namespace Binding

When `spec.kv.operatorNamespace` is set, the CR SHALL be reconciled only by the operator whose own namespace equals this value, which allows the CR to live in a different namespace. When it is empty, the CR SHALL be reconciled by the operator running in the CR's own namespace.

#### Scenario: CR bound to another operator

- **WHEN** a CR has `operatorNamespace: consul-b` and the operator runs in `consul-a`
- **THEN** the operator in `consul-a` SHALL ignore the CR