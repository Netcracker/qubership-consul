// Package consul calls the Consul ACL login and logout endpoints.
package consul

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// Token is a Consul ACL token returned by /v1/acl/login.
type Token struct {
	AccessorID     string     `json:"AccessorID"`
	SecretID       string     `json:"SecretID"`
	ExpirationTime *time.Time `json:"ExpirationTime"`
}

var (
	// ErrLoginDenied: Consul rejected the JWT or no binding rule matched the identity.
	ErrLoginDenied = errors.New("consul login denied")
	// ErrUnavailable: Consul could not be reached.
	ErrUnavailable = errors.New("consul unavailable")
	// ErrUpstream: Consul answered with an unexpected status.
	ErrUpstream = errors.New("consul upstream error")
)

// Client is a minimal Consul ACL client.
type Client struct {
	base       *url.URL
	authMethod string
	http       *http.Client
}

// New returns a client for the Consul address and JWT auth method.
func New(base *url.URL, authMethod string, hc *http.Client) *Client {
	return &Client{base: base, authMethod: authMethod, http: hc}
}

// Login exchanges an IdP JWT for a Consul token through the JWT auth method.
func (c *Client) Login(ctx context.Context, jwt string, meta map[string]string) (*Token, error) {
	body, _ := json.Marshal(map[string]any{"AuthMethod": c.authMethod, "BearerToken": jwt, "Meta": meta})
	resp, err := c.post(ctx, "/v1/acl/login", "", body)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	msg, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	switch resp.StatusCode {
	case http.StatusOK:
		var t Token
		if err := json.Unmarshal(msg, &t); err != nil || t.SecretID == "" {
			return nil, fmt.Errorf("%w: cannot decode login response", ErrUpstream)
		}
		return &t, nil
	case http.StatusUnauthorized, http.StatusForbidden:
		return nil, fmt.Errorf("%w: %d %s", ErrLoginDenied, resp.StatusCode, strings.TrimSpace(string(msg)))
	default:
		return nil, fmt.Errorf("%w: login returned %d %s", ErrUpstream, resp.StatusCode, strings.TrimSpace(string(msg)))
	}
}

// Logout destroys a token created by Login.
func (c *Client) Logout(ctx context.Context, secretID string) error {
	resp, err := c.post(ctx, "/v1/acl/logout", secretID, nil)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 1<<16))
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("%w: logout returned %d", ErrUpstream, resp.StatusCode)
	}
	return nil
}

func (c *Client) post(ctx context.Context, path, token string, body []byte) (*http.Response, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.base.JoinPath(path).String(), bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	if token != "" {
		req.Header.Set("X-Consul-Token", token)
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrUnavailable, err)
	}
	return resp, nil
}
