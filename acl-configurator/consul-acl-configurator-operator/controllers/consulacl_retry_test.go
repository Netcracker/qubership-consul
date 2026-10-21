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

	consulApi "github.com/hashicorp/consul/api"
)

var noLeader = consulApi.StatusError{Code: 500, Body: "No cluster leader"}

// --- Consul 5xx responses are transient ---

func TestProcessPolicies_NoClusterLeaderOnCreate_ErrorReturned(t *testing.T) {
	failing := &failingPolicyClient{mockACLClient: &mockACLClient{}, createErr: noLeader}
	useACLClientIface(t, failing)

	status, _, err := processPolicies([]consulApi.ACLPolicy{{Name: "p", Rules: `key "a" { policy = "read" }`}}, "app", "ns", false)
	if err == nil {
		t.Fatal("a 500 response must be returned so that the request is requeued")
	}
	if !strings.HasPrefix((*status)["app_ns_p"], "error:") {
		t.Errorf("expected error status, got %v", *status)
	}
}

func TestProcessPolicies_BadRequest_NotReturned(t *testing.T) {
	failing := &failingPolicyClient{mockACLClient: &mockACLClient{}, createErr: consulApi.StatusError{Code: 400, Body: "Bad request: invalid rules"}}
	useACLClientIface(t, failing)

	status, _, err := processPolicies([]consulApi.ACLPolicy{{Name: "p"}}, "app", "ns", false)
	if err != nil {
		t.Fatalf("a configuration error must not be retried: %v", err)
	}
	if !strings.HasPrefix((*status)["app_ns_p"], "error:") {
		t.Errorf("expected error status, got %v", *status)
	}
}

// A failed read must not be taken for an absent policy: nothing is created and the request is requeued.
func TestProcessPolicies_ReadFails_NotCreatedAndErrorReturned(t *testing.T) {
	mock := &mockACLClient{
		policyReadByNameFunc: func(string, *consulApi.QueryOptions) (*consulApi.ACLPolicy, *consulApi.QueryMeta, error) {
			return nil, nil, noLeader
		},
	}
	counting := &failingPolicyClient{mockACLClient: mock}
	useACLClientIface(t, counting)

	_, _, err := processPolicies([]consulApi.ACLPolicy{{Name: "p"}}, "app", "ns", false)
	if err == nil {
		t.Fatal("expected the read error to be returned")
	}
	if counting.creates != 0 {
		t.Errorf("the policy must not be created after a failed read, got %d creates", counting.creates)
	}
}

func TestProcessRoles_ReadFails_NotCreatedAndErrorReturned(t *testing.T) {
	mock := &mockACLClient{
		roleReadByNameFunc: func(string, *consulApi.QueryOptions) (*consulApi.ACLRole, *consulApi.QueryMeta, error) {
			return nil, nil, noLeader
		},
	}
	useACLClient(t, mock)

	if _, err := processRoles([]ACLRoleAdapter{{Name: "r"}}, nil, "app", "ns", true); err == nil {
		t.Fatal("expected the read error to be returned")
	}
	if mock.roleCreateCalled || mock.roleUpdateCalled {
		t.Error("the role must not be written after a failed read")
	}
}

func TestReadPolicyAndRole_NotFoundVersusFailure(t *testing.T) {
	mock := &mockACLClient{
		policyReadByNameFunc: func(name string, _ *consulApi.QueryOptions) (*consulApi.ACLPolicy, *consulApi.QueryMeta, error) {
			if name == "missing" {
				return nil, nil, fmt.Errorf("Unexpected response code: 403 (ACL not found)")
			}
			return nil, nil, noLeader
		},
		roleReadByNameFunc: func(string, *consulApi.QueryOptions) (*consulApi.ACLRole, *consulApi.QueryMeta, error) {
			return nil, nil, noLeader
		},
	}
	useACLClient(t, mock)

	if p, err := readPolicy("missing"); p != nil || err != nil {
		t.Errorf("not found: got %v, %v", p, err)
	}
	if _, err := readPolicy("other"); err == nil {
		t.Error("a 500 response must be returned by readPolicy")
	}
	if _, err := readRole("r"); err == nil {
		t.Error("a 500 response must be returned by readRole")
	}
}

// During an outage of Consul the deletion of a CR must fail and be retried, not skip the role.
func TestDeleteRoles_ReadFails_ErrorReturned(t *testing.T) {
	mock := &mockACLClient{
		roleReadByNameFunc: func(string, *consulApi.QueryOptions) (*consulApi.ACLRole, *consulApi.QueryMeta, error) {
			return nil, nil, noLeader
		},
	}
	useACLClient(t, mock)

	if err := deleteRoles(&ACLConfig{Roles: []ACLRoleAdapter{{Name: "r"}}}, "app", "ns", true, nil); err == nil {
		t.Fatal("expected the read error to be returned")
	}
}

// --- missing auth method ---

func TestProcessBindRules_GlobalMethodMissing_NotCreatedAndRetried(t *testing.T) {
	origAuthMethod := authMethod
	authMethod = "applications-k8s-m2m"
	defer func() { authMethod = origAuthMethod }()
	mock := &mockACLClient{authMethodReadFunc: noAuthMethod}
	useACLClient(t, mock)

	status, err := processBindRules([]ACLBindingRuleAdapter{{BindName: "reader", ServiceAccountName: "sa"}}, "app", "ns", false)
	if err == nil || !isRetryable(err) {
		t.Fatalf("a missing auth method must give a retryable error, got %v", err)
	}
	if mock.bindingRuleCreateCalled {
		t.Error("the rule must not be created under a missing method")
	}
	if !strings.Contains((*status)["app_ns_reader"], "does not exist") {
		t.Errorf("expected the error in status, got %v", *status)
	}
}

// --- legacy cleanup keeps the old rule when the new one failed ---

func TestRemoveLegacyBindingRules_FailedReplacementKept(t *testing.T) {
	useLegacyAuthMethods(t, "applications-k8s-m2m", legacyMethod)
	mock := newLegacyMock([]*consulApi.ACLBindingRule{
		{ID: "failed", BindName: "my-service_ns_reader", BindType: consulApi.BindingRuleBindTypeRole},
		{ID: "ok", BindName: "my-service_ns_writer", BindType: consulApi.BindingRuleBindTypeRole},
	})
	useACLClient(t, mock)

	status := StatusHolder{"my-service_ns_reader": "error: Unexpected response code: 400 (invalid selector)"}
	if err := removeLegacyBindingRules(&ACLConfig{}, "my-service", "ns", true, failedEntities(&status)); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if fmt.Sprint(mock.bindingRuleDeletedIDs) != "[ok]" {
		t.Errorf("only the rule with a written replacement may be deleted, got %v", mock.bindingRuleDeletedIDs)
	}
}

// failingPolicyClient wraps the mock and fails policy writes with createErr.
type failingPolicyClient struct {
	*mockACLClient
	createErr error
	creates   int
}

func (c *failingPolicyClient) PolicyCreate(p *consulApi.ACLPolicy, q *consulApi.WriteOptions) (*consulApi.ACLPolicy, *consulApi.WriteMeta, error) {
	c.creates++
	if c.createErr != nil {
		return nil, nil, c.createErr
	}
	return c.mockACLClient.PolicyCreate(p, q)
}

func useACLClientIface(t *testing.T, client consulACLClient) {
	orig := aclClient
	aclClient = client
	t.Cleanup(func() { aclClient = orig })
}
