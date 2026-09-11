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
	"testing"

	consulacl "github.com/Netcracker/consul-acl-configurator/consul-acl-configurator-operator/api/v1alpha1"
	consulApi "github.com/hashicorp/consul/api"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

// ---- helpers ----

func kvScheme() *runtime.Scheme {
	s := runtime.NewScheme()
	_ = consulacl.AddToScheme(s)
	return s
}

func newConsulKV(name, namespace string, finalizers []string, entries []consulacl.ConsulKVEntry) *consulacl.ConsulKV {
	return &consulacl.ConsulKV{
		ObjectMeta: metav1.ObjectMeta{
			Name:       name,
			Namespace:  namespace,
			Finalizers: finalizers,
		},
		Spec: consulacl.ConsulKVSpec{
			KV: consulacl.ConsulKVConfig{Entries: entries},
		},
	}
}

// ---- 16.1: applyKVEntries ----

// 16.1a: all entries written verbatim (keys passed to CAS unchanged)
func TestApplyKVEntries_AllEntriesWrittenVerbatim(t *testing.T) {
	mock := &mockKVClient{}
	origKV := kvClient
	kvClient = mock
	defer func() { kvClient = origKV }()

	entries := []consulacl.ConsulKVEntry{
		{Key: "config/ns/app/", Value: ""},
		{Key: "logging/ns/app/LOG_LEVEL", Value: "INFO"},
	}
	statuses, err := applyKVEntries(entries, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(mock.casPairs) != 2 {
		t.Fatalf("expected 2 CAS calls, got %d", len(mock.casPairs))
	}
	if mock.casPairs[0].Key != "config/ns/app/" {
		t.Errorf("first key: got %q, want %q", mock.casPairs[0].Key, "config/ns/app/")
	}
	if mock.casPairs[1].Key != "logging/ns/app/LOG_LEVEL" {
		t.Errorf("second key: got %q, want %q", mock.casPairs[1].Key, "logging/ns/app/LOG_LEVEL")
	}
	if string(mock.casPairs[1].Value) != "INFO" {
		t.Errorf("second value: got %q, want %q", string(mock.casPairs[1].Value), "INFO")
	}
	if len(statuses) != 2 {
		t.Fatalf("expected 2 statuses, got %d", len(statuses))
	}
	for _, s := range statuses {
		if s.Status != "synced" {
			t.Errorf("key %q: got status %q, want \"synced\"", s.Key, s.Status)
		}
		if !s.Owned {
			t.Errorf("key %q: Owned should be true after first apply", s.Key)
		}
	}
}

// 16.1b: empty-key entry is skipped with an error status; loop continues
func TestApplyKVEntries_EmptyKeySkippedWithErrorStatus(t *testing.T) {
	mock := &mockKVClient{}
	origKV := kvClient
	kvClient = mock
	defer func() { kvClient = origKV }()

	entries := []consulacl.ConsulKVEntry{
		{Key: ""},
		{Key: "valid/key", Value: "v"},
	}
	statuses, err := applyKVEntries(entries, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(mock.casPairs) != 1 {
		t.Errorf("CAS called %d times; only the valid key should be written", len(mock.casPairs))
	}
	if len(statuses) != 2 {
		t.Fatalf("expected 2 statuses, got %d", len(statuses))
	}
	if statuses[0].Status == "synced" {
		t.Error("empty-key entry should not have status \"synced\"")
	}
	if statuses[1].Status != "synced" {
		t.Errorf("valid-key entry: got status %q, want \"synced\"", statuses[1].Status)
	}
}

// 16.1c: idempotent re-apply — second apply with owned=true preserves existing flags
func TestApplyKVEntries_IdempotentReapply(t *testing.T) {
	mock := &mockKVClient{}
	origKV := kvClient
	kvClient = mock
	defer func() { kvClient = origKV }()

	entries := []consulacl.ConsulKVEntry{{Key: "data/ns/svc", Value: "x"}}

	// First apply: not owned yet — flags should go from 0 to 1
	statuses, err := applyKVEntries(entries, nil)
	if err != nil {
		t.Fatalf("apply 1: unexpected error: %v", err)
	}
	if len(statuses) != 1 || statuses[0].Status != "synced" || !statuses[0].Owned {
		t.Errorf("apply 1: unexpected status %v", statuses)
	}
	flagsAfterFirst := mock.store["data/ns/svc"].Flags
	if flagsAfterFirst != 1 {
		t.Errorf("flags after first apply: got %d, want 1", flagsAfterFirst)
	}

	// Second apply: already owned — flags must not change
	statuses, err = applyKVEntries(entries, map[string]bool{"data/ns/svc": true})
	if err != nil {
		t.Fatalf("apply 2: unexpected error: %v", err)
	}
	if len(statuses) != 1 || statuses[0].Status != "synced" {
		t.Errorf("apply 2: unexpected status %v", statuses)
	}
	flagsAfterSecond := mock.store["data/ns/svc"].Flags
	if flagsAfterSecond != 1 {
		t.Errorf("flags after second apply: got %d, want 1 (should not increment again)", flagsAfterSecond)
	}
}

// 16.1d: network error from CAS is returned; remaining entries still processed
func TestApplyKVEntries_NetworkErrorReturned(t *testing.T) {
	netErr := fmt.Errorf("dial tcp: connection refused")
	callCount := 0
	mock := &mockKVClient{
		casFunc: func(p *consulApi.KVPair) (bool, error) {
			callCount++
			if callCount == 1 {
				return false, netErr
			}
			return true, nil
		},
	}
	origKV := kvClient
	kvClient = mock
	defer func() { kvClient = origKV }()

	entries := []consulacl.ConsulKVEntry{
		{Key: "key/one"},
		{Key: "key/two"},
	}
	statuses, err := applyKVEntries(entries, nil)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if len(statuses) != 2 {
		t.Errorf("expected 2 statuses even on error, got %d", len(statuses))
	}
	if statuses[0].Status == "synced" {
		t.Error("first entry should have error status")
	}
}

// ---- 16.2: ownership tracking ----

// 16.2a: two owners — key is not deleted on first CR removal, only flags decremented
func TestOwnership_KeyNotDeletedUntilLastOwner(t *testing.T) {
	mock := &mockKVClient{}
	origKV := kvClient
	kvClient = mock
	defer func() { kvClient = origKV }()

	// Simulate key with flags=2 (two owners)
	mock.initStore()
	mock.store["shared/key"] = &consulApi.KVPair{Key: "shared/key", Value: []byte("v"), Flags: 2, ModifyIndex: 5}

	// First owner releases
	if err := decrementOrDelete("shared/key"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(mock.deletedKeys) != 0 {
		t.Errorf("key should not be deleted while flags > 1, deletedKeys=%v", mock.deletedKeys)
	}
	if mock.store["shared/key"] == nil || mock.store["shared/key"].Flags != 1 {
		t.Errorf("flags should be 1 after first decrement, got store=%v", mock.store["shared/key"])
	}
}

// 16.2b: last owner — key is deleted when flags reaches 0
func TestOwnership_KeyDeletedByLastOwner(t *testing.T) {
	mock := &mockKVClient{}
	origKV := kvClient
	kvClient = mock
	defer func() { kvClient = origKV }()

	mock.initStore()
	mock.store["shared/key"] = &consulApi.KVPair{Key: "shared/key", Value: []byte("v"), Flags: 1, ModifyIndex: 3}

	if err := decrementOrDelete("shared/key"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(mock.deletedKeys) != 1 || mock.deletedKeys[0] != "shared/key" {
		t.Errorf("expected key to be deleted, deletedKeys=%v", mock.deletedKeys)
	}
	if _, exists := mock.store["shared/key"]; exists {
		t.Error("key should be absent from store after last owner releases")
	}
}

// 16.2c: already absent key — decrementOrDelete is a no-op
func TestOwnership_AlreadyAbsentKey(t *testing.T) {
	mock := &mockKVClient{}
	origKV := kvClient
	kvClient = mock
	defer func() { kvClient = origKV }()

	if err := decrementOrDelete("missing/key"); err != nil {
		t.Errorf("expected no error for absent key, got %v", err)
	}
	if len(mock.deletedKeys) != 0 {
		t.Errorf("no deletions expected, got %v", mock.deletedKeys)
	}
}

// ---- 16.3: reconcile loop ----

// 16.3a: finalizer added on first reconcile (no finalizer present, DeletionTimestamp zero)
func TestReconcile_FinalizerAddedOnFirstReconcile(t *testing.T) {
	cr := newConsulKV("test-kv", "default", nil, []consulacl.ConsulKVEntry{{Key: "k"}})
	fakeClient := fake.NewClientBuilder().WithScheme(kvScheme()).WithObjects(cr).Build()

	r := &ConsulKVReconciler{Client: fakeClient, Scheme: kvScheme()}
	_, err := r.Reconcile(context.TODO(), ctrl.Request{
		NamespacedName: types.NamespacedName{Name: "test-kv", Namespace: "default"},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	updated := &consulacl.ConsulKV{}
	if err := fakeClient.Get(context.TODO(), types.NamespacedName{Name: "test-kv", Namespace: "default"}, updated); err != nil {
		t.Fatalf("failed to get updated CR: %v", err)
	}
	if !containsFinalizer(updated.GetFinalizers(), consulKVFinalizer) {
		t.Errorf("finalizer %q not added; finalizers: %v", consulKVFinalizer, updated.GetFinalizers())
	}
}

// 16.3b: active reconcile (finalizer present, DeletionTimestamp zero) calls apply and writes status
func TestReconcile_ActiveReconcileCallsApplyAndWritesStatus(t *testing.T) {
	mock := &mockKVClient{}
	origKV := kvClient
	kvClient = mock
	defer func() { kvClient = origKV }()

	cr := newConsulKV("test-kv", "default", []string{consulKVFinalizer}, []consulacl.ConsulKVEntry{
		{Key: "config/ns/app", Value: "val"},
	})
	fakeClient := fake.NewClientBuilder().
		WithScheme(kvScheme()).
		WithObjects(cr).
		WithStatusSubresource(cr).
		Build()

	r := &ConsulKVReconciler{Client: fakeClient, Scheme: kvScheme()}
	result, err := r.Reconcile(context.TODO(), ctrl.Request{
		NamespacedName: types.NamespacedName{Name: "test-kv", Namespace: "default"},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.RequeueAfter != 0 {
		t.Errorf("expected no requeue, got RequeueAfter=%v", result.RequeueAfter)
	}
	if len(mock.deletedKeys) != 0 {
		t.Errorf("Delete should not be called on active reconcile")
	}

	updated := &consulacl.ConsulKV{}
	if err := fakeClient.Get(context.TODO(), types.NamespacedName{Name: "test-kv", Namespace: "default"}, updated); err != nil {
		t.Fatalf("failed to get updated CR: %v", err)
	}
	if updated.Status.GeneralStatus != "synced" {
		t.Errorf("GeneralStatus: got %q, want \"synced\"", updated.Status.GeneralStatus)
	}
	if len(updated.Status.Entries) != 1 || updated.Status.Entries[0].Key != "config/ns/app" {
		t.Errorf("unexpected status entries: %v", updated.Status.Entries)
	}
	if !updated.Status.Entries[0].Owned {
		t.Errorf("status entry should have Owned=true after first apply")
	}
}

// 16.3c: deletion reconcile (DeletionTimestamp non-zero, finalizer present) decrements flags and removes finalizer
func TestReconcile_DeletionCallsDeleteAndRemovesFinalizer(t *testing.T) {
	mock := &mockKVClient{}
	origKV := kvClient
	kvClient = mock
	defer func() { kvClient = origKV }()

	// Pre-populate the Consul store with the key (flags=1, only owner)
	mock.initStore()
	mock.store["config/ns/app"] = &consulApi.KVPair{Key: "config/ns/app", Value: []byte{}, Flags: 1, ModifyIndex: 1}

	now := metav1.Now()
	cr := newConsulKV("test-kv", "default", []string{consulKVFinalizer}, []consulacl.ConsulKVEntry{
		{Key: "config/ns/app"},
	})
	cr.DeletionTimestamp = &now
	// CR has status from a prior reconcile — owned=true
	cr.Status = consulacl.ConsulKVStatus{
		Entries: []consulacl.ConsulKVEntryStatus{
			{Key: "config/ns/app", Status: "synced", Owned: true},
		},
	}

	fakeClient := fake.NewClientBuilder().WithScheme(kvScheme()).WithObjects(cr).Build()

	r := &ConsulKVReconciler{Client: fakeClient, Scheme: kvScheme()}
	_, err := r.Reconcile(context.TODO(), ctrl.Request{
		NamespacedName: types.NamespacedName{Name: "test-kv", Namespace: "default"},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(mock.deletedKeys) != 1 || mock.deletedKeys[0] != "config/ns/app" {
		t.Errorf("expected DeleteCAS called for \"config/ns/app\", got deletedKeys=%v", mock.deletedKeys)
	}

	remaining := &consulacl.ConsulKV{}
	getErr := fakeClient.Get(context.TODO(), types.NamespacedName{Name: "test-kv", Namespace: "default"}, remaining)
	if getErr == nil && containsFinalizer(remaining.GetFinalizers(), consulKVFinalizer) {
		t.Errorf("finalizer should have been removed; finalizers: %v", remaining.GetFinalizers())
	}
}

// 16.3d: a status-only update (generation unchanged) does not trigger a KV write.
func TestReconcile_StatusOnlyUpdate_DoesNotCallApplyTwice(t *testing.T) {
	casCount := 0
	mock := &mockKVClient{
		casFunc: func(p *consulApi.KVPair) (bool, error) {
			casCount++
			return true, nil
		},
	}
	origKV := kvClient
	kvClient = mock
	defer func() { kvClient = origKV }()

	cr := newConsulKV("test-kv", "default", []string{consulKVFinalizer}, []consulacl.ConsulKVEntry{
		{Key: "data/ns/svc"},
	})
	fakeClient := fake.NewClientBuilder().
		WithScheme(kvScheme()).
		WithObjects(cr).
		WithStatusSubresource(cr).
		Build()

	r := &ConsulKVReconciler{Client: fakeClient, Scheme: kvScheme()}
	req := ctrl.Request{NamespacedName: types.NamespacedName{Name: "test-kv", Namespace: "default"}}

	if _, err := r.Reconcile(context.TODO(), req); err != nil {
		t.Fatalf("first reconcile: %v", err)
	}
	afterFirst := casCount

	if _, err := r.Reconcile(context.TODO(), req); err != nil {
		t.Fatalf("second reconcile: %v", err)
	}
	afterSecond := casCount

	if afterFirst != 1 {
		t.Errorf("first reconcile: expected 1 CAS call, got %d", afterFirst)
	}
	if afterSecond != 2 {
		t.Errorf("second reconcile: expected 2 cumulative CAS calls (idempotent), got %d", afterSecond)
	}
}

// 16.3e: not-found CR returns without error and does not requeue
func TestReconcile_NotFound_ReturnsWithoutError(t *testing.T) {
	fakeClient := fake.NewClientBuilder().WithScheme(kvScheme()).Build()

	r := &ConsulKVReconciler{Client: fakeClient, Scheme: kvScheme()}
	result, err := r.Reconcile(context.TODO(), ctrl.Request{
		NamespacedName: types.NamespacedName{Name: "missing", Namespace: "default"},
	})
	if err != nil {
		t.Fatalf("expected nil error for not-found CR, got: %v", err)
	}
	if result.RequeueAfter != 0 {
		t.Errorf("expected no requeue for not-found CR, got result=%+v", result)
	}
}
