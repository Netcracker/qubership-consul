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

	consulApi "github.com/hashicorp/consul/api"
)

// mockACLClient is a test double for consulACLClient.
// Each call-tracking field records whether the method was invoked and with what args.
type mockACLClient struct {
	// RoleReadByName
	roleReadByNameFunc func(string, *consulApi.QueryOptions) (*consulApi.ACLRole, *consulApi.QueryMeta, error)
	// RoleCreate / RoleUpdate
	roleCreateCalled bool
	roleUpdateCalled bool
	roleCreateFunc   func(*consulApi.ACLRole, *consulApi.WriteOptions) (*consulApi.ACLRole, *consulApi.WriteMeta, error)
	roleUpdateFunc   func(*consulApi.ACLRole, *consulApi.WriteOptions) (*consulApi.ACLRole, *consulApi.WriteMeta, error)
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
	return nil, nil
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
func (m *mockACLClient) RoleDelete(id string, q *consulApi.WriteOptions) (*consulApi.WriteMeta, error) {
	return nil, nil
}
func (m *mockACLClient) BindingRuleCreate(br *consulApi.ACLBindingRule, q *consulApi.WriteOptions) (*consulApi.ACLBindingRule, *consulApi.WriteMeta, error) {
	return br, nil, nil
}
func (m *mockACLClient) BindingRuleUpdate(br *consulApi.ACLBindingRule, q *consulApi.WriteOptions) (*consulApi.ACLBindingRule, *consulApi.WriteMeta, error) {
	return br, nil, nil
}
func (m *mockACLClient) BindingRuleList(authMethod string, q *consulApi.QueryOptions) ([]*consulApi.ACLBindingRule, *consulApi.QueryMeta, error) {
	return nil, nil, nil
}
func (m *mockACLClient) BindingRuleDelete(id string, q *consulApi.WriteOptions) (*consulApi.WriteMeta, error) {
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
