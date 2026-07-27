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
	"testing"

	consulacl "github.com/Netcracker/consul-acl-configurator/consul-acl-configurator-operator/api/v1alpha1"
	consulApi "github.com/hashicorp/consul/api"
)

// mockACLClient is a test double for consulACLClient.
type mockACLClient struct {
	// Role
	roleReadByNameFunc  func(string, *consulApi.QueryOptions) (*consulApi.ACLRole, *consulApi.QueryMeta, error)
	roleListFunc        func(*consulApi.QueryOptions) ([]*consulApi.ACLRole, *consulApi.QueryMeta, error)
	roleCreateCalled    bool
	roleUpdateCalled    bool
	roleDeletedIDs      []string
	roleCreateFunc      func(*consulApi.ACLRole, *consulApi.WriteOptions) (*consulApi.ACLRole, *consulApi.WriteMeta, error)
	roleUpdateFunc      func(*consulApi.ACLRole, *consulApi.WriteOptions) (*consulApi.ACLRole, *consulApi.WriteMeta, error)
	// BindingRule
	bindingRuleListFunc     func(string, *consulApi.QueryOptions) ([]*consulApi.ACLBindingRule, *consulApi.QueryMeta, error)
	bindingRuleCreateCalled bool
	bindingRuleUpdateCalled bool
	bindingRuleDeletedIDs   []string
	// Policy
	policyListFunc   func(*consulApi.QueryOptions) ([]*consulApi.ACLPolicyListEntry, *consulApi.QueryMeta, error)
	policyDeletedIDs []string
}

func (m *mockACLClient) PolicyCreate(p *consulApi.ACLPolicy, q *consulApi.WriteOptions) (*consulApi.ACLPolicy, *consulApi.WriteMeta, error) {
	return p, nil, nil
}
func (m *mockACLClient) PolicyUpdate(p *consulApi.ACLPolicy, q *consulApi.WriteOptions) (*consulApi.ACLPolicy, *consulApi.WriteMeta, error) {
	return p, nil, nil
}
func (m *mockACLClient) PolicyReadByName(name string, q *consulApi.QueryOptions) (*consulApi.ACLPolicy, *consulApi.QueryMeta, error) {
	return nil, nil, nil
}
func (m *mockACLClient) PolicyDelete(id string, q *consulApi.WriteOptions) (*consulApi.WriteMeta, error) {
	m.policyDeletedIDs = append(m.policyDeletedIDs, id)
	return nil, nil
}
func (m *mockACLClient) PolicyList(q *consulApi.QueryOptions) ([]*consulApi.ACLPolicyListEntry, *consulApi.QueryMeta, error) {
	if m.policyListFunc != nil {
		return m.policyListFunc(q)
	}
	return nil, nil, nil
}
func (m *mockACLClient) RoleCreate(r *consulApi.ACLRole, q *consulApi.WriteOptions) (*consulApi.ACLRole, *consulApi.WriteMeta, error) {
	m.roleCreateCalled = true
	if m.roleCreateFunc != nil {
		return m.roleCreateFunc(r, q)
	}
	return r, nil, nil
}
func (m *mockACLClient) RoleUpdate(r *consulApi.ACLRole, q *consulApi.WriteOptions) (*consulApi.ACLRole, *consulApi.WriteMeta, error) {
	m.roleUpdateCalled = true
	if m.roleUpdateFunc != nil {
		return m.roleUpdateFunc(r, q)
	}
	return r, nil, nil
}
func (m *mockACLClient) RoleReadByName(name string, q *consulApi.QueryOptions) (*consulApi.ACLRole, *consulApi.QueryMeta, error) {
	if m.roleReadByNameFunc != nil {
		return m.roleReadByNameFunc(name, q)
	}
	return nil, nil, nil
}
func (m *mockACLClient) RoleList(q *consulApi.QueryOptions) ([]*consulApi.ACLRole, *consulApi.QueryMeta, error) {
	if m.roleListFunc != nil {
		return m.roleListFunc(q)
	}
	return nil, nil, nil
}
func (m *mockACLClient) RoleDelete(id string, q *consulApi.WriteOptions) (*consulApi.WriteMeta, error) {
	m.roleDeletedIDs = append(m.roleDeletedIDs, id)
	return nil, nil
}
func (m *mockACLClient) BindingRuleCreate(br *consulApi.ACLBindingRule, q *consulApi.WriteOptions) (*consulApi.ACLBindingRule, *consulApi.WriteMeta, error) {
	m.bindingRuleCreateCalled = true
	return br, nil, nil
}
func (m *mockACLClient) BindingRuleUpdate(br *consulApi.ACLBindingRule, q *consulApi.WriteOptions) (*consulApi.ACLBindingRule, *consulApi.WriteMeta, error) {
	m.bindingRuleUpdateCalled = true
	return br, nil, nil
}
func (m *mockACLClient) BindingRuleList(am string, q *consulApi.QueryOptions) ([]*consulApi.ACLBindingRule, *consulApi.QueryMeta, error) {
	if m.bindingRuleListFunc != nil {
		return m.bindingRuleListFunc(am, q)
	}
	return nil, nil, nil
}
func (m *mockACLClient) BindingRuleDelete(id string, q *consulApi.WriteOptions) (*consulApi.WriteMeta, error) {
	m.bindingRuleDeletedIDs = append(m.bindingRuleDeletedIDs, id)
	return nil, nil
}

// --- Tests for convertRoleAdapterToRole ---

// 2.3a: ExplicitName false → prefixed name
func TestConvertRoleAdapterToRole_Prefixed(t *testing.T) {
	adapter := ACLRoleAdapter{Name: "reader"}
	role := convertRoleAdapterToRole(adapter, nil, "myapp", "staging", false)
	want := "myapp_staging_reader"
	if role.Name != want {
		t.Errorf("got %q, want %q", role.Name, want)
	}
}

// 2.3b: ExplicitName true → verbatim name
func TestConvertRoleAdapterToRole_Explicit(t *testing.T) {
	adapter := ACLRoleAdapter{Name: "staging_myservice"}
	role := convertRoleAdapterToRole(adapter, nil, "myapp", "staging", true)
	want := "staging_myservice"
	if role.Name != want {
		t.Errorf("got %q, want %q", role.Name, want)
	}
}

// --- Tests for convertBindRuleAdapterToBindRule ---

// 3.3a: ExplicitName false → prefixed BindName
func TestConvertBindRuleAdapterToBindRule_Prefixed(t *testing.T) {
	adapter := ACLBindingRuleAdapter{BindName: "reader"}
	rule := convertBindRuleAdapterToBindRule(adapter, "myapp", "staging", false)
	want := "myapp_staging_reader"
	if rule.BindName != want {
		t.Errorf("got %q, want %q", rule.BindName, want)
	}
}

// 3.3b: ExplicitName true → verbatim BindName
func TestConvertBindRuleAdapterToBindRule_Explicit(t *testing.T) {
	adapter := ACLBindingRuleAdapter{BindName: "${serviceaccount.namespace}_${serviceaccount.name}"}
	rule := convertBindRuleAdapterToBindRule(adapter, "myapp", "staging", true)
	want := "${serviceaccount.namespace}_${serviceaccount.name}"
	if rule.BindName != want {
		t.Errorf("got %q, want %q", rule.BindName, want)
	}
}

// --- Tests for AuthMethod per binding rule ---

// 4.3a: empty AuthMethod falls back to global
func TestConvertBindRuleAdapterToBindRule_GlobalAuthMethod(t *testing.T) {
	orig := authMethod
	authMethod = "cluster-k8s-auth-method"
	defer func() { authMethod = orig }()

	adapter := ACLBindingRuleAdapter{BindName: "reader"}
	rule := convertBindRuleAdapterToBindRule(adapter, "myapp", "staging", false)
	if rule.AuthMethod != "cluster-k8s-auth-method" {
		t.Errorf("got %q, want %q", rule.AuthMethod, "cluster-k8s-auth-method")
	}
}

// 4.3b: non-empty AuthMethod overrides global
func TestConvertBindRuleAdapterToBindRule_PerRuleAuthMethod(t *testing.T) {
	orig := authMethod
	authMethod = "cluster-k8s-auth-method"
	defer func() { authMethod = orig }()

	adapter := ACLBindingRuleAdapter{BindName: "reader", AuthMethod: "new_auth_method"}
	rule := convertBindRuleAdapterToBindRule(adapter, "myapp", "staging", false)
	if rule.AuthMethod != "new_auth_method" {
		t.Errorf("got %q, want %q", rule.AuthMethod, "new_auth_method")
	}
}

// 4.3c: two rules in one CR each use their respective auth methods
func TestConvertBindRuleAdapterToBindRule_TwoRulesDifferentAuthMethods(t *testing.T) {
	orig := authMethod
	authMethod = "global-auth"
	defer func() { authMethod = orig }()

	adapters := []ACLBindingRuleAdapter{
		{BindName: "rule-a"},
		{BindName: "rule-b", AuthMethod: "custom-auth"},
	}
	ruleA := convertBindRuleAdapterToBindRule(adapters[0], "myapp", "staging", false)
	ruleB := convertBindRuleAdapterToBindRule(adapters[1], "myapp", "staging", false)

	if ruleA.AuthMethod != "global-auth" {
		t.Errorf("rule-a: got %q, want %q", ruleA.AuthMethod, "global-auth")
	}
	if ruleB.AuthMethod != "custom-auth" {
		t.Errorf("rule-b: got %q, want %q", ruleB.AuthMethod, "custom-auth")
	}
}

// 2.3c: pre-existing role found by verbatim name → update called, not create
func TestProcessRoles_ExplicitName_ExistingRoleTriggersUpdate(t *testing.T) {
	existingID := "existing-role-uuid"
	mock := &mockACLClient{
		roleReadByNameFunc: func(name string, _ *consulApi.QueryOptions) (*consulApi.ACLRole, *consulApi.QueryMeta, error) {
			if name == "staging_myservice" {
				return &consulApi.ACLRole{ID: existingID, Name: name}, nil, nil
			}
			return nil, nil, fmt.Errorf("ACL not found")
		},
	}

	orig := aclClient
	aclClient = mock
	defer func() { aclClient = orig }()

	roles := []ACLRoleAdapter{{Name: "staging_myservice"}}
	_, err := processRoles(roles, nil, "myapp", "staging", true)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if mock.roleCreateCalled {
		t.Error("RoleCreate was called; expected RoleUpdate for a pre-existing role")
	}
	if !mock.roleUpdateCalled {
		t.Error("RoleUpdate was not called; expected it for a pre-existing role")
	}
}

// --- Tests for idempotent binding-rule reconciliation (section 5) ---

// 5.3a: rule absent → BindingRuleCreate called
func TestProcessBindRules_RuleAbsent_CreateCalled(t *testing.T) {
	mock := &mockACLClient{
		// list returns empty — rule doesn't exist yet
		bindingRuleListFunc: func(am string, _ *consulApi.QueryOptions) ([]*consulApi.ACLBindingRule, *consulApi.QueryMeta, error) {
			return nil, nil, nil
		},
	}
	orig := aclClient
	aclClient = mock
	defer func() { aclClient = orig }()

	rules := []ACLBindingRuleAdapter{{BindName: "reader"}}
	_, err := processBindRules(rules, "myapp", "staging", false)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !mock.bindingRuleCreateCalled {
		t.Error("BindingRuleCreate was not called; expected it when rule is absent")
	}
	if mock.bindingRuleUpdateCalled {
		t.Error("BindingRuleUpdate should not have been called")
	}
}

// 5.3b: rule present under same auth method → BindingRuleUpdate called with existing ID
func TestProcessBindRules_RulePresent_UpdateCalledWithID(t *testing.T) {
	mock := &mockACLClient{
		bindingRuleListFunc: func(am string, _ *consulApi.QueryOptions) ([]*consulApi.ACLBindingRule, *consulApi.QueryMeta, error) {
			return []*consulApi.ACLBindingRule{
				{ID: "existing-br-uuid", BindName: "myapp_staging_reader", AuthMethod: am},
			}, nil, nil
		},
	}
	orig := aclClient
	aclClient = mock
	defer func() { aclClient = orig }()

	rules := []ACLBindingRuleAdapter{{BindName: "reader"}}
	_, err := processBindRules(rules, "myapp", "staging", false)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if mock.bindingRuleCreateCalled {
		t.Error("BindingRuleCreate should not have been called for an existing rule")
	}
	if !mock.bindingRuleUpdateCalled {
		t.Error("BindingRuleUpdate was not called; expected update for existing rule")
	}
}

// 5.3c: rule present under different auth method → not matched, BindingRuleCreate called
func TestProcessBindRules_RulePresentUnderDifferentAuthMethod_CreateCalled(t *testing.T) {
	mock := &mockACLClient{
		// list for "global-auth" returns nothing; rule is registered under "other-auth"
		bindingRuleListFunc: func(am string, _ *consulApi.QueryOptions) ([]*consulApi.ACLBindingRule, *consulApi.QueryMeta, error) {
			if am == "other-auth" {
				return []*consulApi.ACLBindingRule{
					{ID: "some-id", BindName: "myapp_staging_reader", AuthMethod: "other-auth"},
				}, nil, nil
			}
			return nil, nil, nil
		},
	}
	orig := aclClient
	aclClient = mock
	defer func() { aclClient = orig }()

	origAuthMethod := authMethod
	authMethod = "global-auth"
	defer func() { authMethod = origAuthMethod }()

	// rule has no per-rule override → uses global-auth → list returns empty → create
	rules := []ACLBindingRuleAdapter{{BindName: "reader"}}
	_, err := processBindRules(rules, "myapp", "staging", false)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !mock.bindingRuleCreateCalled {
		t.Error("BindingRuleCreate was not called; expected create since rule under different auth method is not matched")
	}
	if mock.bindingRuleUpdateCalled {
		t.Error("BindingRuleUpdate should not have been called")
	}
}

// --- Tests for stale entity removal on update (section 6) ---

func newStaleTestMock(
	policies []*consulApi.ACLPolicyListEntry,
	roles []*consulApi.ACLRole,
	bindRules []*consulApi.ACLBindingRule,
) *mockACLClient {
	return &mockACLClient{
		policyListFunc: func(_ *consulApi.QueryOptions) ([]*consulApi.ACLPolicyListEntry, *consulApi.QueryMeta, error) {
			return policies, nil, nil
		},
		roleListFunc: func(_ *consulApi.QueryOptions) ([]*consulApi.ACLRole, *consulApi.QueryMeta, error) {
			return roles, nil, nil
		},
		bindingRuleListFunc: func(_ string, _ *consulApi.QueryOptions) ([]*consulApi.ACLBindingRule, *consulApi.QueryMeta, error) {
			return bindRules, nil, nil
		},
	}
}

// 6.3a: policy removed from spec is deleted from Consul
func TestRemoveStaleEntities_StalePolicyDeleted(t *testing.T) {
	mock := newStaleTestMock(
		[]*consulApi.ACLPolicyListEntry{
			{ID: "pol-keep", Name: "myapp_staging_reader"},
			{ID: "pol-stale", Name: "myapp_staging_stale"},
		},
		nil, nil,
	)
	orig := aclClient
	aclClient = mock
	defer func() { aclClient = orig }()

	cfg := &ACLConfig{Policies: []consulApi.ACLPolicy{{Name: "reader"}}}
	if err := removeStaleEntities(cfg, "myapp", "staging", false); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(mock.policyDeletedIDs) != 1 || mock.policyDeletedIDs[0] != "pol-stale" {
		t.Errorf("expected pol-stale deleted, got %v", mock.policyDeletedIDs)
	}
}

// 6.3b: role removed from spec is deleted
func TestRemoveStaleEntities_StaleRoleDeleted(t *testing.T) {
	mock := newStaleTestMock(
		nil,
		[]*consulApi.ACLRole{
			{ID: "role-keep", Name: "myapp_staging_writer"},
			{ID: "role-stale", Name: "myapp_staging_old"},
		},
		nil,
	)
	orig := aclClient
	aclClient = mock
	defer func() { aclClient = orig }()

	cfg := &ACLConfig{Roles: []ACLRoleAdapter{{Name: "writer"}}}
	if err := removeStaleEntities(cfg, "myapp", "staging", false); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(mock.roleDeletedIDs) != 1 || mock.roleDeletedIDs[0] != "role-stale" {
		t.Errorf("expected role-stale deleted, got %v", mock.roleDeletedIDs)
	}
}

// 6.3c: binding rule removed from spec is deleted
func TestRemoveStaleEntities_StaleBindingRuleDeleted(t *testing.T) {
	mock := newStaleTestMock(
		nil, nil,
		[]*consulApi.ACLBindingRule{
			{ID: "br-keep", BindName: "myapp_staging_active"},
			{ID: "br-stale", BindName: "myapp_staging_old"},
		},
	)
	orig := aclClient
	aclClient = mock
	defer func() { aclClient = orig }()

	cfg := &ACLConfig{BindRules: []ACLBindingRuleAdapter{{BindName: "active"}}}
	if err := removeStaleEntities(cfg, "myapp", "staging", false); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(mock.bindingRuleDeletedIDs) != 1 || mock.bindingRuleDeletedIDs[0] != "br-stale" {
		t.Errorf("expected br-stale deleted, got %v", mock.bindingRuleDeletedIDs)
	}
}

// --- Tests for deleteBindingRules with per-rule AuthMethod (section 8) ---

// 8.3a: all rules deleted when all use global AuthMethod
func TestDeleteBindingRules_GlobalAuthMethod_AllDeleted(t *testing.T) {
	origAuthMethod := authMethod
	authMethod = "global-auth"
	defer func() { authMethod = origAuthMethod }()

	mock := &mockACLClient{
		bindingRuleListFunc: func(am string, _ *consulApi.QueryOptions) ([]*consulApi.ACLBindingRule, *consulApi.QueryMeta, error) {
			if am == "global-auth" {
				return []*consulApi.ACLBindingRule{
					{ID: "br-1", BindName: "myapp_staging_svc-a"},
					{ID: "br-2", BindName: "myapp_staging_svc-b"},
				}, nil, nil
			}
			return nil, nil, nil
		},
	}
	orig := aclClient
	aclClient = mock
	defer func() { aclClient = orig }()

	cfg := &ACLConfig{
		BindRules: []ACLBindingRuleAdapter{
			{BindName: "svc-a"},
			{BindName: "svc-b"},
		},
	}
	if err := deleteBindingRules(cfg, "myapp", "staging", false); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(mock.bindingRuleDeletedIDs) != 2 {
		t.Errorf("expected 2 rules deleted, got %v", mock.bindingRuleDeletedIDs)
	}
}

// 8.3b: rules deleted under both global and override AuthMethod
func TestDeleteBindingRules_MixedAuthMethods_AllDeleted(t *testing.T) {
	origAuthMethod := authMethod
	authMethod = "global-auth"
	defer func() { authMethod = origAuthMethod }()

	mock := &mockACLClient{
		bindingRuleListFunc: func(am string, _ *consulApi.QueryOptions) ([]*consulApi.ACLBindingRule, *consulApi.QueryMeta, error) {
			switch am {
			case "global-auth":
				return []*consulApi.ACLBindingRule{
					{ID: "br-global", BindName: "myapp_staging_svc-a"},
				}, nil, nil
			case "custom-auth":
				return []*consulApi.ACLBindingRule{
					{ID: "br-custom", BindName: "myapp_staging_svc-b"},
				}, nil, nil
			}
			return nil, nil, nil
		},
	}
	orig := aclClient
	aclClient = mock
	defer func() { aclClient = orig }()

	cfg := &ACLConfig{
		BindRules: []ACLBindingRuleAdapter{
			{BindName: "svc-a"},
			{BindName: "svc-b", AuthMethod: "custom-auth"},
		},
	}
	if err := deleteBindingRules(cfg, "myapp", "staging", false); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	deleted := map[string]bool{}
	for _, id := range mock.bindingRuleDeletedIDs {
		deleted[id] = true
	}
	if !deleted["br-global"] || !deleted["br-custom"] {
		t.Errorf("expected both br-global and br-custom deleted, got %v", mock.bindingRuleDeletedIDs)
	}
}

// --- mockKVClient ---

type mockKVClient struct {
	deletedKeys []string
	deleteFunc  func(key string) error
	putFunc     func(p *consulApi.KVPair) error
}

func (m *mockKVClient) Put(p *consulApi.KVPair, q *consulApi.WriteOptions) (*consulApi.WriteMeta, error) {
	if m.putFunc != nil {
		return nil, m.putFunc(p)
	}
	return nil, nil
}

func (m *mockKVClient) Delete(key string, q *consulApi.WriteOptions) (*consulApi.WriteMeta, error) {
	m.deletedKeys = append(m.deletedKeys, key)
	if m.deleteFunc != nil {
		return nil, m.deleteFunc(key)
	}
	return nil, nil
}

// 14.1: deleteKVEntries calls Delete for every entry
func TestDeleteKVEntries_CallsDeleteForEachEntry(t *testing.T) {
	mock := &mockKVClient{}
	origKV := kvClient
	kvClient = mock
	defer func() { kvClient = origKV }()

	entries := []consulacl.ConsulKVEntry{
		{Key: "config/ns/svc/"},
		{Key: "logging/ns/svc/LOG_LEVEL"},
	}
	if err := deleteKVEntries(entries); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(mock.deletedKeys) != 2 {
		t.Fatalf("expected 2 Delete calls, got %d", len(mock.deletedKeys))
	}
	if mock.deletedKeys[0] != "config/ns/svc/" {
		t.Errorf("first key: got %q, want %q", mock.deletedKeys[0], "config/ns/svc/")
	}
	if mock.deletedKeys[1] != "logging/ns/svc/LOG_LEVEL" {
		t.Errorf("second key: got %q, want %q", mock.deletedKeys[1], "logging/ns/svc/LOG_LEVEL")
	}
}

// 14.2: absent key (Delete returns nil — Consul 200 for missing key) is treated as
// success; remaining entries are still processed.
func TestDeleteKVEntries_AbsentKey_TreatedAsSuccess(t *testing.T) {
	mock := &mockKVClient{
		// first key is "absent": mock returns nil (Consul returns HTTP 200 for DELETEs of
		// missing keys, so the client returns nil error — not a 404).
		deleteFunc: func(key string) error {
			return nil
		},
	}
	origKV := kvClient
	kvClient = mock
	defer func() { kvClient = origKV }()

	entries := []consulacl.ConsulKVEntry{
		{Key: "missing/key"},
		{Key: "present/key"},
	}
	if err := deleteKVEntries(entries); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(mock.deletedKeys) != 2 {
		t.Errorf("both entries must be attempted; got %d Delete calls", len(mock.deletedKeys))
	}
}

// 14.3: network error from Delete is returned to the caller immediately.
func TestDeleteKVEntries_NetworkError_Returned(t *testing.T) {
	netErr := fmt.Errorf("dial tcp: connection refused")
	callCount := 0
	mock := &mockKVClient{
		deleteFunc: func(key string) error {
			callCount++
			if callCount == 1 {
				return netErr
			}
			return nil
		},
	}
	origKV := kvClient
	kvClient = mock
	defer func() { kvClient = origKV }()

	entries := []consulacl.ConsulKVEntry{
		{Key: "key/one"},
		{Key: "key/two"},
	}
	err := deleteKVEntries(entries)
	if err == nil {
		t.Fatal("expected error to be returned, got nil")
	}
	if err.Error() != netErr.Error() {
		t.Errorf("got error %q, want %q", err.Error(), netErr.Error())
	}
	// loop must stop after first error — second key must not be attempted
	if callCount != 1 {
		t.Errorf("expected 1 Delete call before returning error, got %d", callCount)
	}
}

// 6.3d: entities still in spec are not deleted
func TestRemoveStaleEntities_DeclaredEntitiesNotDeleted(t *testing.T) {
	mock := newStaleTestMock(
		[]*consulApi.ACLPolicyListEntry{{ID: "pol-1", Name: "myapp_staging_reader"}},
		[]*consulApi.ACLRole{{ID: "role-1", Name: "myapp_staging_writer"}},
		[]*consulApi.ACLBindingRule{{ID: "br-1", BindName: "myapp_staging_svc"}},
	)
	orig := aclClient
	aclClient = mock
	defer func() { aclClient = orig }()

	cfg := &ACLConfig{
		Policies:  []consulApi.ACLPolicy{{Name: "reader"}},
		Roles:     []ACLRoleAdapter{{Name: "writer"}},
		BindRules: []ACLBindingRuleAdapter{{BindName: "svc"}},
	}
	if err := removeStaleEntities(cfg, "myapp", "staging", false); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(mock.policyDeletedIDs) != 0 {
		t.Errorf("no policies should be deleted, got %v", mock.policyDeletedIDs)
	}
	if len(mock.roleDeletedIDs) != 0 {
		t.Errorf("no roles should be deleted, got %v", mock.roleDeletedIDs)
	}
	if len(mock.bindingRuleDeletedIDs) != 0 {
		t.Errorf("no binding rules should be deleted, got %v", mock.bindingRuleDeletedIDs)
	}
}
