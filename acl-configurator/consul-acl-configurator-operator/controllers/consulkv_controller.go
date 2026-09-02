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

var kvClient consulKVClient = makeKVClient()

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
		return obj.GetNamespace() == r.OwnNamespace
	})

	return ctrl.NewControllerManagedBy(mgr).
		For(&consulacl.ConsulKV{}, builder.WithPredicates(statusPredicate, ownerPredicate)).
		Complete(r)
}

func applyKVEntries(entries []consulacl.ConsulKVEntry, ownedKeys map[string]bool) ([]consulacl.ConsulKVEntryStatus, error) {
	statuses := make([]consulacl.ConsulKVEntryStatus, len(entries))
	var firstErr error

	for i, e := range entries {
		if e.Key == "" {
			statuses[i] = consulacl.ConsulKVEntryStatus{Key: "", Status: "error: key must not be empty"}
			continue
		}
		tookOwnership, err := writeKVWithOwnership(e.Key, e.Value, ownedKeys[e.Key])
		if err != nil {
			statuses[i] = consulacl.ConsulKVEntryStatus{Key: e.Key, Status: "error: " + err.Error()}
			if firstErr == nil {
				firstErr = err
			}
			continue
		}
		owned := ownedKeys[e.Key] || tookOwnership
		status := "synced"
		if !owned {
			status = "synced (not owned: pre-existing key)"
		}
		statuses[i] = consulacl.ConsulKVEntryStatus{Key: e.Key, Status: status, Owned: owned}
	}
	return statuses, firstErr
}

// writeKVWithOwnership atomically writes the KV value and manages ownership via the Flags counter.
// Ownership is tracked using Consul KV Flags as a reference counter:
//   - If the key does not exist: creates it with Flags=1, returns tookOwnership=true.
//   - If the key exists with Flags=0 (created externally): writes the value but keeps Flags=0,
//     returns tookOwnership=false. The key will NOT be deleted when the CR is removed.
//   - If the key is already owned (alreadyOwned=true): updates value without changing Flags.
//   - If the key exists with Flags>0 (owned by another CR): increments Flags (ref-count).
func writeKVWithOwnership(key, value string, alreadyOwned bool) (tookOwnership bool, err error) {
	for attempt := 0; attempt < casMaxRetries; attempt++ {
		pair, _, err := kvClient.Get(key, nil)
		if err != nil {
			return false, fmt.Errorf("KV get %q: %w", key, err)
		}

		var currentFlags uint64
		var modifyIndex uint64
		if pair != nil {
			currentFlags = pair.Flags
			modifyIndex = pair.ModifyIndex
		}

		// Do not take ownership of pre-existing keys with Flags=0 (created externally).
		// Write the value but leave Flags unchanged so the key is not deleted on CR removal.
		if !alreadyOwned && pair != nil && currentFlags == 0 {
			ok, _, err := kvClient.CAS(&consulApi.KVPair{
				Key:         key,
				Value:       []byte(value),
				Flags:       0,
				ModifyIndex: modifyIndex,
			}, nil)
			if err != nil {
				return false, fmt.Errorf("KV cas %q: %w", key, err)
			}
			if ok {
				return false, nil
			}
			// CAS conflict — retry with fresh read
			continue
		}

		newFlags := currentFlags
		if !alreadyOwned {
			newFlags = currentFlags + 1
		}

		ok, _, err := kvClient.CAS(&consulApi.KVPair{
			Key:         key,
			Value:       []byte(value),
			Flags:       newFlags,
			ModifyIndex: modifyIndex,
		}, nil)
		if err != nil {
			return false, fmt.Errorf("KV cas %q: %w", key, err)
		}
		if ok {
			return true, nil
		}
		// CAS conflict — retry with fresh read
	}
	return false, fmt.Errorf("KV cas %q: max retries exceeded", key)
}

// decrementOrDelete decrements the Flags counter via CAS. If Flags reaches 0, deletes the key.
func decrementOrDelete(key string) error {
	for attempt := 0; attempt < casMaxRetries; attempt++ {
		pair, _, err := kvClient.Get(key, nil)
		if err != nil {
			return fmt.Errorf("KV get %q: %w", key, err)
		}
		if pair == nil {
			return nil // already gone — idempotent
		}

		if pair.Flags <= 1 {
			ok, _, err := kvClient.DeleteCAS(pair, nil)
			if err != nil {
				return fmt.Errorf("KV deletecas %q: %w", key, err)
			}
			if ok {
				return nil
			}
			continue // conflict — retry
		}

		ok, _, err := kvClient.CAS(&consulApi.KVPair{
			Key:         key,
			Value:       pair.Value,
			Flags:       pair.Flags - 1,
			ModifyIndex: pair.ModifyIndex,
		}, nil)
		if err != nil {
			return fmt.Errorf("KV cas decrement %q: %w", key, err)
		}
		if ok {
			return nil
		}
		// conflict — retry
	}
	return fmt.Errorf("KV decrement %q: max retries exceeded", key)
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

// deleteKVEntries is called on CR deletion. Decrements Flags for every owned entry.
func deleteKVEntries(statuses []consulacl.ConsulKVEntryStatus) error {
	var firstErr error
	for _, e := range statuses {
		if e.Key == "" || !e.Owned {
			continue
		}
		if err := decrementOrDelete(e.Key); err != nil {
			kvLog.Error(err, "Error during KV ownership cleanup", "key", e.Key)
			if firstErr == nil {
				firstErr = err
			}
		}
	}
	return firstErr
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

// deleteRemovedEntries decrements Flags for entries that were in status but removed from spec.
// Returns the set of keys that were successfully cleaned (for status update).
func deleteRemovedEntries(existing []consulacl.ConsulKVEntryStatus, specEntries []consulacl.ConsulKVEntry) (map[string]bool, error) {
	specKeys := make(map[string]bool, len(specEntries))
	for _, e := range specEntries {
		specKeys[e.Key] = true
	}
	cleanedKeys := make(map[string]bool)
	var firstErr error
	for _, e := range existing {
		if !specKeys[e.Key] && e.Owned {
			if err := decrementOrDelete(e.Key); err != nil {
				kvLog.Error(err, "Error decrementing KV flags on remove", "key", e.Key)
				if firstErr == nil {
					firstErr = err
				}
			} else {
				cleanedKeys[e.Key] = true
			}
		}
	}
	return cleanedKeys, firstErr
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
