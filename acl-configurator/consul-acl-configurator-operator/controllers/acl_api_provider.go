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
	consulApi "github.com/hashicorp/consul/api"
	"os"
)

const (
	tlsCaCertPath = "/consul/tls/ca/tls.crt"
)

type ACLRoleAdapter struct {
	ID          string   `json:"ID,omitempty"`
	Name        string   `json:"Name,omitempty"`
	Description string   `json:"Description,omitempty"`
	PolicyNames []string `json:"policy_names,omitempty"`
}

type ACLBindingRuleAdapter struct {
	ID                 string `json:"ID,omitempty"`
	Description        string `json:"Description,omitempty"`
	ServiceAccountName string `json:"ServiceAccountName,omitempty"`
	BindName           string `json:"BindName,omitempty"`
	AuthMethod         string `json:"AuthMethod,omitempty"`
	Selector           string `json:"Selector,omitempty"`
}

type consulACLClient interface {
	PolicyCreate(*consulApi.ACLPolicy, *consulApi.WriteOptions) (*consulApi.ACLPolicy, *consulApi.WriteMeta, error)
	PolicyUpdate(*consulApi.ACLPolicy, *consulApi.WriteOptions) (*consulApi.ACLPolicy, *consulApi.WriteMeta, error)
	PolicyReadByName(string, *consulApi.QueryOptions) (*consulApi.ACLPolicy, *consulApi.QueryMeta, error)
	PolicyDelete(string, *consulApi.WriteOptions) (*consulApi.WriteMeta, error)
	RoleCreate(*consulApi.ACLRole, *consulApi.WriteOptions) (*consulApi.ACLRole, *consulApi.WriteMeta, error)
	RoleUpdate(*consulApi.ACLRole, *consulApi.WriteOptions) (*consulApi.ACLRole, *consulApi.WriteMeta, error)
	RoleReadByName(string, *consulApi.QueryOptions) (*consulApi.ACLRole, *consulApi.QueryMeta, error)
	RoleList(*consulApi.QueryOptions) ([]*consulApi.ACLRole, *consulApi.QueryMeta, error)
	RoleDelete(string, *consulApi.WriteOptions) (*consulApi.WriteMeta, error)
	PolicyList(*consulApi.QueryOptions) ([]*consulApi.ACLPolicyListEntry, *consulApi.QueryMeta, error)
	BindingRuleCreate(*consulApi.ACLBindingRule, *consulApi.WriteOptions) (*consulApi.ACLBindingRule, *consulApi.WriteMeta, error)
	BindingRuleUpdate(*consulApi.ACLBindingRule, *consulApi.WriteOptions) (*consulApi.ACLBindingRule, *consulApi.WriteMeta, error)
	BindingRuleList(string, *consulApi.QueryOptions) ([]*consulApi.ACLBindingRule, *consulApi.QueryMeta, error)
	BindingRuleDelete(string, *consulApi.WriteOptions) (*consulApi.WriteMeta, error)
	AuthMethodCreate(*consulApi.ACLAuthMethod, *consulApi.WriteOptions) (*consulApi.ACLAuthMethod, *consulApi.WriteMeta, error)
	AuthMethodUpdate(*consulApi.ACLAuthMethod, *consulApi.WriteOptions) (*consulApi.ACLAuthMethod, *consulApi.WriteMeta, error)
	AuthMethodRead(string, *consulApi.QueryOptions) (*consulApi.ACLAuthMethod, *consulApi.QueryMeta, error)
	TokenListFiltered(consulApi.ACLTokenFilterOptions, *consulApi.QueryOptions) ([]*consulApi.ACLTokenListEntry, *consulApi.QueryMeta, error)
	TokenDelete(string, *consulApi.WriteOptions) (*consulApi.WriteMeta, error)
}

type ACLConfig struct {
	Policies  []consulApi.ACLPolicy   `json:"policies,omitempty"`
	Roles     []ACLRoleAdapter        `json:"roles,omitempty"`
	BindRules []ACLBindingRuleAdapter `json:"bind_rules,omitempty"`
}

type PoliciesStatus map[string]string

type RolesStatus map[string]string

type BindRulesStatus map[string]string

type StatusHolder map[string]string

type PolicyChangeFunction func(*consulApi.ACLPolicy, *consulApi.WriteOptions) (*consulApi.ACLPolicy, *consulApi.WriteMeta, error)

type RoleChangeFunction func(*consulApi.ACLRole, *consulApi.WriteOptions) (*consulApi.ACLRole, *consulApi.WriteMeta, error)

type BindRuleChangeFunction func(*consulApi.ACLBindingRule, *consulApi.WriteOptions) (*consulApi.ACLBindingRule, *consulApi.WriteMeta, error)

func (sh StatusHolder) GetStatus() string {
	var resString string
	if len(sh) == 0 {
		return "No action was taken"
	}
	for key, value := range sh {
		if resString != "" {
			resString += ", "
		}
		if key == "innerErrorHandlingItem" {
			resString += value
		}
		resString += fmt.Sprintf("%s: %s", key, value)
	}
	return resString
}

type consulKVClient interface {
	Get(key string, q *consulApi.QueryOptions) (*consulApi.KVPair, *consulApi.QueryMeta, error)
	Put(p *consulApi.KVPair, q *consulApi.WriteOptions) (*consulApi.WriteMeta, error)
	CAS(p *consulApi.KVPair, q *consulApi.WriteOptions) (bool, *consulApi.WriteMeta, error)
	Delete(key string, q *consulApi.WriteOptions) (*consulApi.WriteMeta, error)
	DeleteCAS(p *consulApi.KVPair, q *consulApi.WriteOptions) (bool, *consulApi.WriteMeta, error)
	DeleteTree(prefix string, q *consulApi.WriteOptions) (*consulApi.WriteMeta, error)
}

func makeConsulClient() *consulApi.Client {
	consulConfig := consulApi.DefaultConfig()
	consulConfig.Address = fmt.Sprintf("%s:%s", ConsulClientService, ConsulClientPort)
	consulConfig.Scheme = ConsulClientScheme
	if _, err := os.Stat(tlsCaCertPath); err == nil {
		consulConfig.TLSConfig.CAFile = tlsCaCertPath
	}
	consulConfig.Token = bootstrapToken
	c, err := consulApi.NewClient(consulConfig)
	if err != nil {
		log.Error(err, "Can not create a Consul client configuration")
	}
	return c
}

func makeAclClient() consulACLClient {
	return makeConsulClient().ACL()
}

func makeKVClient() consulKVClient {
	return makeConsulClient().KV()
}
