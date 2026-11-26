package controllers

import (
	"reflect"
	"testing"
	"time"

	consulApi "github.com/hashicorp/consul/api"
)

func setupIdpEnv(t *testing.T) {
	t.Setenv("IDP_AUTH_METHOD_NAME", "platform-idp")
	t.Setenv("IDP_JWKS_URL", "https://idp.example/realms/master/protocol/openid-connect/certs")
	t.Setenv("IDP_BOUND_ISSUER", "https://idp.example/realms/master")
	t.Setenv("IDP_BOUND_AUDIENCES", "consul-ui")
	t.Setenv("IDP_USERNAME_CLAIM", "")
	t.Setenv("IDP_JWKS_CA_CERT", "")
}

func TestEnsureIdpAuthMethod_NotConfigured_DoesNothing(t *testing.T) {
	t.Setenv("IDP_AUTH_METHOD_NAME", "")
	mock := &mockACLClient{authMethodReadFunc: noAuthMethod}
	useACLClient(t, mock)

	if err := EnsureIdpAuthMethod(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(mock.authMethodCreated) != 0 || len(mock.authMethodUpdated) != 0 {
		t.Fatal("the method must not be written when it is not configured")
	}
}

func TestEnsureIdpAuthMethod_Create(t *testing.T) {
	setupIdpEnv(t)
	t.Setenv("IDP_JWKS_CA_CERT", "-----BEGIN CERTIFICATE-----\nabc\n-----END CERTIFICATE-----\n")
	mock := &mockACLClient{authMethodReadFunc: noAuthMethod}
	useACLClient(t, mock)

	if err := EnsureIdpAuthMethod(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(mock.authMethodCreated) != 1 || len(mock.authMethodUpdated) != 0 {
		t.Fatalf("expected one create and no update, got %d/%d", len(mock.authMethodCreated), len(mock.authMethodUpdated))
	}
	am := mock.authMethodCreated[0]
	if am.Name != "platform-idp" || am.Type != "jwt" || am.MaxTokenTTL != 15*time.Minute {
		t.Errorf("unexpected method %+v", am)
	}
	want := map[string]interface{}{
		"JWKSURL":           "https://idp.example/realms/master/protocol/openid-connect/certs",
		"BoundIssuer":       "https://idp.example/realms/master",
		"BoundAudiences":    []string{"consul-ui"},
		"JWTSupportedAlgs":  []string{"RS256"},
		"ClaimMappings":     map[string]string{"preferred_username": "username"},
		"ListClaimMappings": map[string]string{"/realm_access/roles": "roles"},
		"JWKSCACert":        "-----BEGIN CERTIFICATE-----\nabc\n-----END CERTIFICATE-----",
	}
	if !reflect.DeepEqual(am.Config, want) {
		t.Errorf("unexpected config\n got: %v\nwant: %v", am.Config, want)
	}
}

func TestEnsureIdpAuthMethod_CustomUsernameClaim_NoCA_NoAudiences(t *testing.T) {
	setupIdpEnv(t)
	t.Setenv("IDP_USERNAME_CLAIM", "email")
	t.Setenv("IDP_BOUND_AUDIENCES", "")
	mock := &mockACLClient{authMethodReadFunc: noAuthMethod}
	useACLClient(t, mock)

	if err := EnsureIdpAuthMethod(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	cfg := mock.authMethodCreated[0].Config
	if !reflect.DeepEqual(cfg["ClaimMappings"], map[string]string{"email": "username"}) {
		t.Errorf("unexpected claim mappings %v", cfg["ClaimMappings"])
	}
	if _, ok := cfg["JWKSCACert"]; ok {
		t.Error("JWKSCACert must be absent when there is no CA")
	}
	if _, ok := cfg["BoundAudiences"]; ok {
		t.Error("BoundAudiences must be absent when no audience is configured")
	}
}

func TestEnsureIdpAuthMethod_Unchanged_NoUpdate(t *testing.T) {
	setupIdpEnv(t)
	first := &mockACLClient{authMethodReadFunc: noAuthMethod}
	useACLClient(t, first)
	if err := EnsureIdpAuthMethod(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	stored := consulStoredMethod(t, first.authMethodCreated[0])

	mock := &mockACLClient{
		authMethodReadFunc: func(string, *consulApi.QueryOptions) (*consulApi.ACLAuthMethod, *consulApi.QueryMeta, error) {
			return stored, nil, nil
		},
	}
	useACLClient(t, mock)
	if err := EnsureIdpAuthMethod(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(mock.authMethodUpdated) != 0 || len(mock.authMethodCreated) != 0 {
		t.Errorf("unchanged method must not be written, got %d updates, %d creates", len(mock.authMethodUpdated), len(mock.authMethodCreated))
	}
}

func TestEnsureIdpAuthMethod_Changed_Updated(t *testing.T) {
	setupIdpEnv(t)
	stored := &consulApi.ACLAuthMethod{
		Name:        "platform-idp",
		Type:        "jwt",
		Description: "Auth method of the platform IdP for the Consul UI users",
		MaxTokenTTL: 15 * time.Minute,
		Config: map[string]interface{}{
			"JWKSURL":     "https://old.example/certs",
			"BoundIssuer": "https://old.example",
		},
	}
	mock := &mockACLClient{
		authMethodReadFunc: func(string, *consulApi.QueryOptions) (*consulApi.ACLAuthMethod, *consulApi.QueryMeta, error) {
			return stored, nil, nil
		},
	}
	useACLClient(t, mock)

	if err := EnsureIdpAuthMethod(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(mock.authMethodUpdated) != 1 {
		t.Fatalf("expected one update, got %d", len(mock.authMethodUpdated))
	}
	if got := mock.authMethodUpdated[0].Config["BoundIssuer"]; got != "https://idp.example/realms/master" {
		t.Errorf("BoundIssuer: got %v", got)
	}
}

func TestEnsureIdpAuthMethod_MissingSettings_Error(t *testing.T) {
	setupIdpEnv(t)
	t.Setenv("IDP_JWKS_URL", "")
	mock := &mockACLClient{authMethodReadFunc: noAuthMethod}
	useACLClient(t, mock)

	if err := EnsureIdpAuthMethod(); err == nil {
		t.Fatal("expected an error for a missing JWKS URL")
	}
	if len(mock.authMethodCreated) != 0 {
		t.Error("the method must not be written with incomplete settings")
	}
}
