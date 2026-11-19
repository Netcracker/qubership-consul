// Package idp talks to the platform IdP (Keycloak-based identity-provider): discovery, the
// authorization code flow with PKCE, refresh, back-channel logout and access token validation.
package idp

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"slices"
	"strings"
	"time"

	gooidc "github.com/coreos/go-oidc/v3/oidc"
)

// Options configure the client.
type Options struct {
	// Issuer is the external issuer (the iss claim), e.g. https://gw.example.com/auth/realms/cloud-common.
	Issuer string
	// InternalURL is an optional in-cluster base URL of the same realm for server-side calls.
	InternalURL   string
	ClientID      string
	ClientSecret  string
	Scopes        []string
	Audiences     []string
	UsernameClaim string
	ClockSkew     time.Duration
	RedirectURL   string
	HTTPClient    *http.Client
}

// Client is an OIDC relying party of the IdP.
type Client struct {
	o         Options
	http      *http.Client
	authURL   string // browser-facing
	tokenURL  string // server-side
	logoutURL string // server-side, back-channel logout with the refresh token
	verifier  *gooidc.IDTokenVerifier
	now       func() time.Time
}

// Tokens is the result of a code exchange or a refresh.
type Tokens struct {
	AccessToken  string
	RefreshToken string
	// RefreshExpiry is zero when the IdP does not report it (offline tokens).
	RefreshExpiry time.Time
	// SessionState is the IdP user session id (Keycloak session_state / sid).
	SessionState string
}

// Identity is a validated access token.
type Identity struct {
	Subject  string
	Username string
	Expiry   time.Time
}

var (
	// ErrInvalidToken means an access token failed validation.
	ErrInvalidToken = errors.New("invalid access token")
	// ErrInvalidGrant means the IdP rejected the code or the refresh token: the session is over.
	ErrInvalidGrant = errors.New("IdP rejected the grant")
)

type discovery struct {
	Issuer                string `json:"issuer"`
	AuthorizationEndpoint string `json:"authorization_endpoint"`
	TokenEndpoint         string `json:"token_endpoint"`
	JWKSURI               string `json:"jwks_uri"`
	EndSessionEndpoint    string `json:"end_session_endpoint"`
}

// New runs discovery. When InternalURL is set, discovery and all server-side calls go there with
// X-Forwarded-* headers of the external issuer: the IdP runs with --hostname-strict=false and
// derives iss from the request host, so without them a refreshed token would carry the internal iss.
func New(ctx context.Context, o Options) (*Client, error) {
	o.Issuer = strings.TrimSuffix(o.Issuer, "/")
	o.InternalURL = strings.TrimSuffix(o.InternalURL, "/")
	hc := o.HTTPClient
	if hc == nil {
		hc = http.DefaultClient
	}
	base := o.Issuer
	if o.InternalURL != "" {
		ext, err := url.Parse(o.Issuer)
		if err != nil {
			return nil, err
		}
		hc = &http.Client{Timeout: hc.Timeout, Transport: forwardedHost{next: transportOf(hc), ext: ext}}
		base = o.InternalURL
	}

	doc, err := fetchDiscovery(ctx, hc, base+"/.well-known/openid-configuration")
	if err != nil {
		return nil, err
	}
	if doc.AuthorizationEndpoint == "" || doc.TokenEndpoint == "" || doc.JWKSURI == "" {
		return nil, errors.New("discovery document lacks authorization_endpoint, token_endpoint or jwks_uri")
	}
	if strings.TrimSuffix(doc.Issuer, "/") != o.Issuer {
		return nil, fmt.Errorf("discovery issuer %q does not match OIDC_ISSUER %q", doc.Issuer, o.Issuer)
	}
	// Endpoints are published under the external issuer; server-side calls go to the internal URL.
	internal := func(endpoint string) string {
		if o.InternalURL != "" && strings.HasPrefix(endpoint, o.Issuer) {
			return o.InternalURL + strings.TrimPrefix(endpoint, o.Issuer)
		}
		return endpoint
	}

	keySet := gooidc.NewRemoteKeySet(gooidc.ClientContext(context.WithoutCancel(ctx), hc), internal(doc.JWKSURI))
	skew := o.ClockSkew
	c := &Client{
		o:         o,
		http:      hc,
		authURL:   doc.AuthorizationEndpoint,
		tokenURL:  internal(doc.TokenEndpoint),
		logoutURL: internal(doc.EndSessionEndpoint),
		now:       time.Now,
	}
	c.verifier = gooidc.NewVerifier(o.Issuer, keySet, &gooidc.Config{
		SkipClientIDCheck:    true, // aud is checked against Audiences in Verify
		SupportedSigningAlgs: []string{"RS256", "RS384", "RS512", "ES256", "ES384", "PS256"},
		Now:                  func() time.Time { return time.Now().Add(-skew) },
	})
	return c, nil
}

// NewPKCE returns a PKCE verifier.
func NewPKCE() string {
	b := make([]byte, 32)
	_, _ = rand.Read(b)
	return base64.RawURLEncoding.EncodeToString(b)
}

// AuthCodeURL is the IdP login form URL (authorization code + PKCE S256).
func (c *Client) AuthCodeURL(state, verifier string) string {
	sum := sha256.Sum256([]byte(verifier))
	q := url.Values{
		"response_type":         {"code"},
		"client_id":             {c.o.ClientID},
		"redirect_uri":          {c.o.RedirectURL},
		"scope":                 {strings.Join(c.o.Scopes, " ")},
		"state":                 {state},
		"code_challenge":        {base64.RawURLEncoding.EncodeToString(sum[:])},
		"code_challenge_method": {"S256"},
	}
	sep := "?"
	if strings.Contains(c.authURL, "?") {
		sep = "&"
	}
	return c.authURL + sep + q.Encode()
}

// Exchange trades the authorization code for tokens.
func (c *Client) Exchange(ctx context.Context, code, verifier string) (*Tokens, error) {
	return c.token(ctx, url.Values{
		"grant_type":    {"authorization_code"},
		"code":          {code},
		"code_verifier": {verifier},
		"redirect_uri":  {c.o.RedirectURL},
	})
}

// Refresh gets a new access token (and usually a new refresh token) with the refresh token.
func (c *Client) Refresh(ctx context.Context, refreshToken string) (*Tokens, error) {
	return c.token(ctx, url.Values{
		"grant_type":    {"refresh_token"},
		"refresh_token": {refreshToken},
	})
}

// Logout ends the IdP user session server-side (Keycloak back-channel logout with the refresh
// token), so the next login shows the IdP login form again.
func (c *Client) Logout(ctx context.Context, refreshToken string) error {
	if c.logoutURL == "" || refreshToken == "" {
		return nil
	}
	resp, err := c.post(ctx, c.logoutURL, url.Values{"refresh_token": {refreshToken}})
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 1<<16))
	if resp.StatusCode/100 != 2 {
		return fmt.Errorf("IdP logout returned %d", resp.StatusCode)
	}
	return nil
}

// Verify validates an access token: signature against the JWKS, iss, exp with leeway, and aud
// when audiences are configured.
func (c *Client) Verify(ctx context.Context, raw string) (*Identity, error) {
	t, err := c.verifier.Verify(gooidc.ClientContext(ctx, c.http), raw)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrInvalidToken, err)
	}
	if len(c.o.Audiences) > 0 && !slices.ContainsFunc(t.Audience, func(a string) bool {
		return slices.Contains(c.o.Audiences, a)
	}) {
		return nil, fmt.Errorf("%w: aud %v does not contain any of %v", ErrInvalidToken, t.Audience, c.o.Audiences)
	}
	var claims map[string]any
	_ = t.Claims(&claims)
	username, _ := claims[c.o.UsernameClaim].(string)
	if username == "" {
		username = t.Subject
	}
	return &Identity{Subject: t.Subject, Username: username, Expiry: t.Expiry}, nil
}

type tokenResponse struct {
	AccessToken      string `json:"access_token"`
	RefreshToken     string `json:"refresh_token"`
	RefreshExpiresIn int64  `json:"refresh_expires_in"`
	SessionState     string `json:"session_state"`
	Error            string `json:"error"`
	ErrorDescription string `json:"error_description"`
}

func (c *Client) token(ctx context.Context, form url.Values) (*Tokens, error) {
	resp, err := c.post(ctx, c.tokenURL, form)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	var tr tokenResponse
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	_ = json.Unmarshal(body, &tr)
	if resp.StatusCode != http.StatusOK {
		if tr.Error == "invalid_grant" || tr.Error == "invalid_token" || resp.StatusCode == http.StatusUnauthorized {
			return nil, fmt.Errorf("%w: %s %s", ErrInvalidGrant, tr.Error, tr.ErrorDescription)
		}
		return nil, fmt.Errorf("IdP token endpoint returned %d: %s %s", resp.StatusCode, tr.Error, tr.ErrorDescription)
	}
	if tr.AccessToken == "" {
		return nil, errors.New("IdP token response has no access_token")
	}
	out := &Tokens{AccessToken: tr.AccessToken, RefreshToken: tr.RefreshToken, SessionState: tr.SessionState}
	switch {
	case tr.RefreshExpiresIn > 0:
		out.RefreshExpiry = c.now().Add(time.Duration(tr.RefreshExpiresIn) * time.Second)
	case tr.RefreshToken != "":
		// Keycloak refresh tokens are JWTs signed with a realm secret: exp can be read, not verified.
		out.RefreshExpiry = unverifiedExpiry(tr.RefreshToken)
	}
	return out, nil
}

func (c *Client) post(ctx context.Context, endpoint string, form url.Values) (*http.Response, error) {
	form.Set("client_id", c.o.ClientID)
	if c.o.ClientSecret != "" {
		form.Set("client_secret", c.o.ClientSecret)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, strings.NewReader(form.Encode()))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Accept", "application/json")
	resp, err := c.http.Do(req)
	if err != nil {
		return nil, fmt.Errorf("IdP request failed: %w", err)
	}
	return resp, nil
}

func unverifiedExpiry(jwt string) time.Time {
	parts := strings.Split(jwt, ".")
	if len(parts) != 3 {
		return time.Time{}
	}
	b, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return time.Time{}
	}
	var c struct {
		Exp int64 `json:"exp"`
	}
	if json.Unmarshal(b, &c) != nil || c.Exp == 0 {
		return time.Time{}
	}
	return time.Unix(c.Exp, 0)
}

func fetchDiscovery(ctx context.Context, hc *http.Client, u string) (*discovery, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return nil, err
	}
	resp, err := hc.Do(req)
	if err != nil {
		return nil, fmt.Errorf("discovery request failed: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("discovery %s returned %d", u, resp.StatusCode)
	}
	var doc discovery
	if err := json.NewDecoder(io.LimitReader(resp.Body, 1<<20)).Decode(&doc); err != nil {
		return nil, fmt.Errorf("cannot decode discovery document: %w", err)
	}
	return &doc, nil
}

// forwardedHost makes the IdP build URLs and iss for the external host on internal calls.
type forwardedHost struct {
	next http.RoundTripper
	ext  *url.URL
}

func (f forwardedHost) RoundTrip(r *http.Request) (*http.Response, error) {
	r = r.Clone(r.Context())
	r.Header.Set("X-Forwarded-Host", f.ext.Host)
	r.Header.Set("X-Forwarded-Proto", f.ext.Scheme)
	if p := f.ext.Port(); p != "" {
		r.Header.Set("X-Forwarded-Port", p)
	} else if f.ext.Scheme == "https" {
		r.Header.Set("X-Forwarded-Port", "443")
	} else {
		r.Header.Set("X-Forwarded-Port", "80")
	}
	return f.next.RoundTrip(r)
}

func transportOf(hc *http.Client) http.RoundTripper {
	if hc.Transport != nil {
		return hc.Transport
	}
	return http.DefaultTransport
}
