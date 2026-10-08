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
	"strings"
	"testing"

	consulacl "github.com/Netcracker/consul-acl-configurator/consul-acl-configurator-operator/api/v1alpha1"
	consulApi "github.com/hashicorp/consul/api"
	apimeta "k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

// reconcileACLWithStatus runs one reconcile of a CR that already has a status and returns the CR after it.
func reconcileACLWithStatus(t *testing.T, json string) *consulacl.ConsulACL {
	cr := &consulacl.ConsulACL{
		ObjectMeta: metav1.ObjectMeta{Name: "app", Namespace: "ns", Finalizers: []string{consulAclFinalizer}},
		Spec:       consulacl.ConsulACLSpec{ACL: &consulacl.ACL{Name: "app", Json: json}},
		Status: consulacl.ConsulACLStatus{
			PoliciesStatus:  "previous policies",
			RolesStatus:     "previous roles",
			BindRulesStatus: "previous rules",
		},
	}
	fakeClient := fake.NewClientBuilder().WithScheme(kvScheme()).WithObjects(cr).WithStatusSubresource(cr).Build()
	r := &ConsulACLReconciler{Client: fakeClient, OwnNamespace: "ns"}
	req := ctrl.Request{NamespacedName: types.NamespacedName{Name: "app", Namespace: "ns"}}
	result, err := r.Reconcile(context.TODO(), req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.RequeueAfter == 0 && periodTime != 0 {
		t.Error("a failed reconcile must be requeued")
	}
	after := &consulacl.ConsulACL{}
	if err = fakeClient.Get(context.TODO(), req.NamespacedName, after); err != nil {
		t.Fatal(err)
	}
	return after
}

// 24.1: a transient error on roles keeps the status of the binding rules (not reached), writes the
// processed policies and the partial status of the roles, and sets Successful=False.
func TestReconcile_TransientErrorOnRoles_StatusKept(t *testing.T) {
	mock := &mockACLClient{
		roleCreateFunc: func(*consulApi.ACLRole, *consulApi.WriteOptions) (*consulApi.ACLRole, *consulApi.WriteMeta, error) {
			return nil, nil, noLeader
		},
	}
	useACLClient(t, mock)

	cr := reconcileACLWithStatus(t, `{"policies":[{"Name":"p"}],"roles":[{"Name":"r","policy_names":["p"]}],"bind_rules":[{"BindName":"r"}]}`)

	if !strings.Contains(cr.Status.PoliciesStatus, "app_ns_p: created") {
		t.Errorf("policies status must be updated, got %q", cr.Status.PoliciesStatus)
	}
	if !strings.Contains(cr.Status.RolesStatus, "app_ns_r: error:") {
		t.Errorf("roles status must show the failed role, got %q", cr.Status.RolesStatus)
	}
	if cr.Status.BindRulesStatus != "previous rules" {
		t.Errorf("status of a stage that was not reached must be kept, got %q", cr.Status.BindRulesStatus)
	}
	if c := apimeta.FindStatusCondition(cr.Status.Conditions, "Successful"); c == nil || c.Status != metav1.ConditionFalse {
		t.Errorf("expected Successful=False, got %+v", c)
	}
}

// 24.1: an invalid configuration keeps the whole status and sets Successful=False.
func TestReconcile_InvalidConfig_StatusKept(t *testing.T) {
	useACLClient(t, &mockACLClient{})

	cr := reconcileACLWithStatus(t, `{not json`)

	if cr.Status.PoliciesStatus != "previous policies" || cr.Status.RolesStatus != "previous roles" || cr.Status.BindRulesStatus != "previous rules" {
		t.Errorf("status must be kept, got %+v", cr.Status)
	}
	if c := apimeta.FindStatusCondition(cr.Status.Conditions, "Successful"); c == nil || c.Status != metav1.ConditionFalse {
		t.Errorf("expected Successful=False, got %+v", c)
	}
}
