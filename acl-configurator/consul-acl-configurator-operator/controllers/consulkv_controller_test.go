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
	origTxn := txnClient
	kvClient = mock
	txnClient = mock
	defer func() { kvClient = origKV; txnClient = origTxn }()

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
	origTxn := txnClient
	kvClient = mock
	txnClient = mock
	defer func() { kvClient = origKV; txnClient = origTxn }()

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
	origTxn := txnClient
	kvClient = mock
	txnClient = mock
	defer func() { kvClient = origKV; txnClient = origTxn }()

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
	origTxn := txnClient
	kvClient = mock
	txnClient = mock
	defer func() { kvClient = origKV; txnClient = origTxn }()

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
	origTxn := txnClient
	kvClient = mock
	txnClient = mock
	defer func() { kvClient = origKV; txnClient = origTxn }()

	// Simulate key with flags=2 (two owners)
	mock.initStore()
	mock.store["shared/key"] = &consulApi.KVPair{Key: "shared/key", Value: []byte("v"), Flags: 2, ModifyIndex: 5}

	// First owner releases
	if err := deleteKVBatch([]string{"shared/key"}); err != nil {
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
	origTxn := txnClient
	kvClient = mock
	txnClient = mock
	defer func() { kvClient = origKV; txnClient = origTxn }()

	mock.initStore()
	mock.store["shared/key"] = &consulApi.KVPair{Key: "shared/key", Value: []byte("v"), Flags: 1, ModifyIndex: 3}

	if err := deleteKVBatch([]string{"shared/key"}); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(mock.deletedKeys) != 1 || mock.deletedKeys[0] != "shared/key" {
		t.Errorf("expected key to be deleted, deletedKeys=%v", mock.deletedKeys)
	}
	if _, exists := mock.store["shared/key"]; exists {
		t.Error("key should be absent from store after last owner releases")
	}
}

// 16.2c: already absent key — deleteKVBatch is a no-op
func TestOwnership_AlreadyAbsentKey(t *testing.T) {
	mock := &mockKVClient{}
	origKV := kvClient
	origTxn := txnClient
	kvClient = mock
	txnClient = mock
	defer func() { kvClient = origKV; txnClient = origTxn }()

	if err := deleteKVBatch([]string{"missing/key"}); err != nil {
		t.Errorf("expected no error for absent key, got %v", err)
	}
	if len(mock.deletedKeys) != 0 {
		t.Errorf("no deletions expected, got %v", mock.deletedKeys)
	}
}

// ---- 16.4: transaction behavior ----

// 16.4a: a CAS conflict (txn ok=false) retries the whole batch until it succeeds.
func TestWriteKVChunk_RetriesOnCASConflict(t *testing.T) {
	mock := &mockKVClient{}
	attempts := 0
	mock.txnFunc = func(ops consulApi.TxnOps) (bool, *consulApi.TxnResponse, error) {
		attempts++
		if attempts == 1 {
			return false, &consulApi.TxnResponse{Errors: consulApi.TxnErrors{{OpIndex: 0, What: "index mismatch"}}}, nil
		}
		return true, &consulApi.TxnResponse{}, nil
	}
	origKV := kvClient
	origTxn := txnClient
	kvClient = mock
	txnClient = mock
	defer func() { kvClient = origKV; txnClient = origTxn }()

	owned := map[string]bool{}
	if err := writeKVChunk([]consulacl.ConsulKVEntry{{Key: "k", Value: "v"}}, nil, owned); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if attempts != 2 {
		t.Errorf("expected 2 attempts (1 conflict + 1 success), got %d", attempts)
	}
	if !owned["k"] {
		t.Error("key should be owned after a successful apply")
	}
}

// 16.4b: entries beyond txnBatchSize are split into multiple transactions.
func TestWriteKVBatch_SplitsByTxnBatchSize(t *testing.T) {
	mock := &mockKVClient{}
	var sizes []int
	mock.txnFunc = func(ops consulApi.TxnOps) (bool, *consulApi.TxnResponse, error) {
		sizes = append(sizes, len(ops))
		return true, &consulApi.TxnResponse{}, nil
	}
	origKV := kvClient
	origTxn := txnClient
	kvClient = mock
	txnClient = mock
	defer func() { kvClient = origKV; txnClient = origTxn }()

	entries := make([]consulacl.ConsulKVEntry, txnBatchSize+5)
	for i := range entries {
		entries[i] = consulacl.ConsulKVEntry{Key: fmt.Sprintf("k/%d", i)}
	}
	if _, err := writeKVBatchWithOwnership(entries, nil); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(sizes) != 2 || sizes[0] != txnBatchSize || sizes[1] != 5 {
		t.Errorf("expected batches [%d, 5], got %v", txnBatchSize, sizes)
	}
}

// 16.4c: one transaction both decrements a shared key and deletes a last-owner key.
func TestDeleteKVBatch_DecrementAndDeleteMixed(t *testing.T) {
	mock := &mockKVClient{}
	origKV := kvClient
	origTxn := txnClient
	kvClient = mock
	txnClient = mock
	defer func() { kvClient = origKV; txnClient = origTxn }()

	mock.initStore()
	mock.store["shared"] = &consulApi.KVPair{Key: "shared", Value: []byte("v"), Flags: 2, ModifyIndex: 7}
	mock.store["solo"] = &consulApi.KVPair{Key: "solo", Value: []byte("v"), Flags: 1, ModifyIndex: 9}

	if err := deleteKVBatch([]string{"shared", "solo"}); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if mock.store["shared"] == nil || mock.store["shared"].Flags != 1 {
		t.Errorf("shared should be decremented to Flags=1, got %v", mock.store["shared"])
	}
	if _, ok := mock.store["solo"]; ok {
		t.Error("solo should be deleted once its counter reaches 0")
	}
	if len(mock.deletedKeys) != 1 || mock.deletedKeys[0] != "solo" {
		t.Errorf("expected only \"solo\" deleted, got %v", mock.deletedKeys)
	}
}

// 16.4d: a pre-existing external key (Flags=0) gets its value written but is not owned.
func TestApplyKVEntries_ExternalKeyFlagsZero_NotOwned(t *testing.T) {
	mock := &mockKVClient{}
	origKV := kvClient
	origTxn := txnClient
	kvClient = mock
	txnClient = mock
	defer func() { kvClient = origKV; txnClient = origTxn }()

	mock.initStore()
	mock.store["ext/key"] = &consulApi.KVPair{Key: "ext/key", Value: []byte("old"), Flags: 0, ModifyIndex: 4}

	statuses, err := applyKVEntries([]consulacl.ConsulKVEntry{{Key: "ext/key", Value: "new"}}, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if statuses[0].Owned {
		t.Error("external key with Flags=0 must not be owned")
	}
	if mock.store["ext/key"].Flags != 0 {
		t.Errorf("Flags must stay 0 for external key, got %d", mock.store["ext/key"].Flags)
	}
	if string(mock.store["ext/key"].Value) != "new" {
		t.Errorf("value should be updated, got %q", string(mock.store["ext/key"].Value))
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
	origTxn := txnClient
	kvClient = mock
	txnClient = mock
	defer func() { kvClient = origKV; txnClient = origTxn }()

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
	origTxn := txnClient
	kvClient = mock
	txnClient = mock
	defer func() { kvClient = origKV; txnClient = origTxn }()

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
	origTxn := txnClient
	kvClient = mock
	txnClient = mock
	defer func() { kvClient = origKV; txnClient = origTxn }()

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
