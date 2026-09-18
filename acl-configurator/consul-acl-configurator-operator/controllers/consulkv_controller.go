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
		if !containsFinalizer(instance.GetFinalizers(), consulKVFinalizer) {
			err = crUpdater.updateWithRetry(func(cr *consulacl.ConsulKV) {
				controllerutil.AddFinalizer(cr, consulKVFinalizer)
			})
			if err != nil {
				return reconcile.Result{}, err
			}
		}
	} else {
		if containsFinalizer(instance.GetFinalizers(), consulKVFinalizer) {
			var deleteErr error
			if instance.Spec.KV.PurgeOnDelete {
				deleteErr = deleteKVTree(instance.Status.Entries)
			} else {
				deleteErr = deleteKVEntries(instance.Status.Entries)
			}
			if deleteErr != nil {
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

func applyKVEntries(entries []consulacl.ConsulKVEntry, ownedKeys map[string]bool) ([]consulacl.ConsulKVEntryStatus, error) {
	statuses := make([]consulacl.ConsulKVEntryStatus, len(entries))

	// Empty keys are reported per-entry and excluded from the transaction batch.
	valid := make([]consulacl.ConsulKVEntry, 0, len(entries))
	for i, e := range entries {
		if e.Key == "" {
			statuses[i] = consulacl.ConsulKVEntryStatus{Key: "", Status: "error: key must not be empty"}
		} else {
			valid = append(valid, e)
		}
	}

	owned, err := writeKVBatchWithOwnership(valid, ownedKeys)

	// The batch is all-or-nothing, so every valid entry shares the outcome.
	for i, e := range entries {
		if e.Key == "" {
			continue
		}
		if err != nil {
			statuses[i] = consulacl.ConsulKVEntryStatus{Key: e.Key, Status: "error: " + err.Error()}
			continue
		}
		status := "synced"
		if !owned[e.Key] {
			status = "synced (not owned: pre-existing key)"
		}
		statuses[i] = consulacl.ConsulKVEntryStatus{Key: e.Key, Status: status, Owned: owned[e.Key]}
	}
	return statuses, err
}

// writeKVBatchWithOwnership applies entries in CAS transactions of up to txnBatchSize
// operations. It returns a key->owned map describing whether this CR now owns each key.
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
// cleanedKeys contains keys that were successfully decremented this reconcile cycle.
func mergeKVStatuses(existing, updated []consulacl.ConsulKVEntryStatus, cleanedKeys map[string]bool) []consulacl.ConsulKVEntryStatus {
	updatedMap := make(map[string]consulacl.ConsulKVEntryStatus, len(updated))
	for _, e := range updated {
		updatedMap[e.Key] = e
	}

	result := make([]consulacl.ConsulKVEntryStatus, 0, len(existing)+len(updated))
	seen := make(map[string]bool, len(existing))

	for _, e := range existing {
		seen[e.Key] = true
		if entry, ok := updatedMap[e.Key]; ok {
			result = append(result, entry)
		} else {
			// key removed from spec: owned=false only if decrement succeeded
			owned := e.Owned && !cleanedKeys[e.Key]
			result = append(result, consulacl.ConsulKVEntryStatus{Key: e.Key, Status: "removed", Owned: owned})
		}
	}

	for _, e := range updated {
		if !seen[e.Key] {
			result = append(result, e)
		}
	}

	return result
}

// deleteKVEntries is called on CR deletion. Decrements Flags for every owned entry,
// deleting a key once its counter reaches zero.
func deleteKVEntries(statuses []consulacl.ConsulKVEntryStatus) error {
	var keys []string
	for _, e := range statuses {
		if e.Key != "" && e.Owned {
			keys = append(keys, e.Key)
		}
	}
	return deleteKVBatch(keys)
}

// deleteKVBatch releases the given keys in CAS transactions of up to txnBatchSize
// operations (each batch is applied atomically on its own).
func deleteKVBatch(keys []string) error {
	for start := 0; start < len(keys); start += txnBatchSize {
		end := min(start+txnBatchSize, len(keys))
		if err := deleteKVChunk(keys[start:end]); err != nil {
			return err
		}
	}
	return nil
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

// deleteKVTree is called on CR deletion when PurgeOnDelete is set.
// It recursively deletes all Consul keys under each entry's key prefix, bypassing ownership checks.
func deleteKVTree(statuses []consulacl.ConsulKVEntryStatus) error {
	var firstErr error
	for _, e := range statuses {
		if e.Key == "" {
			continue
		}
		if _, err := kvClient.DeleteTree(e.Key, nil); err != nil {
			kvLog.Error(err, "Error deleting KV tree", "prefix", e.Key)
			if firstErr == nil {
				firstErr = err
			}
		}
	}
	return firstErr
}

// deleteRemovedEntries decrements Flags for keys that were owned but removed from spec.
// Returns the set of keys that were successfully released (for status update).
func deleteRemovedEntries(existing []consulacl.ConsulKVEntryStatus, specEntries []consulacl.ConsulKVEntry) (map[string]bool, error) {
	specKeys := make(map[string]bool, len(specEntries))
	for _, e := range specEntries {
		specKeys[e.Key] = true
	}

	var keys []string
	for _, e := range existing {
		if !specKeys[e.Key] && e.Owned {
			keys = append(keys, e.Key)
		}
	}

	if err := deleteKVBatch(keys); err != nil {
		return nil, err // batch is atomic: on failure nothing was released
	}

	cleanedKeys := make(map[string]bool, len(keys))
	for _, k := range keys {
		cleanedKeys[k] = true
	}
	return cleanedKeys, nil
}

func containsFinalizer(finalizers []string, finalizer string) bool {
	for _, f := range finalizers {
		if f == finalizer {
			return true
		}
	}
	return false
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
