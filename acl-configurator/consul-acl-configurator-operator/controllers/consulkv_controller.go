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

	consulacl "github.com/Netcracker/consul-acl-configurator/consul-acl-configurator-operator/api/v1alpha1"
	consulApi "github.com/hashicorp/consul/api"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/util/retry"
	"time"
)

var consulKVFinalizer = consulacl.GroupVersion.Group + "/consulkvconfigurator-controller"

var kvLog = logf.Log.WithName("controller_consulkv")

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
			if err := deleteKVEntries(instance.Spec.KV.Entries); err != nil {
				return reconcile.Result{RequeueAfter: time.Second * time.Duration(periodTime)}, nil
			}
			err = crUpdater.updateWithRetry(func(cr *consulacl.ConsulKV) {
				controllerutil.RemoveFinalizer(cr, consulKVFinalizer)
			})
			return reconcile.Result{}, err
		}
		return reconcile.Result{}, nil
	}

	entryStatuses, applyErr := applyKVEntries(instance.Spec.KV.Entries)

	statusErr := crUpdater.updateStatusWithRetry(func(cr *consulacl.ConsulKV) {
		cr.Status.Entries = mergeKVStatuses(cr.Status.Entries, entryStatuses)
		cr.Status.ManagedBy = "consul-acl-configurator-operator_" + r.OwnNamespace
		if applyErr != nil {
			cr.Status.GeneralStatus = "Degraded"
		} else {
			cr.Status.GeneralStatus = "Synced"
		}
	})
	if statusErr != nil {
		kvLog.Error(statusErr, "Error updating ConsulKV status")
		return reconcile.Result{RequeueAfter: time.Second * time.Duration(periodTime)}, nil
	}

	if applyErr != nil {
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

	return ctrl.NewControllerManagedBy(mgr).
		For(&consulacl.ConsulKV{}, builder.WithPredicates(statusPredicate)).
		Complete(r)
}

func applyKVEntries(entries []consulacl.ConsulKVEntry) ([]consulacl.ConsulKVEntryStatus, error) {
	statuses := make([]consulacl.ConsulKVEntryStatus, 0, len(entries))
	var firstNetErr error
	for _, entry := range entries {
		if entry.Key == "" {
			statuses = append(statuses, consulacl.ConsulKVEntryStatus{
				Key:    "",
				Status: "error: key must not be empty",
			})
			continue
		}
		_, err := kvClient.Put(&consulApi.KVPair{Key: entry.Key, Value: []byte(entry.Value)}, nil)
		if err != nil {
			statuses = append(statuses, consulacl.ConsulKVEntryStatus{
				Key:    entry.Key,
				Status: "error: " + err.Error(),
			})
			if firstNetErr == nil {
				firstNetErr = err
			}
		} else {
			statuses = append(statuses, consulacl.ConsulKVEntryStatus{
				Key:    entry.Key,
				Status: "Synced",
				Info:   "Created",
			})
		}
	}
	return statuses, firstNetErr
}

// mergeKVStatuses preserves the order of existing status entries and appends new ones at the bottom.
// Entries removed from spec are marked "deleted" and kept in the status history.
func mergeKVStatuses(existing, updated []consulacl.ConsulKVEntryStatus) []consulacl.ConsulKVEntryStatus {
	updatedMap := make(map[string]string, len(updated))
	for _, e := range updated {
		updatedMap[e.Key] = e.Status
	}

	result := make([]consulacl.ConsulKVEntryStatus, 0, len(existing)+len(updated))
	seen := make(map[string]bool, len(existing))

	// First pass: existing entries — update status or mark deleted if removed from spec.
	for _, e := range existing {
		seen[e.Key] = true
		if s, ok := updatedMap[e.Key]; ok {
			result = append(result, consulacl.ConsulKVEntryStatus{Key: e.Key, Status: s})
		} else if e.Info != "Removed" {
			result = append(result, consulacl.ConsulKVEntryStatus{Key: e.Key, Status: "Synced", Info: "Removed"})
		} else {
			result = append(result, e)
		}
	}

	// Second pass: append new entries (not seen in existing) in spec order.
	for _, e := range updated {
		if !seen[e.Key] {
			result = append(result, e)
		}
	}

	return result
}

func deleteKVEntries(entries []consulacl.ConsulKVEntry) error {
	for _, entry := range entries {
		if entry.Key == "" {
			continue
		}
		if _, err := kvClient.Delete(entry.Key, nil); err != nil {
			kvLog.Error(err, "Error deleting KV entry", "key", entry.Key)
			return err
		}
	}
	return nil
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
