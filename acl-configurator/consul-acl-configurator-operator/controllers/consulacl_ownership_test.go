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
	"fmt"
	"net"
	"testing"
	"time"

	consulacl "github.com/Netcracker/consul-acl-configurator/consul-acl-configurator-operator/api/v1alpha1"
	consulApi "github.com/hashicorp/consul/api"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

func useACLClient(t *testing.T, mock *mockACLClient) {
	orig := aclClient
	aclClient = mock
	t.Cleanup(func() { aclClient = orig })
}

func owners(namespaces ...string) string {
	return buildDescription("", namespaces)
}

// --- 21.6: StatusHolder.GetStatus ---

func TestGetStatus_InnerErrorHandlingItem_NotDuplicated(t *testing.T) {
	sh := StatusHolder{"innerErrorHandlingItem": "Some roles have not got a name"}
	if got := sh.GetStatus(); got != "Some roles have not got a name" {
		t.Errorf("unexpected status %q", got)
	}
}

// --- 21.8: the first network error is kept ---

func TestProcessRoles_NetworkErrorOnFirstRole_ErrorReturned(t *testing.T) {
	netErr := &net.OpError{Op: "dial", Net: "tcp", Err: fmt.Errorf("connection refused")}
	mock := &mockACLClient{
		roleCreateFunc: func(r *consulApi.ACLRole, _ *consulApi.WriteOptions) (*consulApi.ACLRole, *consulApi.WriteMeta, error) {
			if r.Name == "first" {
				return nil, nil, netErr
			}
			return r, nil, nil
		},
	}
	useACLClient(t, mock)

	roles := []ACLRoleAdapter{{Name: "first"}, {Name: "second"}}
	_, err := processRoles(roles, nil, "myapp", "staging", true)
	if err == nil {
		t.Fatal("network error of the first role must be returned even if the second role succeeds")
	}
}

func TestFirstNetError(t *testing.T) {
	netErr := &net.OpError{Op: "dial", Net: "tcp", Err: fmt.Errorf("refused")}
	if firstNetError(nil, fmt.Errorf("plain")) != nil {
		t.Error("non-network error must not be kept")
	}
	if firstNetError(nil, netErr) != netErr {
		t.Error("network error must be kept")
	}
	if firstNetError(netErr, nil) != netErr {
		t.Error("kept error must not be reset by a later success")
	}
}

// --- 21.5: owners are recorded on apply ---

func TestProcessRoles_AddsNamespaceToExistingOwners(t *testing.T) {
	mock := &mockACLClient{
		roleReadByNameFunc: func(name string, _ *consulApi.QueryOptions) (*consulApi.ACLRole, *consulApi.QueryMeta, error) {
			return &consulApi.ACLRole{ID: "role-1", Name: name, Description: "reader\n" + owners("ns1")}, nil, nil
		},
	}
	useACLClient(t, mock)

	roles := []ACLRoleAdapter{{Name: "shared-reader", Description: "reader"}}
	if _, err := processRoles(roles, nil, "app", "ns2", true); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(mock.roleUpdated) != 1 {
		t.Fatalf("expected one RoleUpdate, got %d", len(mock.roleUpdated))
	}
	if want := "reader\n" + owners("ns1", "ns2"); mock.roleUpdated[0].Description != want {
		t.Errorf("description: got %q, want %q", mock.roleUpdated[0].Description, want)
	}
}

// An ID from the spec must not drop the owners recorded in Consul.
func TestProcessRoles_IDInSpec_KeepsExistingOwners(t *testing.T) {
	mock := &mockACLClient{
		roleReadByNameFunc: func(name string, _ *consulApi.QueryOptions) (*consulApi.ACLRole, *consulApi.QueryMeta, error) {
			return &consulApi.ACLRole{ID: "role-1", Name: name, Description: owners("ns1")}, nil, nil
		},
	}
	useACLClient(t, mock)

	roles := []ACLRoleAdapter{{ID: "role-1", Name: "shared-reader"}}
	if _, err := processRoles(roles, nil, "app", "ns2", true); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if want := owners("ns1", "ns2"); len(mock.roleUpdated) != 1 || mock.roleUpdated[0].Description != want {
		t.Errorf("expected update with owners %q, got %v", want, mock.roleUpdated)
	}
}

func TestProcessBindRules_NewRule_OwnerRecorded(t *testing.T) {
	mock := &mockACLClient{}
	useACLClient(t, mock)

	rules := []ACLBindingRuleAdapter{{BindName: "shared-reader", Description: "rule"}}
	if _, err := processBindRules(rules, "app", "ns1", true); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if want := "rule\n" + owners("ns1"); len(mock.bindingRuleCreated) != 1 || mock.bindingRuleCreated[0].Description != want {
		t.Errorf("expected create with description %q, got %v", want, mock.bindingRuleCreated)
	}
}

func TestProcessBindRules_ExistingRule_OwnerMerged(t *testing.T) {
	mock := &mockACLClient{
		bindingRuleListFunc: func(am string, _ *consulApi.QueryOptions) ([]*consulApi.ACLBindingRule, *consulApi.QueryMeta, error) {
			return []*consulApi.ACLBindingRule{{ID: "br-1", BindName: "shared-reader", Description: owners("ns1")}}, nil, nil
		},
	}
	useACLClient(t, mock)

	rules := []ACLBindingRuleAdapter{{BindName: "shared-reader"}}
	if _, err := processBindRules(rules, "app", "ns2", true); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if want := owners("ns1", "ns2"); len(mock.bindingRuleUpdated) != 1 || mock.bindingRuleUpdated[0].Description != want {
		t.Errorf("expected update with owners %q, got %v", want, mock.bindingRuleUpdated)
	}
}

// --- 7.2 / 21.5: deletion releases shared roles and binding rules ---

func TestDeleteRoles_SharedRole_KeptAndTokensNotRevoked(t *testing.T) {
	tokensListed := false
	mock := &mockACLClient{
		roleReadByNameFunc: func(name string, _ *consulApi.QueryOptions) (*consulApi.ACLRole, *consulApi.QueryMeta, error) {
			return &consulApi.ACLRole{ID: "role-1", Name: name, Description: owners("ns1", "ns2")}, nil, nil
		},
		tokenListFilteredFunc: func(consulApi.ACLTokenFilterOptions, *consulApi.QueryOptions) ([]*consulApi.ACLTokenListEntry, *consulApi.QueryMeta, error) {
			tokensListed = true
			return []*consulApi.ACLTokenListEntry{{AccessorID: "acc-1"}}, nil, nil
		},
	}
	useACLClient(t, mock)

	cfg := &ACLConfig{Roles: []ACLRoleAdapter{{Name: "shared-reader"}}}
	if err := deleteRoles(cfg, "app", "ns1", true, nil); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(mock.roleDeletedIDs) != 0 {
		t.Errorf("shared role must not be deleted, got %v", mock.roleDeletedIDs)
	}
	if tokensListed || len(mock.tokenDeletedIDs) != 0 {
		t.Errorf("tokens of a shared role must not be revoked, got %v", mock.tokenDeletedIDs)
	}
	if want := owners("ns2"); len(mock.roleUpdated) != 1 || mock.roleUpdated[0].Description != want {
		t.Errorf("expected owners %q after release, got %v", want, mock.roleUpdated)
	}
}

func TestDeleteRoles_LastOwner_TokensRevokedAndRoleDeleted(t *testing.T) {
	mock := &mockACLClient{
		roleReadByNameFunc: func(name string, _ *consulApi.QueryOptions) (*consulApi.ACLRole, *consulApi.QueryMeta, error) {
			return &consulApi.ACLRole{ID: "role-1", Name: name, Description: owners("ns1")}, nil, nil
		},
		tokenListFilteredFunc: func(consulApi.ACLTokenFilterOptions, *consulApi.QueryOptions) ([]*consulApi.ACLTokenListEntry, *consulApi.QueryMeta, error) {
			return []*consulApi.ACLTokenListEntry{{AccessorID: "acc-1"}}, nil, nil
		},
	}
	useACLClient(t, mock)

	cfg := &ACLConfig{Roles: []ACLRoleAdapter{{Name: "shared-reader"}}}
	if err := deleteRoles(cfg, "app", "ns1", true, nil); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(mock.roleDeletedIDs) != 1 || mock.roleDeletedIDs[0] != "role-1" {
		t.Errorf("expected role-1 deleted, got %v", mock.roleDeletedIDs)
	}
	if len(mock.tokenDeletedIDs) != 1 || mock.tokenDeletedIDs[0] != "acc-1" {
		t.Errorf("expected token acc-1 revoked, got %v", mock.tokenDeletedIDs)
	}
}

// A role created before the upgrade has no owner marker and is deleted as before.
func TestDeleteRoles_NoMarker_DeletedAsBefore(t *testing.T) {
	mock := &mockACLClient{
		roleReadByNameFunc: func(name string, _ *consulApi.QueryOptions) (*consulApi.ACLRole, *consulApi.QueryMeta, error) {
			return &consulApi.ACLRole{ID: "role-1", Name: name, Description: "legacy"}, nil, nil
		},
	}
	useACLClient(t, mock)

	cfg := &ACLConfig{Roles: []ACLRoleAdapter{{Name: "legacy-role"}}}
	if err := deleteRoles(cfg, "app", "ns1", true, nil); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(mock.roleDeletedIDs) != 1 {
		t.Errorf("expected legacy role deleted, got %v", mock.roleDeletedIDs)
	}
}

// Another CR of the same namespace still declares the role: the namespace stays an owner.
func TestDeleteRoles_DeclaredBySibling_Skipped(t *testing.T) {
	mock := &mockACLClient{
		roleReadByNameFunc: func(name string, _ *consulApi.QueryOptions) (*consulApi.ACLRole, *consulApi.QueryMeta, error) {
			return &consulApi.ACLRole{ID: "role-1", Name: name, Description: owners("ns1")}, nil, nil
		},
	}
	useACLClient(t, mock)

	siblings := newDeclaredEntities()
	siblings.add(&ACLConfig{Roles: []ACLRoleAdapter{{Name: "shared-reader"}}}, "other", "ns1", true)
	cfg := &ACLConfig{Roles: []ACLRoleAdapter{{Name: "shared-reader"}}}
	if err := deleteRoles(cfg, "app", "ns1", true, siblings); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(mock.roleDeletedIDs) != 0 || len(mock.roleUpdated) != 0 {
		t.Errorf("role declared by a sibling must not be touched, deleted %v, updated %v", mock.roleDeletedIDs, mock.roleUpdated)
	}
}

func TestDeleteBindingRules_SharedRule_Kept(t *testing.T) {
	mock := &mockACLClient{
		bindingRuleListFunc: func(am string, _ *consulApi.QueryOptions) ([]*consulApi.ACLBindingRule, *consulApi.QueryMeta, error) {
			return []*consulApi.ACLBindingRule{{ID: "br-1", BindName: "shared-reader", Description: owners("ns1", "ns2")}}, nil, nil
		},
	}
	useACLClient(t, mock)

	cfg := &ACLConfig{BindRules: []ACLBindingRuleAdapter{{BindName: "shared-reader"}}}
	if err := deleteBindingRules(cfg, "app", "ns1", true, nil); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(mock.bindingRuleDeletedIDs) != 0 {
		t.Errorf("shared binding rule must not be deleted, got %v", mock.bindingRuleDeletedIDs)
	}
	if want := owners("ns2"); len(mock.bindingRuleUpdated) != 1 || mock.bindingRuleUpdated[0].Description != want {
		t.Errorf("expected owners %q after release, got %v", want, mock.bindingRuleUpdated)
	}
}

func TestDeletePolicies_DeclaredBySibling_Skipped(t *testing.T) {
	mock := &mockACLClient{
		policyReadByNameFunc: func(name string, _ *consulApi.QueryOptions) (*consulApi.ACLPolicy, *consulApi.QueryMeta, error) {
			return &consulApi.ACLPolicy{ID: "pol-1", Name: name, Description: owners("ns1")}, nil, nil
		},
	}
	useACLClient(t, mock)

	siblings := newDeclaredEntities()
	siblings.add(&ACLConfig{Policies: []consulApi.ACLPolicy{{Name: "shared-policy"}}}, "other", "ns1", true)
	cfg := &ACLConfig{Policies: []consulApi.ACLPolicy{{Name: "shared-policy"}}}
	if err := deletePolicies(cfg, "app", "ns1", true, siblings); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(mock.policyDeletedIDs) != 0 || len(mock.policyUpdated) != 0 {
		t.Errorf("policy declared by a sibling must not be touched, deleted %v, updated %v", mock.policyDeletedIDs, mock.policyUpdated)
	}
}

// --- 21.4 / 21.5: stale cleanup in explicit mode uses the owner list in Consul ---

func TestRemoveStaleExplicit_LastOwnerPolicy_DeletedWithoutStatus(t *testing.T) {
	mock := &mockACLClient{
		policyListFunc: func(*consulApi.QueryOptions) ([]*consulApi.ACLPolicyListEntry, *consulApi.QueryMeta, error) {
			return []*consulApi.ACLPolicyListEntry{
				{ID: "pol-keep", Name: "kept", Description: owners("ns1")},
				{ID: "pol-stale", Name: "removed", Description: owners("ns1")},
			}, nil, nil
		},
		policyReadByNameFunc: func(name string, _ *consulApi.QueryOptions) (*consulApi.ACLPolicy, *consulApi.QueryMeta, error) {
			return &consulApi.ACLPolicy{ID: "pol-stale", Name: name, Description: owners("ns1")}, nil, nil
		},
	}
	useACLClient(t, mock)

	cfg := &ACLConfig{Policies: []consulApi.ACLPolicy{{Name: "kept"}}}
	if err := removeStaleEntities(cfg, "app", "ns1", true, nil); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(mock.policyDeletedIDs) != 1 || mock.policyDeletedIDs[0] != "pol-stale" {
		t.Errorf("expected pol-stale deleted, got %v", mock.policyDeletedIDs)
	}
}

func TestRemoveStaleExplicit_SharedPolicy_OwnerRemovedAndRulesKept(t *testing.T) {
	mock := &mockACLClient{
		policyListFunc: func(*consulApi.QueryOptions) ([]*consulApi.ACLPolicyListEntry, *consulApi.QueryMeta, error) {
			return []*consulApi.ACLPolicyListEntry{{ID: "pol-1", Name: "shared", Description: owners("ns1", "ns2")}}, nil, nil
		},
		policyReadByNameFunc: func(name string, _ *consulApi.QueryOptions) (*consulApi.ACLPolicy, *consulApi.QueryMeta, error) {
			return &consulApi.ACLPolicy{ID: "pol-1", Name: name, Rules: `key_prefix "" { policy = "read" }`, Description: owners("ns1", "ns2")}, nil, nil
		},
	}
	useACLClient(t, mock)

	if err := removeStaleEntities(&ACLConfig{}, "app", "ns1", true, nil); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(mock.policyDeletedIDs) != 0 {
		t.Errorf("shared policy must not be deleted, got %v", mock.policyDeletedIDs)
	}
	if len(mock.policyUpdated) != 1 {
		t.Fatalf("expected one PolicyUpdate, got %d", len(mock.policyUpdated))
	}
	if got := mock.policyUpdated[0]; got.Description != owners("ns2") || got.Rules == "" {
		t.Errorf("expected owners %q and rules kept, got %+v", owners("ns2"), got)
	}
}

func TestRemoveStaleExplicit_Role_TokensRevokedAndDeleted(t *testing.T) {
	mock := &mockACLClient{
		roleListFunc: func(*consulApi.QueryOptions) ([]*consulApi.ACLRole, *consulApi.QueryMeta, error) {
			return []*consulApi.ACLRole{
				{ID: "role-keep", Name: "kept", Description: owners("ns1")},
				{ID: "role-stale", Name: "removed", Description: owners("ns1")},
			}, nil, nil
		},
		tokenListFilteredFunc: func(f consulApi.ACLTokenFilterOptions, _ *consulApi.QueryOptions) ([]*consulApi.ACLTokenListEntry, *consulApi.QueryMeta, error) {
			return []*consulApi.ACLTokenListEntry{{AccessorID: "acc-" + f.Role}}, nil, nil
		},
	}
	useACLClient(t, mock)

	cfg := &ACLConfig{Roles: []ACLRoleAdapter{{Name: "kept"}}}
	if err := removeStaleEntities(cfg, "app", "ns1", true, nil); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(mock.roleDeletedIDs) != 1 || mock.roleDeletedIDs[0] != "role-stale" {
		t.Errorf("expected role-stale deleted, got %v", mock.roleDeletedIDs)
	}
	if len(mock.tokenDeletedIDs) != 1 || mock.tokenDeletedIDs[0] != "acc-role-stale" {
		t.Errorf("expected tokens of role-stale revoked, got %v", mock.tokenDeletedIDs)
	}
}

func TestRemoveStaleExplicit_SharedBindingRule_OwnerRemoved(t *testing.T) {
	mock := &mockACLClient{
		bindingRuleListFunc: func(am string, _ *consulApi.QueryOptions) ([]*consulApi.ACLBindingRule, *consulApi.QueryMeta, error) {
			return []*consulApi.ACLBindingRule{{ID: "br-1", BindName: "shared", Description: owners("ns1", "ns2")}}, nil, nil
		},
	}
	useACLClient(t, mock)

	if err := removeStaleEntities(&ACLConfig{}, "app", "ns1", true, nil); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(mock.bindingRuleDeletedIDs) != 0 {
		t.Errorf("shared binding rule must not be deleted, got %v", mock.bindingRuleDeletedIDs)
	}
	if want := owners("ns2"); len(mock.bindingRuleUpdated) != 1 || mock.bindingRuleUpdated[0].Description != want {
		t.Errorf("expected owners %q, got %v", want, mock.bindingRuleUpdated)
	}
}

// Entities without the marker, owned by other namespaces only, or declared by a sibling
// CR of the same namespace are not touched.
func TestRemoveStaleExplicit_NotOwnedOrDeclaredBySibling_Untouched(t *testing.T) {
	mock := &mockACLClient{
		roleListFunc: func(*consulApi.QueryOptions) ([]*consulApi.ACLRole, *consulApi.QueryMeta, error) {
			return []*consulApi.ACLRole{
				{ID: "role-legacy", Name: "legacy", Description: "created before the upgrade"},
				{ID: "role-foreign", Name: "foreign", Description: owners("ns2")},
				{ID: "role-sibling", Name: "sibling", Description: owners("ns1")},
				{ID: "role-sibling-prefixed", Name: "other_ns1_reader", Description: owners("ns1")},
			}, nil, nil
		},
	}
	useACLClient(t, mock)

	siblings := newDeclaredEntities()
	siblings.add(&ACLConfig{Roles: []ACLRoleAdapter{{Name: "sibling"}}}, "explicit", "ns1", true)
	siblings.add(&ACLConfig{Roles: []ACLRoleAdapter{{Name: "reader"}}}, "other", "ns1", false)
	if err := removeStaleEntities(&ACLConfig{}, "app", "ns1", true, siblings); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(mock.roleDeletedIDs) != 0 || len(mock.roleUpdated) != 0 {
		t.Errorf("no roles should be touched, deleted %v, updated %v", mock.roleDeletedIDs, mock.roleUpdated)
	}
}

// --- siblingEntities ---

func newConsulACL(name, namespace, json string, explicitName bool) *consulacl.ConsulACL {
	return &consulacl.ConsulACL{
		ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: namespace},
		Spec:       consulacl.ConsulACLSpec{ACL: &consulacl.ACL{Json: json, ExplicitName: explicitName}},
	}
}

func TestSiblingEntities_SameNamespaceOnly(t *testing.T) {
	self := newConsulACL("self", "ns1", `{"roles":[{"Name":"own"}]}`, true)
	explicit := newConsulACL("explicit", "ns1", `{"roles":[{"Name":"shared"}]}`, true)
	prefixed := newConsulACL("prefixed", "ns1", `{"policies":[{"Name":"reader"}]}`, false)
	otherNs := newConsulACL("other", "ns2", `{"roles":[{"Name":"other-ns-role"}]}`, true)
	deleting := newConsulACL("deleting", "ns1", `{"roles":[{"Name":"deleting-role"}]}`, true)
	now := metav1.NewTime(time.Now())
	deleting.DeletionTimestamp = &now
	deleting.Finalizers = []string{consulAclFinalizer}
	foreignOperator := newConsulACL("foreign", "ns1", `{"roles":[{"Name":"foreign-role"}]}`, true)
	foreignOperator.Spec.ACL.OperatorNamespace = "another-consul"

	r := &ConsulACLReconciler{
		Client:       fake.NewClientBuilder().WithScheme(kvScheme()).WithObjects(self, explicit, prefixed, otherNs, deleting, foreignOperator).Build(),
		OwnNamespace: "ns1",
	}
	siblings, err := r.siblingEntities(self)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !siblings.hasRole("shared") || !siblings.hasPolicy("prefixed_ns1_reader") {
		t.Errorf("entities of siblings in the same namespace must be collected: %+v", siblings)
	}
	for _, role := range []string{"own", "other-ns-role", "deleting-role", "foreign-role"} {
		if siblings.hasRole(role) {
			t.Errorf("role %q must not be counted as a sibling entity", role)
		}
	}
}

func TestSiblingEntities_InvalidSiblingConfig_ErrorReturned(t *testing.T) {
	self := newConsulACL("self", "ns1", `{}`, true)
	broken := newConsulACL("broken", "ns1", `{not json`, true)
	r := &ConsulACLReconciler{
		Client:       fake.NewClientBuilder().WithScheme(kvScheme()).WithObjects(self, broken).Build(),
		OwnNamespace: "ns1",
	}
	if _, err := r.siblingEntities(self); err == nil {
		t.Fatal("expected an error when a sibling configuration can not be parsed")
	}
}
