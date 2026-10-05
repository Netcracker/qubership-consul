// Copyright 2024-2025 NetCracker Technology Corporation
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package controllers

import (
	"context"
	"fmt"
	"strings"

	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/builder"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/event"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	"time"

	consulacl "github.com/Netcracker/consul-acl-configurator/consul-acl-configurator-operator/api/v1alpha1"
	"github.com/Netcracker/consul-acl-configurator/consul-acl-configurator-operator/util"
	consulApi "github.com/hashicorp/consul/api"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/util/retry"
)

var consulKVFinalizer = consulacl.GroupVersion.Group + "/consulkvconfigurator-controller"

var kvLog = logf.Log.WithName("controller_consulkv")

const casMaxRetries = 10

// txnBatchSize is the maximum number of KV operations per Consul transaction.
// Consul caps a transaction at 64 operations, so larger CRs are split into
// several transactions (each batch is applied atomically on its own).
const txnBatchSize = 64

var kvClient consulKVClient = makeKVClient()
var txnClient consulTxnClient = makeTxnClient()

// ConsulKVReconciler reconciles a ConsulKV object
type ConsulKVReconciler struct {
	Client       client.Client
	Scheme       *runtime.Scheme
	OwnNamespace string
}

//+kubebuilder:rbac:groups=netcracker.com,resources=consulkvs,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=netcracker.com,resources=consulkvs/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=netcracker.com,resources=consulkvs/finalizers,verbs=update

func (r *ConsulKVReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	reqLogger := kvLog.WithValues("Request.Namespace", req.Namespace, "Request.Name", req.Name)
	reqLogger.Info("Reconciling ConsulKV")

	instance := &consulacl.ConsulKV{}
	err := r.Client.Get(ctx, req.NamespacedName, instance)
	if err != nil {
		if errors.IsNotFound(err) {
			return reconcile.Result{}, nil
		}
		return reconcile.Result{}, err
	}

	crUpdater := newKVUpdater(r.Client, instance)

	if instance.DeletionTimestamp.IsZero() {
		if !util.Contains(consulKVFinalizer, instance.GetFinalizers()) {
			err = crUpdater.updateWithRetry(func(cr *consulacl.ConsulKV) {
				controllerutil.AddFinalizer(cr, consulKVFinalizer)
			})
			if err != nil {
				return reconcile.Result{}, err
			}
		}
	} else {
		if util.Contains(consulKVFinalizer, instance.GetFinalizers()) {
			var deleteErr error
			if instance.Spec.KV.PurgeOnDelete {
				deleteErr = deleteKVTree(instance.Status.Entries)
			} else {
				var released map[string]bool
				released, deleteErr = deleteKVEntries(instance.Status.Entries)
				if deleteErr != nil && len(released) > 0 {
					// Record the keys of the committed batches so that the retry does not decrement them again.
					if statusErr := crUpdater.updateStatusWithRetry(func(cr *consulacl.ConsulKV) {
						cr.Status.Entries = markReleased(cr.Status.Entries, released)
					}); statusErr != nil {
						kvLog.Error(statusErr, "Error updating ConsulKV status after partial release")
					}
				}
			}
			if deleteErr != nil {
				reqLogger.Error(deleteErr, "Error releasing ConsulKV entries")
				return reconcile.Result{RequeueAfter: time.Second * time.Duration(periodTime)}, nil
			}
			err = crUpdater.updateWithRetry(func(cr *consulacl.ConsulKV) {
				controllerutil.RemoveFinalizer(cr, consulKVFinalizer)
			})
			return reconcile.Result{}, err
		}
		return reconcile.Result{}, nil
	}

	ownedKeys := make(map[string]bool, len(instance.Status.Entries))
	for _, e := range instance.Status.Entries {
		if e.Owned {
			ownedKeys[e.Key] = true
		}
	}

	entryStatuses, applyErr := applyKVEntries(instance.Spec.KV.Entries, ownedKeys)
	cleanedKeys, removeErr := deleteRemovedEntries(instance.Status.Entries, instance.Spec.KV.Entries)

	reconcileErr := applyErr
	if reconcileErr == nil {
		reconcileErr = removeErr
	}

	statusErr := crUpdater.updateStatusWithRetry(func(cr *consulacl.ConsulKV) {
		cr.Status.Entries = mergeKVStatuses(cr.Status.Entries, entryStatuses, cleanedKeys)
		cr.Status.ManagedBy = "consul-acl-configurator-operator_" + r.OwnNamespace
		if applyErr != nil || removeErr != nil {
			cr.Status.GeneralStatus = "degraded"
		} else {
			cr.Status.GeneralStatus = "synced"
		}
		setSuccessfulCondition(&cr.Status.Conditions, reconcileErr, cr.Generation)
	})
	if statusErr != nil {
		kvLog.Error(statusErr, "Error updating ConsulKV status")
		return reconcile.Result{RequeueAfter: time.Second * time.Duration(periodTime)}, nil
	}

	if applyErr != nil || removeErr != nil {
		return reconcile.Result{RequeueAfter: time.Second * time.Duration(periodTime)}, nil
	}

	reqLogger.Info("Reconcile cycle succeeded")
	return reconcile.Result{}, nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *ConsulKVReconciler) SetupWithManager(mgr ctrl.Manager) error {
	statusPredicate := predicate.Funcs{
		UpdateFunc: func(e event.UpdateEvent) bool {
			return e.ObjectOld.GetGeneration() != e.ObjectNew.GetGeneration()
		},
		DeleteFunc: func(e event.DeleteEvent) bool {
			return !e.DeleteStateUnknown
		},
	}

	ownerPredicate := predicate.NewPredicateFuncs(func(obj client.Object) bool {
		cr, ok := obj.(*consulacl.ConsulKV)
		if !ok {
			return true
		}
		// When operatorNamespace is set, the CR is owned by the operator whose own
		// namespace matches it, regardless of the CR's own namespace. Otherwise fall
		// back to matching the CR's namespace against the operator's namespace.
		operatorNs := cr.Spec.KV.OperatorNamespace
		if operatorNs != "" {
			return operatorNs == r.OwnNamespace
		}
		return obj.GetNamespace() == r.OwnNamespace
	})

	return ctrl.NewControllerManagedBy(mgr).
		For(&consulacl.ConsulKV{}, builder.WithPredicates(statusPredicate, ownerPredicate)).
		Complete(r)
}

const statusSkippedDuplicate = "skipped (duplicate key)"

func applyKVEntries(entries []consulacl.ConsulKVEntry, ownedKeys map[string]bool) ([]consulacl.ConsulKVEntryStatus, error) {
	statuses := make([]consulacl.ConsulKVEntryStatus, len(entries))

	// The last entry of a key wins; two operations on one key would make the transaction fail.
	lastIndex := make(map[string]int, len(entries))
	for i, e := range entries {
		lastIndex[e.Key] = i
	}

	// Empty keys and earlier duplicates are reported per-entry and excluded from the transactions.
	valid := make([]consulacl.ConsulKVEntry, 0, len(entries))
	for i, e := range entries {
		switch {
		case e.Key == "":
			statuses[i] = consulacl.ConsulKVEntryStatus{Key: "", Status: "error: key must not be empty"}
		case lastIndex[e.Key] != i:
			statuses[i] = consulacl.ConsulKVEntryStatus{Key: e.Key, Status: statusSkippedDuplicate}
		default:
			valid = append(valid, e)
		}
	}

	owned, err := writeKVBatchWithOwnership(valid, ownedKeys)

	// Each batch is atomic, the whole write is not: keys of committed batches are synced even if
	// a later batch failed, so that the retry does not increment their Flags again.
	for i, e := range entries {
		if e.Key == "" || lastIndex[e.Key] != i {
			continue
		}
		isOwned, committed := owned[e.Key]
		if !committed {
			// Not written in this cycle; a key owned before stays owned. writeKVBatchWithOwnership
			// returns nil only when every entry is committed, the check keeps it safe if that changes.
			reason := "not written"
			if err != nil {
				reason = err.Error()
			}
			statuses[i] = consulacl.ConsulKVEntryStatus{Key: e.Key, Status: "error: " + reason, Owned: ownedKeys[e.Key]}
			continue
		}
		status := "synced"
		if !isOwned {
			status = "synced (not owned: pre-existing key)"
		}
		statuses[i] = consulacl.ConsulKVEntryStatus{Key: e.Key, Status: status, Owned: isOwned}
	}
	return statuses, err
}

// writeKVBatchWithOwnership applies entries in CAS transactions of up to txnBatchSize
// operations. It returns a key->owned map with an entry for each key of the committed batches,
// describing whether this CR now owns the key; on error the keys of the failed and later batches are absent.
func writeKVBatchWithOwnership(entries []consulacl.ConsulKVEntry, ownedKeys map[string]bool) (map[string]bool, error) {
	owned := make(map[string]bool, len(entries))
	for start := 0; start < len(entries); start += txnBatchSize {
		end := min(start+txnBatchSize, len(entries))
		if err := writeKVChunk(entries[start:end], ownedKeys, owned); err != nil {
			return owned, err
		}
	}
	return owned, nil
}

// writeKVChunk applies one batch (<= txnBatchSize) atomically via a KVCAS transaction,
// retrying the whole batch on a CAS conflict. Ownership is tracked with the Flags counter:
//   - key absent: created with Flags=1 (this CR becomes owner);
//   - key exists with Flags=0 (created externally): value is written, Flags stays 0, not owned;
//   - key already owned by this CR: value updated, Flags unchanged;
//   - key exists with Flags>0 (owned by another CR): Flags incremented (ref-count).
func writeKVChunk(entries []consulacl.ConsulKVEntry, ownedKeys, owned map[string]bool) error {
	for attempt := 0; attempt < casMaxRetries; attempt++ {
		ops := make(consulApi.TxnOps, len(entries))
		willOwn := make([]bool, len(entries))

		// First pass: read each key and compute its target Flags before building the txn,
		// because transaction operations cannot branch on values read inside the txn.
		for i, e := range entries {
			pair, _, err := kvClient.Get(e.Key, nil)
			if err != nil {
				return fmt.Errorf("KV get %q: %w", e.Key, err)
			}
			var flags, index uint64
			if pair != nil {
				flags, index = pair.Flags, pair.ModifyIndex
			}

			newFlags := flags
			switch {
			case ownedKeys[e.Key]:
				willOwn[i] = true // already owned — keep Flags as is
			case pair != nil && flags == 0:
				willOwn[i] = false // pre-existing external key — write value, do not own
			default:
				newFlags = flags + 1 // new owner (index==0 means create-if-absent)
				willOwn[i] = true
			}

			ops[i] = &consulApi.TxnOp{KV: &consulApi.KVTxnOp{
				Verb:  consulApi.KVCAS,
				Key:   e.Key,
				Value: []byte(e.Value),
				Flags: newFlags,
				Index: index,
			}}
		}

		ok, resp, _, err := txnClient.Txn(ops, nil)
		if err != nil {
			return fmt.Errorf("KV apply txn: %w", err)
		}
		if ok {
			// Record ownership only after the transaction actually committed.
			for i, e := range entries {
				owned[e.Key] = willOwn[i]
			}
			return nil
		}
		// ok=false: at least one key changed since it was read. Nothing was written; retry.
		for _, te := range resp.Errors {
			kvLog.Info("KV apply txn rejected, will retry", "opIndex", te.OpIndex, "what", te.What)
		}
	}
	return fmt.Errorf("KV apply txn: max retries exceeded (%d)", casMaxRetries)
}

// mergeKVStatuses preserves the order of existing status entries and appends new ones at the bottom.
// A key may have several updated entries (earlier duplicates are reported as skipped), they are kept together.
// cleanedKeys contains keys that were successfully decremented this reconcile cycle.
func mergeKVStatuses(existing, updated []consulacl.ConsulKVEntryStatus, cleanedKeys map[string]bool) []consulacl.ConsulKVEntryStatus {
	updatedByKey := make(map[string][]consulacl.ConsulKVEntryStatus, len(updated))
	for _, e := range updated {
		updatedByKey[e.Key] = append(updatedByKey[e.Key], e)
	}

	result := make([]consulacl.ConsulKVEntryStatus, 0, len(existing)+len(updated))
	seen := make(map[string]bool, len(existing))

	for _, e := range existing {
		if seen[e.Key] {
			continue
		}
		seen[e.Key] = true
		if entries, ok := updatedByKey[e.Key]; ok {
			result = append(result, entries...)
		} else {
			// key removed from spec: owned=false only if decrement succeeded
			owned := e.Owned && !cleanedKeys[e.Key]
			result = append(result, consulacl.ConsulKVEntryStatus{Key: e.Key, Status: "removed", Owned: owned})
		}
	}

	for _, e := range updated {
		if !seen[e.Key] {
			seen[e.Key] = true
			result = append(result, updatedByKey[e.Key]...)
		}
	}

	return result
}

// ownedStatusKeys returns the distinct non-empty keys owned according to the status entries.
func ownedStatusKeys(statuses []consulacl.ConsulKVEntryStatus) []string {
	seen := make(map[string]bool, len(statuses))
	var keys []string
	for _, e := range statuses {
		if e.Key != "" && e.Owned && !seen[e.Key] {
			seen[e.Key] = true
			keys = append(keys, e.Key)
		}
	}
	return keys
}

// deleteKVEntries is called on CR deletion. Decrements Flags for every owned entry,
// deleting a key once its counter reaches zero. It returns the keys released by the
// committed batches, also when a later batch fails.
func deleteKVEntries(statuses []consulacl.ConsulKVEntryStatus) (map[string]bool, error) {
	return deleteKVBatch(ownedStatusKeys(statuses))
}

// deleteKVBatch releases the given keys in CAS transactions of up to txnBatchSize
// operations. Each batch is atomic, the whole operation is not: the returned set contains
// the keys of the batches that committed before an error.
func deleteKVBatch(keys []string) (map[string]bool, error) {
	released := make(map[string]bool, len(keys))
	for start := 0; start < len(keys); start += txnBatchSize {
		end := min(start+txnBatchSize, len(keys))
		if err := deleteKVChunk(keys[start:end]); err != nil {
			return released, err
		}
		for _, k := range keys[start:end] {
			released[k] = true
		}
	}
	return released, nil
}

// deleteKVChunk applies one batch atomically: for each key it either decrements Flags
// (KVCAS) or, when Flags<=1, deletes the key (KVDeleteCAS). The whole batch is retried
// on a CAS conflict.
func deleteKVChunk(keys []string) error {
	for attempt := 0; attempt < casMaxRetries; attempt++ {
		ops := make(consulApi.TxnOps, 0, len(keys))

		// First pass: read each key and decide delete vs. decrement before building the txn.
		for _, key := range keys {
			pair, _, err := kvClient.Get(key, nil)
			if err != nil {
				return fmt.Errorf("KV get %q: %w", key, err)
			}
			if pair == nil {
				continue // already gone — idempotent
			}
			if pair.Flags <= 1 {
				// Last owner: delete the key only if it has not changed since the read.
				ops = append(ops, &consulApi.TxnOp{KV: &consulApi.KVTxnOp{
					Verb:  consulApi.KVDeleteCAS,
					Key:   key,
					Index: pair.ModifyIndex,
				}})
			} else {
				// Still used by other CRs: decrement the counter, preserving the value.
				ops = append(ops, &consulApi.TxnOp{KV: &consulApi.KVTxnOp{
					Verb:  consulApi.KVCAS,
					Key:   key,
					Value: pair.Value,
					Flags: pair.Flags - 1,
					Index: pair.ModifyIndex,
				}})
			}
		}

		if len(ops) == 0 {
			return nil // all keys already absent
		}

		ok, resp, _, err := txnClient.Txn(ops, nil)
		if err != nil {
			return fmt.Errorf("KV delete txn: %w", err)
		}
		if ok {
			return nil
		}
		// ok=false: a key changed since it was read. Nothing was written; retry.
		for _, te := range resp.Errors {
			kvLog.Info("KV delete txn rejected, will retry", "opIndex", te.OpIndex, "what", te.What)
		}
	}
	return fmt.Errorf("KV delete txn: max retries exceeded (%d)", casMaxRetries)
}

// deleteKVTree is called on CR deletion when PurgeOnDelete is set. Each declared key is
// treated as a directory: the exact key and the tree "<key>/" are deleted, bypassing ownership
// checks. Consul deletes a tree by raw string prefix, so the trailing "/" is required to keep
// keys that only share the prefix (config/application/... for config/app).
func deleteKVTree(statuses []consulacl.ConsulKVEntryStatus) error {
	var firstErr error
	keepFirst := func(err error) {
		if firstErr == nil {
			firstErr = err
		}
	}
	seen := make(map[string]bool, len(statuses))
	for _, e := range statuses {
		if e.Key == "" || seen[e.Key] {
			continue
		}
		seen[e.Key] = true
		treePrefix := e.Key
		if !strings.HasSuffix(treePrefix, "/") {
			if _, err := kvClient.Delete(e.Key, nil); err != nil {
				kvLog.Error(err, "Error deleting KV key", "key", e.Key)
				keepFirst(err)
				continue
			}
			treePrefix += "/"
		}
		if _, err := kvClient.DeleteTree(treePrefix, nil); err != nil {
			kvLog.Error(err, "Error deleting KV tree", "prefix", treePrefix)
			keepFirst(err)
		}
	}
	return firstErr
}

// deleteRemovedEntries decrements Flags for keys that were owned but removed from spec.
// Returns the set of keys that were released (for status update), including the keys of
// committed batches when a later batch fails.
func deleteRemovedEntries(existing []consulacl.ConsulKVEntryStatus, specEntries []consulacl.ConsulKVEntry) (map[string]bool, error) {
	specKeys := make(map[string]bool, len(specEntries))
	for _, e := range specEntries {
		specKeys[e.Key] = true
	}

	var removed []consulacl.ConsulKVEntryStatus
	for _, e := range existing {
		if !specKeys[e.Key] {
			removed = append(removed, e)
		}
	}
	return deleteKVBatch(ownedStatusKeys(removed))
}

// markReleased returns statuses in which the released keys are no longer owned.
func markReleased(statuses []consulacl.ConsulKVEntryStatus, released map[string]bool) []consulacl.ConsulKVEntryStatus {
	result := make([]consulacl.ConsulKVEntryStatus, len(statuses))
	for i, e := range statuses {
		if released[e.Key] {
			e.Owned = false
			e.Status = "released"
		}
		result[i] = e
	}
	return result
}

// kvUpdater handles retried updates for ConsulKV resources.
type kvUpdater struct {
	client    client.Client
	name      string
	namespace string
}

func newKVUpdater(c client.Client, cr *consulacl.ConsulKV) kvUpdater {
	return kvUpdater{client: c, name: cr.Name, namespace: cr.Namespace}
}

func (u kvUpdater) updateWithRetry(fn func(*consulacl.ConsulKV)) error {
	return retry.RetryOnConflict(retry.DefaultRetry, func() error {
		cr := &consulacl.ConsulKV{}
		if err := u.client.Get(context.TODO(), types.NamespacedName{Name: u.name, Namespace: u.namespace}, cr); err != nil {
			return err
		}
		fn(cr)
		return u.client.Update(context.TODO(), cr)
	})
}

func (u kvUpdater) updateStatusWithRetry(fn func(*consulacl.ConsulKV)) error {
	return retry.RetryOnConflict(retry.DefaultRetry, func() error {
		cr := &consulacl.ConsulKV{}
		if err := u.client.Get(context.TODO(), types.NamespacedName{Name: u.name, Namespace: u.namespace}, cr); err != nil {
			return err
		}
		fn(cr)
		return u.client.Status().Update(context.TODO(), cr)
	})
}
