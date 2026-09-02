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
	"strings"
	"testing"

	consulacl "github.com/Netcracker/consul-acl-configurator/consul-acl-configurator-operator/api/v1alpha1"
	consulApi "github.com/hashicorp/consul/api"
)

// mockACLClient is a test double for consulACLClient.
type mockACLClient struct {
	// Role
	roleReadByNameFunc func(string, *consulApi.QueryOptions) (*consulApi.ACLRole, *consulApi.QueryMeta, error)
	roleListFunc       func(*consulApi.QueryOptions) ([]*consulApi.ACLRole, *consulApi.QueryMeta, error)
	roleCreateCalled   bool
	roleUpdateCalled   bool
	roleDeletedIDs     []string
	roleCreateFunc     func(*consulApi.ACLRole, *consulApi.WriteOptions) (*consulApi.ACLRole, *consulApi.WriteMeta, error)
	roleUpdateFunc     func(*consulApi.ACLRole, *consulApi.WriteOptions) (*consulApi.ACLRole, *consulApi.WriteMeta, error)
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
func (m *mockACLClient) AuthMethodCreate(am *consulApi.ACLAuthMethod, q *consulApi.WriteOptions) (*consulApi.ACLAuthMethod, *consulApi.WriteMeta, error) {
	return am, nil, nil
}
func (m *mockACLClient) AuthMethodRead(name string, q *consulApi.QueryOptions) (*consulApi.ACLAuthMethod, *consulApi.QueryMeta, error) {
	return nil, nil, nil
}
func (m *mockACLClient) AuthMethodUpdate(am *consulApi.ACLAuthMethod, q *consulApi.WriteOptions) (*consulApi.ACLAuthMethod, *consulApi.WriteMeta, error) {
	return am, nil, nil
}
func (m *mockACLClient) TokenListFiltered(f consulApi.ACLTokenFilterOptions, q *consulApi.QueryOptions) ([]*consulApi.ACLTokenListEntry, *consulApi.QueryMeta, error) {
	return nil, nil, nil
}
func (m *mockACLClient) TokenDelete(accessorID string, q *consulApi.WriteOptions) (*consulApi.WriteMeta, error) {
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
	store          map[string]*consulApi.KVPair
	deletedKeys    []string
	deletedTrees   []string
	casPairs       []consulApi.KVPair
	getFunc        func(key string) (*consulApi.KVPair, error)
	casFunc        func(p *consulApi.KVPair) (bool, error)
	putFunc        func(p *consulApi.KVPair) error
	deleteFunc     func(key string) error
	deleteCASFunc  func(p *consulApi.KVPair) (bool, error)
	deleteTreeFunc func(prefix string) error
}

func (m *mockKVClient) initStore() {
	if m.store == nil {
		m.store = make(map[string]*consulApi.KVPair)
	}
}

func (m *mockKVClient) Get(key string, _ *consulApi.QueryOptions) (*consulApi.KVPair, *consulApi.QueryMeta, error) {
	m.initStore()
	if m.getFunc != nil {
		p, err := m.getFunc(key)
		return p, nil, err
	}
	return m.store[key], nil, nil
}

func (m *mockKVClient) CAS(p *consulApi.KVPair, _ *consulApi.WriteOptions) (bool, *consulApi.WriteMeta, error) {
	m.initStore()
	if m.casFunc != nil {
		ok, err := m.casFunc(p)
		return ok, nil, err
	}
	m.casPairs = append(m.casPairs, *p)
	current := m.store[p.Key]
	var currentIdx uint64
	if current != nil {
		currentIdx = current.ModifyIndex
	}
	if p.ModifyIndex != currentIdx {
		return false, nil, nil
	}
	cp := *p
	cp.ModifyIndex = currentIdx + 1
	m.store[p.Key] = &cp
	return true, nil, nil
}

func (m *mockKVClient) Put(p *consulApi.KVPair, _ *consulApi.WriteOptions) (*consulApi.WriteMeta, error) {
	m.initStore()
	if m.putFunc != nil {
		return nil, m.putFunc(p)
	}
	cp := *p
	m.store[p.Key] = &cp
	return nil, nil
}

func (m *mockKVClient) Delete(key string, _ *consulApi.WriteOptions) (*consulApi.WriteMeta, error) {
	m.initStore()
	m.deletedKeys = append(m.deletedKeys, key)
	if m.deleteFunc != nil {
		return nil, m.deleteFunc(key)
	}
	delete(m.store, key)
	return nil, nil
}

func (m *mockKVClient) DeleteCAS(p *consulApi.KVPair, _ *consulApi.WriteOptions) (bool, *consulApi.WriteMeta, error) {
	m.initStore()
	if m.deleteCASFunc != nil {
		ok, err := m.deleteCASFunc(p)
		return ok, nil, err
	}
	current := m.store[p.Key]
	if current == nil {
		return true, nil, nil
	}
	if p.ModifyIndex != current.ModifyIndex {
		return false, nil, nil
	}
	m.deletedKeys = append(m.deletedKeys, p.Key)
	delete(m.store, p.Key)
	return true, nil, nil
}

func (m *mockKVClient) DeleteTree(prefix string, _ *consulApi.WriteOptions) (*consulApi.WriteMeta, error) {
	m.initStore()
	if m.deleteTreeFunc != nil {
		return nil, m.deleteTreeFunc(prefix)
	}
	m.deletedTrees = append(m.deletedTrees, prefix)
	for key := range m.store {
		if key == prefix || strings.HasPrefix(key, prefix+"/") {
			delete(m.store, key)
		}
	}
	return nil, nil
}

// 14.1: deleteKVEntries deletes only owned entries via decrementOrDelete
func TestDeleteKVEntries_CallsDeleteForEachOwnedEntry(t *testing.T) {
	mock := &mockKVClient{}
	origKV := kvClient
	kvClient = mock
	defer func() { kvClient = origKV }()

	mock.initStore()
	mock.store["config/ns/svc/"] = &consulApi.KVPair{Key: "config/ns/svc/", Flags: 1, ModifyIndex: 1}
	mock.store["logging/ns/svc/LOG_LEVEL"] = &consulApi.KVPair{Key: "logging/ns/svc/LOG_LEVEL", Flags: 1, ModifyIndex: 1}

	statuses := []consulacl.ConsulKVEntryStatus{
		{Key: "config/ns/svc/", Status: "synced", Owned: true},
		{Key: "logging/ns/svc/LOG_LEVEL", Status: "synced", Owned: true},
	}
	if err := deleteKVEntries(statuses); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(mock.deletedKeys) != 2 {
		t.Fatalf("expected 2 deletions, got %d: %v", len(mock.deletedKeys), mock.deletedKeys)
	}
}

// 14.2: not-owned entries are skipped by deleteKVEntries
func TestDeleteKVEntries_NotOwnedEntries_Skipped(t *testing.T) {
	mock := &mockKVClient{}
	origKV := kvClient
	kvClient = mock
	defer func() { kvClient = origKV }()

	statuses := []consulacl.ConsulKVEntryStatus{
		{Key: "config/ns/svc/", Status: "synced", Owned: false},
	}
	if err := deleteKVEntries(statuses); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(mock.deletedKeys) != 0 {
		t.Errorf("no deletions expected for not-owned entries, got %v", mock.deletedKeys)
	}
}

// 14.3: network error from Get is returned; remaining entries are still processed
func TestDeleteKVEntries_NetworkError_Returned(t *testing.T) {
	netErr := fmt.Errorf("dial tcp: connection refused")
	callCount := 0
	mock := &mockKVClient{
		getFunc: func(key string) (*consulApi.KVPair, error) {
			callCount++
			if callCount == 1 {
				return nil, netErr
			}
			return &consulApi.KVPair{Key: key, Flags: 1, ModifyIndex: 1}, nil
		},
	}
	origKV := kvClient
	kvClient = mock
	defer func() { kvClient = origKV }()

	statuses := []consulacl.ConsulKVEntryStatus{
		{Key: "key/one", Status: "synced", Owned: true},
		{Key: "key/two", Status: "synced", Owned: true},
	}
	err := deleteKVEntries(statuses)
	if err == nil {
		t.Fatal("expected error to be returned, got nil")
	}
	// all entries are attempted even on partial error
	if callCount != 2 {
		t.Errorf("expected Get called for both entries, got %d calls", callCount)
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
