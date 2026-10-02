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
	"sort"
	"strings"
	"testing"

	consulacl "github.com/Netcracker/consul-acl-configurator/consul-acl-configurator-operator/api/v1alpha1"
	consulApi "github.com/hashicorp/consul/api"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

func useKVMock(t *testing.T, mock *mockKVClient) {
	origKV, origTxn := kvClient, txnClient
	kvClient, txnClient = mock, mock
	mock.initStore()
	t.Cleanup(func() { kvClient, txnClient = origKV, origTxn })
}

// failTxnCall makes the n-th transaction (1-based) fail with a network-like error; the other
// transactions are executed by the in-memory store.
func failTxnCall(mock *mockKVClient, n int) {
	calls := 0
	var txn func(ops consulApi.TxnOps) (bool, *consulApi.TxnResponse, error)
	txn = func(ops consulApi.TxnOps) (bool, *consulApi.TxnResponse, error) {
		calls++
		if calls == n {
			return false, nil, fmt.Errorf("connection refused")
		}
		// Run the default in-memory transaction, then restore the hook for the next call.
		mock.txnFunc = nil
		defer func() { mock.txnFunc = txn }()
		ok, resp, _, err := mock.Txn(ops, nil)
		return ok, resp, err
	}
	mock.txnFunc = txn
}

func storeKeys(mock *mockKVClient) []string {
	var keys []string
	for k := range mock.store {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

func manyEntries(n int) []consulacl.ConsulKVEntry {
	entries := make([]consulacl.ConsulKVEntry, n)
	for i := range entries {
		entries[i] = consulacl.ConsulKVEntry{Key: fmt.Sprintf("config/app/key-%03d", i), Value: "v"}
	}
	return entries
}

// --- 22.4: purgeOnDelete treats the key as a directory ---

func TestDeleteKVTree_PrefixCollision_SiblingsKept(t *testing.T) {
	mock := &mockKVClient{}
	useKVMock(t, mock)
	for _, k := range []string{"config/app", "config/app/db_url", "config/app/nested/x", "config/application/x", "config/app-gateway/y"} {
		mock.store[k] = &consulApi.KVPair{Key: k, ModifyIndex: 1}
	}

	if err := deleteKVTree([]consulacl.ConsulKVEntryStatus{{Key: "config/app"}}); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want := []string{"config/app-gateway/y", "config/application/x"}
	if got := storeKeys(mock); strings.Join(got, ",") != strings.Join(want, ",") {
		t.Errorf("remaining keys: got %v, want %v", got, want)
	}
	if len(mock.deletedTrees) != 1 || mock.deletedTrees[0] != "config/app/" {
		t.Errorf("tree must be deleted with a trailing slash, got %v", mock.deletedTrees)
	}
}

// A declared key with a trailing slash is purged as a tree, also keys shared with other CRs.
func TestDeleteKVTree_KeyWithSlash_SharedKeysPurged(t *testing.T) {
	mock := &mockKVClient{}
	useKVMock(t, mock)
	mock.store["config/app/"] = &consulApi.KVPair{Key: "config/app/", ModifyIndex: 1}
	mock.store["config/app/shared"] = &consulApi.KVPair{Key: "config/app/shared", Flags: 2, ModifyIndex: 1}
	mock.store["config/application/x"] = &consulApi.KVPair{Key: "config/application/x", ModifyIndex: 1}

	if err := deleteKVTree([]consulacl.ConsulKVEntryStatus{{Key: "config/app/"}}); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got := storeKeys(mock); len(got) != 1 || got[0] != "config/application/x" {
		t.Errorf("remaining keys: got %v", got)
	}
	if len(mock.deletedKeys) != 0 {
		t.Errorf("no exact delete is needed for a key ending with '/', got %v", mock.deletedKeys)
	}
}

func TestDeleteKVTree_ErrorReturnedAndOtherKeysProcessed(t *testing.T) {
	mock := &mockKVClient{deleteTreeFunc: func(prefix string) error {
		if prefix == "config/a/" {
			return fmt.Errorf("connection refused")
		}
		return nil
	}}
	useKVMock(t, mock)

	err := deleteKVTree([]consulacl.ConsulKVEntryStatus{{Key: "config/a"}, {Key: "config/b"}})
	if err == nil {
		t.Fatal("expected the error to be returned")
	}
	if len(mock.deletedKeys) != 2 {
		t.Errorf("both keys must be processed, got %v", mock.deletedKeys)
	}
}

// 22.7: deletion reconcile with purgeOnDelete removes the tree and the finalizer.
func TestReconcile_PurgeOnDelete_RemovesTreeAndFinalizer(t *testing.T) {
	mock := &mockKVClient{}
	useKVMock(t, mock)
	for _, k := range []string{"config/app", "config/app/db_url", "config/application/x"} {
		mock.store[k] = &consulApi.KVPair{Key: k, Flags: 2, ModifyIndex: 1}
	}

	now := metav1.Now()
	cr := newConsulKV("test-kv", "default", []string{consulKVFinalizer}, []consulacl.ConsulKVEntry{{Key: "config/app"}})
	cr.Spec.KV.PurgeOnDelete = true
	cr.DeletionTimestamp = &now
	cr.Status.Entries = []consulacl.ConsulKVEntryStatus{{Key: "config/app", Status: "synced", Owned: true}}
	fakeClient := fake.NewClientBuilder().WithScheme(kvScheme()).WithObjects(cr).Build()

	r := &ConsulKVReconciler{Client: fakeClient, Scheme: kvScheme()}
	if _, err := r.Reconcile(context.TODO(), ctrl.Request{NamespacedName: types.NamespacedName{Name: "test-kv", Namespace: "default"}}); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got := storeKeys(mock); len(got) != 1 || got[0] != "config/application/x" {
		t.Errorf("remaining keys: got %v", got)
	}
	remaining := &consulacl.ConsulKV{}
	if err := fakeClient.Get(context.TODO(), types.NamespacedName{Name: "test-kv", Namespace: "default"}, remaining); err == nil &&
		containsFinalizer(remaining.GetFinalizers(), consulKVFinalizer) {
		t.Errorf("finalizer should have been removed")
	}
}

// --- 22.5: duplicate keys ---

func TestApplyKVEntries_DuplicateKey_LastWinsOthersWritten(t *testing.T) {
	mock := &mockKVClient{}
	useKVMock(t, mock)

	entries := []consulacl.ConsulKVEntry{
		{Key: "config/app/url", Value: "first"},
		{Key: "config/app/port", Value: "8080"},
		{Key: "config/app/url", Value: "last"},
	}
	statuses, err := applyKVEntries(entries, nil)
	if err != nil {
		t.Fatalf("a duplicate must not fail the transaction: %v", err)
	}
	if v := string(mock.store["config/app/url"].Value); v != "last" {
		t.Errorf("value of the last entry must win, got %q", v)
	}
	if mock.store["config/app/url"].Flags != 1 {
		t.Errorf("duplicate must be counted once, Flags=%d", mock.store["config/app/url"].Flags)
	}
	if mock.store["config/app/port"] == nil {
		t.Error("other keys must be written")
	}
	if statuses[0].Status != statusSkippedDuplicate || statuses[0].Owned {
		t.Errorf("earlier duplicate: got %+v", statuses[0])
	}
	if statuses[2].Status != "synced" || !statuses[2].Owned {
		t.Errorf("last duplicate: got %+v", statuses[2])
	}
}

func TestMergeKVStatuses_DuplicateKeptTogetherAndStable(t *testing.T) {
	updated := []consulacl.ConsulKVEntryStatus{
		{Key: "a", Status: statusSkippedDuplicate},
		{Key: "b", Status: "synced", Owned: true},
		{Key: "a", Status: "synced", Owned: true},
	}
	first := mergeKVStatuses(nil, updated, nil)
	second := mergeKVStatuses(first, updated, nil)
	if len(first) != 3 || len(second) != 3 {
		t.Fatalf("expected 3 entries in both cycles, got %v and %v", first, second)
	}
	if got := ownedStatusKeys(second); strings.Join(got, ",") != "a,b" {
		t.Errorf("owned keys: got %v", got)
	}
}

// --- 22.6: partial failure across batches ---

func TestApplyKVEntries_SecondBatchFails_FirstBatchOwnedAndNotIncrementedAgain(t *testing.T) {
	mock := &mockKVClient{}
	useKVMock(t, mock)
	entries := manyEntries(100)

	failTxnCall(mock, 2)
	statuses, err := applyKVEntries(entries, nil)
	if err == nil {
		t.Fatal("expected the error of the second batch")
	}
	for i, s := range statuses {
		if i < txnBatchSize && (!s.Owned || s.Status != "synced") {
			t.Fatalf("entry %d of the committed batch: got %+v", i, s)
		}
		if i >= txnBatchSize && (s.Owned || !strings.HasPrefix(s.Status, "error:")) {
			t.Fatalf("entry %d of the failed batch: got %+v", i, s)
		}
	}

	// Retry with the ownership recorded in status: Flags of the first batch stay 1.
	mock.txnFunc = nil
	ownedKeys := map[string]bool{}
	for _, k := range ownedStatusKeys(statuses) {
		ownedKeys[k] = true
	}
	if _, err = applyKVEntries(entries, ownedKeys); err != nil {
		t.Fatalf("unexpected error on retry: %v", err)
	}
	for _, e := range entries {
		if f := mock.store[e.Key].Flags; f != 1 {
			t.Fatalf("key %s: Flags=%d, want 1", e.Key, f)
		}
	}
}

// A key owned before keeps Owned=true when its batch fails, so ownership is not lost.
func TestApplyKVEntries_FailedBatch_PreviouslyOwnedKeyStaysOwned(t *testing.T) {
	mock := &mockKVClient{txnFunc: func(consulApi.TxnOps) (bool, *consulApi.TxnResponse, error) {
		return false, nil, fmt.Errorf("connection refused")
	}}
	useKVMock(t, mock)
	mock.store["config/app/url"] = &consulApi.KVPair{Key: "config/app/url", Flags: 1, ModifyIndex: 1}

	statuses, err := applyKVEntries([]consulacl.ConsulKVEntry{{Key: "config/app/url", Value: "v2"}}, map[string]bool{"config/app/url": true})
	if err == nil {
		t.Fatal("expected an error")
	}
	if !statuses[0].Owned {
		t.Errorf("previously owned key must stay owned, got %+v", statuses[0])
	}
}

func TestDeleteKVBatch_SecondBatchFails_ReleasedKeysReturned(t *testing.T) {
	mock := &mockKVClient{}
	useKVMock(t, mock)
	var keys []string
	for _, e := range manyEntries(100) {
		mock.store[e.Key] = &consulApi.KVPair{Key: e.Key, Flags: 2, ModifyIndex: 1}
		keys = append(keys, e.Key)
	}

	failTxnCall(mock, 2)
	released, err := deleteKVBatch(keys)
	if err == nil {
		t.Fatal("expected the error of the second batch")
	}
	if len(released) != txnBatchSize {
		t.Fatalf("expected %d released keys, got %d", txnBatchSize, len(released))
	}
	for i, k := range keys {
		if want := i < txnBatchSize; released[k] != want {
			t.Fatalf("key %s: released=%v, want %v", k, released[k], want)
		}
	}
}

// Deletion reconcile: a failed later batch records the released keys in status, and the
// retry releases only the rest, so a key shared with another CR is not decremented twice.
func TestReconcile_Deletion_PartialRelease_NotDecrementedTwice(t *testing.T) {
	mock := &mockKVClient{}
	useKVMock(t, mock)
	entries := manyEntries(100)
	var statusEntries []consulacl.ConsulKVEntryStatus
	for _, e := range entries {
		mock.store[e.Key] = &consulApi.KVPair{Key: e.Key, Flags: 2, ModifyIndex: 1}
		statusEntries = append(statusEntries, consulacl.ConsulKVEntryStatus{Key: e.Key, Status: "synced", Owned: true})
	}

	now := metav1.Now()
	cr := newConsulKV("test-kv", "default", []string{consulKVFinalizer}, entries)
	cr.DeletionTimestamp = &now
	cr.Status.Entries = statusEntries
	fakeClient := fake.NewClientBuilder().WithScheme(kvScheme()).WithObjects(cr).WithStatusSubresource(cr).Build()
	r := &ConsulKVReconciler{Client: fakeClient, Scheme: kvScheme()}
	req := ctrl.Request{NamespacedName: types.NamespacedName{Name: "test-kv", Namespace: "default"}}

	failTxnCall(mock, 2)
	if _, err := r.Reconcile(context.TODO(), req); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	mock.txnFunc = nil
	if _, err := r.Reconcile(context.TODO(), req); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	for _, e := range entries {
		if f := mock.store[e.Key].Flags; f != 1 {
			t.Fatalf("key %s: Flags=%d, want 1 (released exactly once)", e.Key, f)
		}
	}
	remaining := &consulacl.ConsulKV{}
	if err := fakeClient.Get(context.TODO(), req.NamespacedName, remaining); err == nil &&
		containsFinalizer(remaining.GetFinalizers(), consulKVFinalizer) {
		t.Errorf("finalizer should have been removed after the retry")
	}
}

// Keys removed from the spec: the keys of a committed batch are reported as released.
func TestDeleteRemovedEntries_PartialFailure_ReleasedKeysReturned(t *testing.T) {
	mock := &mockKVClient{}
	useKVMock(t, mock)
	var existing []consulacl.ConsulKVEntryStatus
	for _, e := range manyEntries(100) {
		mock.store[e.Key] = &consulApi.KVPair{Key: e.Key, Flags: 2, ModifyIndex: 1}
		existing = append(existing, consulacl.ConsulKVEntryStatus{Key: e.Key, Status: "synced", Owned: true})
	}

	failTxnCall(mock, 2)
	cleaned, err := deleteRemovedEntries(existing, nil)
	if err == nil {
		t.Fatal("expected an error")
	}
	merged := mergeKVStatuses(existing, nil, cleaned)
	if got := len(ownedStatusKeys(merged)); got != 100-txnBatchSize {
		t.Errorf("only the keys of the failed batch must stay owned, got %d", got)
	}
}
