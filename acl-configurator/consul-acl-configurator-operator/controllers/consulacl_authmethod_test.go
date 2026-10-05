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
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"

	consulApi "github.com/hashicorp/consul/api"
)

// --- 20.5: selector depends on the auth method type ---

func TestConvertBindRuleAdapterToBindRule_KubernetesMethodSelector(t *testing.T) {
	adapter := ACLBindingRuleAdapter{BindName: "reader", ServiceAccountName: "my-sa", AuthMethod: "k8s"}
	rule := convertBindRuleAdapterToBindRule(adapter, "myapp", "staging", false, "kubernetes")
	want := `serviceaccount.namespace == "staging" and serviceaccount.name == "my-sa"`
	if rule.Selector != want {
		t.Errorf("selector: got %q, want %q", rule.Selector, want)
	}
}

func TestConvertBindRuleAdapterToBindRule_UnknownMethodType_JWTSelector(t *testing.T) {
	adapter := ACLBindingRuleAdapter{BindName: "reader", ServiceAccountName: "my-sa"}
	rule := convertBindRuleAdapterToBindRule(adapter, "myapp", "staging", false, "")
	want := `value.namespace == "staging" and value.serviceaccount == "my-sa"`
	if rule.Selector != want {
		t.Errorf("selector: got %q, want %q", rule.Selector, want)
	}
}

// Each auth method is read once per processBindRules call, and the selector of each rule
// follows the type of its own method.
func TestProcessBindRules_SelectorPerAuthMethodType(t *testing.T) {
	origAuthMethod := authMethod
	authMethod = "applications-k8s-m2m"
	defer func() { authMethod = origAuthMethod }()

	mock := &mockACLClient{
		authMethodReadFunc: func(name string, _ *consulApi.QueryOptions) (*consulApi.ACLAuthMethod, *consulApi.QueryMeta, error) {
			if name == "k8s" {
				return &consulApi.ACLAuthMethod{Name: name, Type: "kubernetes"}, nil, nil
			}
			return &consulApi.ACLAuthMethod{Name: name, Type: "jwt"}, nil, nil
		},
	}
	useACLClient(t, mock)

	rules := []ACLBindingRuleAdapter{
		{BindName: "a", ServiceAccountName: "sa-a"},
		{BindName: "b", ServiceAccountName: "sa-b", AuthMethod: "k8s"},
		{BindName: "c", ServiceAccountName: "sa-c", AuthMethod: "k8s"},
	}
	if _, err := processBindRules(rules, "app", "ns1", true); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	selectors := map[string]string{}
	for _, r := range mock.bindingRuleCreated {
		selectors[r.BindName] = r.Selector
	}
	if want := `value.namespace == "ns1" and value.serviceaccount == "sa-a"`; selectors["a"] != want {
		t.Errorf("rule a: got %q, want %q", selectors["a"], want)
	}
	if want := `serviceaccount.namespace == "ns1" and serviceaccount.name == "sa-b"`; selectors["b"] != want {
		t.Errorf("rule b: got %q, want %q", selectors["b"], want)
	}
	if len(mock.authMethodReadNames) != 2 {
		t.Errorf("expected each auth method to be read once, got %v", mock.authMethodReadNames)
	}
}

// --- 20.6: BoundIssuer and BoundAudiences ---

func newOpenIDServer(t *testing.T, status int, body string) (*httptest.Server, *int) {
	requests := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests++
		if r.URL.Path != openIDConfigPath {
			http.NotFound(w, r)
			return
		}
		w.WriteHeader(status)
		_, _ = w.Write([]byte(body))
	}))
	t.Cleanup(srv.Close)
	return srv, &requests
}

func TestOpenIDConfigURL_SameHostAsJWKS(t *testing.T) {
	got, err := openIDConfigURL("http://consul-acl-configurator-jwks-proxy:8080/openid/v1/jwks")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if want := "http://consul-acl-configurator-jwks-proxy:8080/.well-known/openid-configuration"; got != want {
		t.Errorf("got %q, want %q", got, want)
	}
	if _, err = openIDConfigURL("not-a-url"); err == nil {
		t.Error("expected an error for a URL without scheme and host")
	}
}

func TestBoundIssuerAndAudiences_IssuerDetected(t *testing.T) {
	srv, _ := newOpenIDServer(t, http.StatusOK, `{"issuer":"https://oidc.example.com/cluster"}`)
	t.Setenv("BOUND_ISSUER", "")
	t.Setenv("BOUND_AUDIENCES", "")

	issuer, audiences, err := boundIssuerAndAudiences(srv.URL + "/openid/v1/jwks")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if issuer != "https://oidc.example.com/cluster" {
		t.Errorf("issuer: got %q", issuer)
	}
	if !reflect.DeepEqual(audiences, []string{"https://oidc.example.com/cluster"}) {
		t.Errorf("audiences must default to the issuer, got %v", audiences)
	}
}

func TestBoundIssuerAndAudiences_OverrideUsedWithoutRequest(t *testing.T) {
	srv, requests := newOpenIDServer(t, http.StatusOK, `{"issuer":"https://detected"}`)
	t.Setenv("BOUND_ISSUER", "https://explicit")
	t.Setenv("BOUND_AUDIENCES", "aud-1, aud-2")

	issuer, audiences, err := boundIssuerAndAudiences(srv.URL + "/openid/v1/jwks")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if issuer != "https://explicit" || !reflect.DeepEqual(audiences, []string{"aud-1", "aud-2"}) {
		t.Errorf("explicit values must be used, got %q %v", issuer, audiences)
	}
	if *requests != 0 {
		t.Errorf("OpenID configuration must not be requested when the issuer is set, got %d requests", *requests)
	}
}

func TestBoundIssuerAndAudiences_OnlyAudiencesSet_IssuerDetected(t *testing.T) {
	srv, _ := newOpenIDServer(t, http.StatusOK, `{"issuer":"https://detected"}`)
	t.Setenv("BOUND_ISSUER", "")
	t.Setenv("BOUND_AUDIENCES", "custom-aud")

	issuer, audiences, err := boundIssuerAndAudiences(srv.URL + "/openid/v1/jwks")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if issuer != "https://detected" || !reflect.DeepEqual(audiences, []string{"custom-aud"}) {
		t.Errorf("got %q %v", issuer, audiences)
	}
}

func TestBoundIssuerAndAudiences_RequestFails_ErrorReturned(t *testing.T) {
	t.Setenv("BOUND_ISSUER", "")
	t.Setenv("BOUND_AUDIENCES", "")
	for name, srv := range map[string]string{
		"status":    "500",
		"no issuer": "{}",
	} {
		t.Run(name, func(t *testing.T) {
			status, body := http.StatusOK, srv
			if srv == "500" {
				status, body = http.StatusInternalServerError, ""
			}
			s, _ := newOpenIDServer(t, status, body)
			if _, _, err := boundIssuerAndAudiences(s.URL + "/openid/v1/jwks"); err == nil {
				t.Error("expected an error, a hard-coded fallback must not be used")
			}
		})
	}
}

// 24.5: a response larger than maxOpenIDConfigSize is not read into memory and is rejected.
func TestDetectIssuer_OversizedResponse_Rejected(t *testing.T) {
	body := `{"issuer":"https://issuer.example","padding":"` + strings.Repeat("x", maxOpenIDConfigSize) + `"}`
	srv, _ := newOpenIDServer(t, http.StatusOK, body)
	if _, err := detectIssuer(srv.URL + "/openid/v1/jwks"); err == nil {
		t.Fatal("expected an error for an oversized OpenID configuration")
	}
}

// --- 20.9: EnsureApplicationsAuthMethod ---

// noAuthMethod simulates Consul without the auth method.
func noAuthMethod(string, *consulApi.QueryOptions) (*consulApi.ACLAuthMethod, *consulApi.QueryMeta, error) {
	return nil, nil, nil
}

func setupEnsureTest(t *testing.T) string {
	srv, _ := newOpenIDServer(t, http.StatusOK, `{"issuer":"https://issuer.example"}`)
	jwksURL := srv.URL + "/openid/v1/jwks"
	t.Setenv("JWKS_URL", jwksURL)
	t.Setenv("BOUND_ISSUER", "")
	t.Setenv("BOUND_AUDIENCES", "")
	return jwksURL
}

// consulStoredMethod simulates the method returned by Consul: the config goes through JSON,
// so lists and maps come back as []interface{} and map[string]interface{}.
func consulStoredMethod(t *testing.T, am *consulApi.ACLAuthMethod) *consulApi.ACLAuthMethod {
	data, err := json.Marshal(am)
	if err != nil {
		t.Fatal(err)
	}
	stored := &consulApi.ACLAuthMethod{}
	if err = json.Unmarshal(data, stored); err != nil {
		t.Fatal(err)
	}
	return stored
}

func TestEnsureApplicationsAuthMethod_Create(t *testing.T) {
	jwksURL := setupEnsureTest(t)
	mock := &mockACLClient{authMethodReadFunc: noAuthMethod}
	useACLClient(t, mock)

	if err := EnsureApplicationsAuthMethod(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(mock.authMethodCreated) != 1 || len(mock.authMethodUpdated) != 0 {
		t.Fatalf("expected one create and no update, got %d/%d", len(mock.authMethodCreated), len(mock.authMethodUpdated))
	}
	cfg := mock.authMethodCreated[0].Config
	if cfg["JWKSURL"] != jwksURL || cfg["BoundIssuer"] != "https://issuer.example" {
		t.Errorf("unexpected config %v", cfg)
	}
	if !reflect.DeepEqual(cfg["BoundAudiences"], []string{"https://issuer.example"}) {
		t.Errorf("unexpected audiences %v", cfg["BoundAudiences"])
	}
}

func TestEnsureApplicationsAuthMethod_Unchanged_NoUpdate(t *testing.T) {
	setupEnsureTest(t)
	first := &mockACLClient{authMethodReadFunc: noAuthMethod}
	useACLClient(t, first)
	if err := EnsureApplicationsAuthMethod(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	stored := consulStoredMethod(t, first.authMethodCreated[0])

	mock := &mockACLClient{
		authMethodReadFunc: func(string, *consulApi.QueryOptions) (*consulApi.ACLAuthMethod, *consulApi.QueryMeta, error) {
			return stored, nil, nil
		},
	}
	useACLClient(t, mock)
	if err := EnsureApplicationsAuthMethod(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(mock.authMethodUpdated) != 0 || len(mock.authMethodCreated) != 0 {
		t.Errorf("unchanged method must not be written, got %d updates, %d creates", len(mock.authMethodUpdated), len(mock.authMethodCreated))
	}
}

func TestEnsureApplicationsAuthMethod_Changed_Updated(t *testing.T) {
	setupEnsureTest(t)
	stored := &consulApi.ACLAuthMethod{
		Name:        "applications-k8s-m2m",
		Type:        "jwt",
		Description: "Auth method for application M2M authentication",
		Config: map[string]interface{}{
			"BoundIssuer":    "https://kubernetes.default.svc.cluster.local",
			"BoundAudiences": []interface{}{"https://kubernetes.default.svc.cluster.local"},
		},
	}
	mock := &mockACLClient{
		authMethodReadFunc: func(string, *consulApi.QueryOptions) (*consulApi.ACLAuthMethod, *consulApi.QueryMeta, error) {
			return stored, nil, nil
		},
	}
	useACLClient(t, mock)

	if err := EnsureApplicationsAuthMethod(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(mock.authMethodUpdated) != 1 {
		t.Fatalf("expected one update, got %d", len(mock.authMethodUpdated))
	}
	if got := mock.authMethodUpdated[0].Config["BoundIssuer"]; got != "https://issuer.example" {
		t.Errorf("BoundIssuer: got %v", got)
	}
}

func TestEnsureApplicationsAuthMethod_IssuerNotAvailable_ErrorAndNoWrite(t *testing.T) {
	srv, _ := newOpenIDServer(t, http.StatusServiceUnavailable, "")
	t.Setenv("JWKS_URL", srv.URL+"/openid/v1/jwks")
	t.Setenv("BOUND_ISSUER", "")
	t.Setenv("BOUND_AUDIENCES", "")
	mock := &mockACLClient{authMethodReadFunc: noAuthMethod}
	useACLClient(t, mock)

	if err := EnsureApplicationsAuthMethod(); err == nil {
		t.Fatal("expected an error so that the caller retries")
	}
	if len(mock.authMethodCreated) != 0 || len(mock.authMethodUpdated) != 0 {
		t.Error("the method must not be written without a resolved issuer")
	}
}
