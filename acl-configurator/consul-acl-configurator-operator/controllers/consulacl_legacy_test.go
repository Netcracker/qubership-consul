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
	"sort"
	"testing"

	consulApi "github.com/hashicorp/consul/api"
)

const legacyMethod = "consul-k8s-auth-method"

func useLegacyAuthMethods(t *testing.T, global string, legacy ...string) {
	origGlobal, origLegacy := authMethod, legacyAuthMethods
	authMethod, legacyAuthMethods = global, legacy
	t.Cleanup(func() { authMethod, legacyAuthMethods = origGlobal, origLegacy })
}

// newLegacyMock returns a client where the legacy method exists and has the given rules.
func newLegacyMock(rules []*consulApi.ACLBindingRule) *mockACLClient {
	return &mockACLClient{
		authMethodReadFunc: func(name string, _ *consulApi.QueryOptions) (*consulApi.ACLAuthMethod, *consulApi.QueryMeta, error) {
			if name == legacyMethod {
				return &consulApi.ACLAuthMethod{Name: name, Type: "kubernetes"}, nil, nil
			}
			return nil, nil, nil
		},
		bindingRuleListFunc: func(am string, _ *consulApi.QueryOptions) ([]*consulApi.ACLBindingRule, *consulApi.QueryMeta, error) {
			if am == legacyMethod {
				return rules, nil, nil
			}
			return nil, nil, nil
		},
	}
}

func sortedIDs(ids []string) []string {
	out := append([]string(nil), ids...)
	sort.Strings(out)
	return out
}

func TestRemoveLegacyBindingRules_OnlyRoleRulesOfTheCRDeleted(t *testing.T) {
	useLegacyAuthMethods(t, "applications-k8s-m2m", legacyMethod)
	mock := newLegacyMock([]*consulApi.ACLBindingRule{
		{ID: "own-1", BindName: "my-service_ns_reader", BindType: consulApi.BindingRuleBindTypeRole},
		{ID: "own-2", BindName: "my-service_ns_writer", BindType: consulApi.BindingRuleBindTypeRole},
		{ID: "other-cr", BindName: "other_ns_reader", BindType: consulApi.BindingRuleBindTypeRole},
		{ID: "mesh", BindName: "${serviceaccount.name}", BindType: consulApi.BindingRuleBindTypeService},
		{ID: "prefix-like-service", BindName: "my-service_ns_x", BindType: consulApi.BindingRuleBindTypeService},
	})
	useACLClient(t, mock)

	cfg := &ACLConfig{BindRules: []ACLBindingRuleAdapter{{BindName: "reader"}}}
	if err := removeLegacyBindingRules(cfg, "my-service", "ns", true, nil); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got := sortedIDs(mock.bindingRuleDeletedIDs); fmt.Sprint(got) != "[own-1 own-2]" {
		t.Errorf("deleted: got %v, want [own-1 own-2]", got)
	}
}

// A rule that the spec explicitly keeps under the legacy method (per-rule AuthMethod) stays on
// apply and is removed together with the CR.
func TestRemoveLegacyBindingRules_PerRuleLegacyMethodKeptOnApply(t *testing.T) {
	useLegacyAuthMethods(t, "applications-k8s-m2m", legacyMethod)
	rules := []*consulApi.ACLBindingRule{
		{ID: "kept", BindName: "my-service_ns_reader", BindType: consulApi.BindingRuleBindTypeRole},
		{ID: "stale", BindName: "my-service_ns_writer", BindType: consulApi.BindingRuleBindTypeRole},
	}
	cfg := &ACLConfig{BindRules: []ACLBindingRuleAdapter{{BindName: "reader", AuthMethod: legacyMethod}}}

	mock := newLegacyMock(rules)
	useACLClient(t, mock)
	if err := removeLegacyBindingRules(cfg, "my-service", "ns", true, nil); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if fmt.Sprint(mock.bindingRuleDeletedIDs) != "[stale]" {
		t.Errorf("apply: got %v, want [stale]", mock.bindingRuleDeletedIDs)
	}

	mock = newLegacyMock(rules)
	useACLClient(t, mock)
	if err := removeLegacyBindingRules(cfg, "my-service", "ns", false, nil); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got := sortedIDs(mock.bindingRuleDeletedIDs); fmt.Sprint(got) != "[kept stale]" {
		t.Errorf("delete: got %v, want both rules", got)
	}
}

func TestRemoveLegacyBindingRules_CurrentGlobalMethodNotTreatedAsLegacy(t *testing.T) {
	useLegacyAuthMethods(t, legacyMethod, legacyMethod)
	mock := newLegacyMock([]*consulApi.ACLBindingRule{
		{ID: "own", BindName: "my-service_ns_reader", BindType: consulApi.BindingRuleBindTypeRole},
	})
	useACLClient(t, mock)

	if err := removeLegacyBindingRules(&ACLConfig{}, "my-service", "ns", false, nil); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(mock.bindingRuleDeletedIDs) != 0 || len(mock.authMethodReadNames) != 0 {
		t.Errorf("the current global method must be skipped, deleted %v", mock.bindingRuleDeletedIDs)
	}
}

func TestRemoveLegacyBindingRules_LegacyMethodMissing_Skipped(t *testing.T) {
	useLegacyAuthMethods(t, "applications-k8s-m2m", legacyMethod)
	listed := false
	mock := &mockACLClient{
		authMethodReadFunc: noAuthMethod,
		bindingRuleListFunc: func(string, *consulApi.QueryOptions) ([]*consulApi.ACLBindingRule, *consulApi.QueryMeta, error) {
			listed = true
			return nil, nil, fmt.Errorf("auth method not found")
		},
	}
	useACLClient(t, mock)

	if err := removeLegacyBindingRules(&ACLConfig{}, "my-service", "ns", true, nil); err != nil {
		t.Fatalf("a missing legacy method must not fail the reconcile: %v", err)
	}
	if listed {
		t.Error("binding rules must not be listed for a missing method")
	}
}

func TestRemoveLegacyBindingRules_NetworkError_Returned(t *testing.T) {
	useLegacyAuthMethods(t, "applications-k8s-m2m", legacyMethod)
	netErr := &net.OpError{Op: "dial", Net: "tcp", Err: fmt.Errorf("connection refused")}
	mock := &mockACLClient{
		authMethodReadFunc: func(string, *consulApi.QueryOptions) (*consulApi.ACLAuthMethod, *consulApi.QueryMeta, error) {
			return nil, nil, netErr
		},
	}
	useACLClient(t, mock)

	if err := removeLegacyBindingRules(&ACLConfig{}, "my-service", "ns", true, nil); err == nil {
		t.Fatal("expected the network error to be returned")
	}
}

func TestSplitList(t *testing.T) {
	if got := fmt.Sprint(splitList(" a, ,b ,")); got != "[a b]" {
		t.Errorf("got %v", got)
	}
	if splitList("") != nil {
		t.Error("empty value must give no items")
	}
}
