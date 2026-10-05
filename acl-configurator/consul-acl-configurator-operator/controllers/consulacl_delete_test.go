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
	"github.com/Netcracker/consul-acl-configurator/consul-acl-configurator-operator/util"
	consulApi "github.com/hashicorp/consul/api"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

// deleteNextToBrokenSibling deletes the CR "app" in ns1 while the CR "broken" of the same namespace
// has an invalid configuration, and reports whether the finalizer was removed.
func deleteNextToBrokenSibling(t *testing.T, json string, explicitName bool) bool {
	now := metav1.Now()
	cr := newConsulACL("app", "ns1", json, explicitName)
	cr.Finalizers = []string{consulAclFinalizer}
	cr.DeletionTimestamp = &now
	broken := newConsulACL("broken", "ns1", `{not json`, true)
	fakeClient := fake.NewClientBuilder().WithScheme(kvScheme()).WithObjects(cr, broken).Build()

	r := &ConsulACLReconciler{Client: fakeClient, OwnNamespace: "ns1"}
	req := ctrl.Request{NamespacedName: types.NamespacedName{Name: "app", Namespace: "ns1"}}
	if _, err := r.Reconcile(context.TODO(), req); err != nil {
		t.Fatalf("a broken sibling must not block the deletion: %v", err)
	}
	after := &consulacl.ConsulACL{}
	err := fakeClient.Get(context.TODO(), req.NamespacedName, after)
	return errors.IsNotFound(err) || (err == nil && !util.Contains(consulAclFinalizer, after.GetFinalizers()))
}

// 24.7: prefixed names belong to the CR only, the siblings are not consulted and the entities are deleted.
func TestDeleteACL_PrefixedNextToBrokenSibling_Deleted(t *testing.T) {
	mock := &mockACLClient{
		roleReadByNameFunc: func(name string, _ *consulApi.QueryOptions) (*consulApi.ACLRole, *consulApi.QueryMeta, error) {
			return &consulApi.ACLRole{ID: "role-1", Name: name, Description: owners("ns1")}, nil, nil
		},
	}
	useACLClient(t, mock)

	if !deleteNextToBrokenSibling(t, `{"roles":[{"Name":"reader"}]}`, false) {
		t.Error("finalizer must be removed")
	}
	if fmt.Sprint(mock.roleDeletedIDs) != "[role-1]" {
		t.Errorf("the role of the CR must be deleted, got %v", mock.roleDeletedIDs)
	}
}

// 24.7: with explicit names the shared entities are kept (the broken sibling may use them), the
// deletion completes, and the rules of the CR under legacy methods are still removed.
func TestDeleteACL_ExplicitNextToBrokenSibling_SharedKeptFinalizerRemoved(t *testing.T) {
	useLegacyAuthMethods(t, "applications-k8s-m2m", legacyMethod)
	mock := newLegacyMock([]*consulApi.ACLBindingRule{
		{ID: "legacy-rule", BindName: "app_ns1_old", BindType: consulApi.BindingRuleBindTypeRole},
	})
	mock.roleReadByNameFunc = func(name string, _ *consulApi.QueryOptions) (*consulApi.ACLRole, *consulApi.QueryMeta, error) {
		return &consulApi.ACLRole{ID: "role-1", Name: name, Description: owners("ns1")}, nil, nil
	}
	mock.policyReadByNameFunc = func(name string, _ *consulApi.QueryOptions) (*consulApi.ACLPolicy, *consulApi.QueryMeta, error) {
		return &consulApi.ACLPolicy{ID: "pol-1", Name: name, Description: owners("ns1")}, nil, nil
	}
	useACLClient(t, mock)

	if !deleteNextToBrokenSibling(t, `{"policies":[{"Name":"shared-policy"}],"roles":[{"Name":"shared-reader"}]}`, true) {
		t.Error("finalizer must be removed")
	}
	if len(mock.roleDeletedIDs) != 0 || len(mock.roleUpdated) != 0 || len(mock.policyDeletedIDs) != 0 || len(mock.policyUpdated) != 0 {
		t.Errorf("shared entities must not be touched, roles deleted %v updated %v, policies deleted %v updated %v",
			mock.roleDeletedIDs, mock.roleUpdated, mock.policyDeletedIDs, mock.policyUpdated)
	}
	if len(mock.tokenDeletedIDs) != 0 {
		t.Errorf("tokens must not be revoked, got %v", mock.tokenDeletedIDs)
	}
	if fmt.Sprint(mock.bindingRuleDeletedIDs) != "[legacy-rule]" {
		t.Errorf("the legacy rule of the CR must still be removed, got %v", mock.bindingRuleDeletedIDs)
	}
}

// 24.7: a broken sibling does not fail the reconcile of an explicit CR; the stale clean-up releases nothing.
func TestApplyACL_ExplicitNextToBrokenSibling_StaleNotReleased(t *testing.T) {
	mock := &mockACLClient{
		roleListFunc: func(*consulApi.QueryOptions) ([]*consulApi.ACLRole, *consulApi.QueryMeta, error) {
			return []*consulApi.ACLRole{{ID: "role-stale", Name: "old", Description: owners("ns1")}}, nil, nil
		},
	}
	useACLClient(t, mock)

	cr := newConsulACL("app", "ns1", `{}`, true)
	broken := newConsulACL("broken", "ns1", `{not json`, true)
	r := &ConsulACLReconciler{Client: fake.NewClientBuilder().WithScheme(kvScheme()).WithObjects(cr, broken).Build(), OwnNamespace: "ns1"}
	if _, err := r.applyACL(context.TODO(), cr); err != nil {
		t.Fatalf("a broken sibling must not fail the reconcile: %v", err)
	}
	if len(mock.roleDeletedIDs) != 0 || len(mock.roleUpdated) != 0 {
		t.Errorf("nothing may be released while a sibling is unknown, deleted %v updated %v", mock.roleDeletedIDs, mock.roleUpdated)
	}
}
