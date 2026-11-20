package proxy

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/go-jose/go-jose/v4"

	"github.com/Netcracker/qubership-consul/consul-idp-proxy/internal/config"
	"github.com/Netcracker/qubership-consul/consul-idp-proxy/internal/consul"
	"github.com/Netcracker/qubership-consul/consul-idp-proxy/internal/idp"
	"github.com/Netcracker/qubership-consul/consul-idp-proxy/internal/seal"
)

const (
	publicURL = "https://consul.example.com"
	clientID  = "consul-ui"
	realmPath = "/auth/realms/cloud-common"
)

// ---------------------------------------------------------------- fake IdP (Keycloak-like)

type fakeIdP struct {
	t   *testing.T
	srv *httptest.Server
	key *rsa.PrivateKey

	mu         sync.Mutex
	challenges map[string]string // code -> code_challenge
	user       string

	refreshTTL   atomic.Int64 // refresh_expires_in, seconds
	refreshFails atomic.Bool
	refreshes    atomic.Int32
	logouts      atomic.Int32
	lastRefresh  atomic.Value // refresh token presented on the last refresh
	forwarded    atomic.Value // X-Forwarded-Host of the last token request
}

func newFakeIdP(t *testing.T) *fakeIdP {
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	f := &fakeIdP{t: t, key: key, challenges: map[string]string{}, user: "alice"}
	f.refreshTTL.Store(1800)
	mux := http.NewServeMux()
	mux.HandleFunc(realmPath+"/.well-known/openid-configuration", func(w http.ResponseWriter, _ *http.Request) {
		iss := f.issuer()
		_ = json.NewEncoder(w).Encode(map[string]string{
			"issuer":                 iss,
			"authorization_endpoint": iss + "/protocol/openid-connect/auth",
			"token_endpoint":         iss + "/protocol/openid-connect/token",
			"jwks_uri":               iss + "/protocol/openid-connect/certs",
			"end_session_endpoint":   iss + "/protocol/openid-connect/logout",
		})
	})
	mux.HandleFunc(realmPath+"/protocol/openid-connect/certs", func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode(jose.JSONWebKeySet{Keys: []jose.JSONWebKey{
			{Key: &key.PublicKey, KeyID: "k1", Algorithm: "RS256", Use: "sig"},
		}})
	})
	mux.HandleFunc(realmPath+"/protocol/openid-connect/token", f.token)
	mux.HandleFunc(realmPath+"/protocol/openid-connect/logout", func(w http.ResponseWriter, r *http.Request) {
		_ = r.ParseForm()
		if r.Method != http.MethodPost || r.Form.Get("refresh_token") == "" || r.Form.Get("client_id") != clientID {
			http.Error(w, `{"error":"invalid_request"}`, http.StatusBadRequest)
			return
		}
		f.logouts.Add(1)
		w.WriteHeader(http.StatusNoContent)
	})
	f.srv = httptest.NewServer(mux)
	t.Cleanup(f.srv.Close)
	return f
}

func (f *fakeIdP) issuer() string { return f.srv.URL + realmPath }

func (f *fakeIdP) token(w http.ResponseWriter, r *http.Request) {
	_ = r.ParseForm()
	f.forwarded.Store(r.Header.Get("X-Forwarded-Host"))
	if r.Form.Get("client_id") != clientID {
		http.Error(w, `{"error":"unauthorized_client"}`, http.StatusUnauthorized)
		return
	}
	switch r.Form.Get("grant_type") {
	case "authorization_code":
		code := r.Form.Get("code")
		f.mu.Lock()
		challenge, ok := f.challenges[code]
		delete(f.challenges, code)
		f.mu.Unlock()
		sum := sha256.Sum256([]byte(r.Form.Get("code_verifier")))
		if !ok || base64.RawURLEncoding.EncodeToString(sum[:]) != challenge ||
			r.Form.Get("redirect_uri") != publicURL+"/auth/callback" {
			http.Error(w, `{"error":"invalid_grant"}`, http.StatusBadRequest)
			return
		}
	case "refresh_token":
		f.refreshes.Add(1)
		f.lastRefresh.Store(r.Form.Get("refresh_token"))
		if f.refreshFails.Load() {
			http.Error(w, `{"error":"invalid_grant","error_description":"Session not active"}`, http.StatusBadRequest)
			return
		}
	default:
		http.Error(w, `{"error":"unsupported_grant_type"}`, http.StatusBadRequest)
		return
	}
	now := time.Now()
	f.mu.Lock()
	user := f.user
	f.mu.Unlock()
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{
		"access_token": f.sign(map[string]any{
			"iss": f.issuer(), "sub": "sub-" + user, "aud": "account", "typ": "Bearer", "azp": clientID,
			"exp": now.Add(5 * time.Minute).Unix(), "iat": now.Unix(),
			"preferred_username": user, "realm_access": map[string]any{"roles": []string{"consul-readers"}},
		}),
		"expires_in":         300,
		"refresh_token":      fmt.Sprintf("rt-%d", f.refreshes.Load()),
		"refresh_expires_in": f.refreshTTL.Load(),
		"token_type":         "Bearer",
		"session_state":      "kc-session-1",
	})
}

// authorize simulates the user logging in at the IdP login form.
func (f *fakeIdP) authorize(t *testing.T, authURL string) (code, state string) {
	t.Helper()
	u, err := url.Parse(authURL)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(authURL, f.issuer()+"/protocol/openid-connect/auth?") {
		t.Fatalf("unexpected authorization endpoint: %s", authURL)
	}
	q := u.Query()
	if q.Get("code_challenge_method") != "S256" || q.Get("code_challenge") == "" ||
		q.Get("client_id") != clientID || q.Get("redirect_uri") != publicURL+"/auth/callback" {
		t.Fatalf("bad authorization request: %s", authURL)
	}
	code = fmt.Sprintf("code-%d", time.Now().UnixNano())
	f.mu.Lock()
	f.challenges[code] = q.Get("code_challenge")
	f.mu.Unlock()
	return code, q.Get("state")
}

func (f *fakeIdP) sign(claims map[string]any) string {
	signer, err := jose.NewSigner(jose.SigningKey{Algorithm: jose.RS256, Key: f.key},
		(&jose.SignerOptions{}).WithType("JWT").WithHeader("kid", "k1"))
	if err != nil {
		f.t.Fatal(err)
	}
	payload, _ := json.Marshal(claims)
	obj, err := signer.Sign(payload)
	if err != nil {
		f.t.Fatal(err)
	}
	s, _ := obj.CompactSerialize()
	return s
}

// ---------------------------------------------------------------- fake Consul

type fakeConsul struct {
	srv       *httptest.Server
	logins    atomic.Int32
	logouts   atomic.Int32
	tokenTTL  atomic.Int64 // seconds
	mu        sync.Mutex
	lastToken string
	lastHdr   http.Header
	lastQuery url.Values
	hits      int
}

func newFakeConsul(t *testing.T) *fakeConsul {
	f := &fakeConsul{}
	f.tokenTTL.Store(3600)
	f.srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/v1/acl/login":
			var body struct{ AuthMethod, BearerToken string }
			_ = json.NewDecoder(r.Body).Decode(&body)
			if body.AuthMethod != "platform-idp" || body.BearerToken == "" {
				http.Error(w, "auth method not found", http.StatusForbidden)
				return
			}
			if strings.Contains(payloadOf(body.BearerToken), `"preferred_username":"nobody"`) {
				http.Error(w, "Permission denied", http.StatusForbidden)
				return
			}
			n := f.logins.Add(1)
			_ = json.NewEncoder(w).Encode(map[string]any{
				"AccessorID":     fmt.Sprintf("acc-%d", n),
				"SecretID":       fmt.Sprintf("sec-%d", n),
				"ExpirationTime": time.Now().Add(time.Duration(f.tokenTTL.Load()) * time.Second).Format(time.RFC3339),
			})
		case "/v1/acl/logout":
			f.logouts.Add(1)
		case "/v1/kv/revoked":
			http.Error(w, "ACL not found", http.StatusForbidden)
		default:
			f.mu.Lock()
			f.lastToken = r.Header.Get("X-Consul-Token")
			f.lastHdr = r.Header.Clone()
			f.lastQuery = r.URL.Query()
			f.hits++
			f.mu.Unlock()
			_, _ = io.WriteString(w, "ok")
		}
	}))
	t.Cleanup(f.srv.Close)
	return f
}

func (f *fakeConsul) last() (string, http.Header, int) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.lastToken, f.lastHdr, f.hits
}

func payloadOf(jwt string) string {
	parts := strings.Split(jwt, ".")
	if len(parts) != 3 {
		return ""
	}
	b, _ := base64.RawURLEncoding.DecodeString(parts[1])
	return string(b)
}

// ---------------------------------------------------------------- harness

type harness struct {
	t       *testing.T
	idp     *fakeIdP
	consul  *fakeConsul
	cfg     *config.Config
	srv     *Server
	handler http.Handler
	jar     map[string]*http.Cookie // a minimal browser cookie jar
}

func newHarness(t *testing.T, mutate ...func(*config.Config)) *harness {
	t.Helper()
	fi := newFakeIdP(t)
	fc := newFakeConsul(t)
	env := map[string]string{
		"PUBLIC_URL":     publicURL,
		"OIDC_ISSUER":    fi.issuer(),
		"OIDC_CLIENT_ID": clientID,
		"CONSUL_ADDR":    fc.srv.URL,
		"COOKIE_SECRET":  strings.Repeat("s", 32),
	}
	cfg, err := config.Load(func(k string) string { return env[k] })
	if err != nil {
		t.Fatal(err)
	}
	for _, m := range mutate {
		m(cfg)
	}
	provider, err := idp.New(context.Background(), idp.Options{
		Issuer: cfg.OIDCIssuer, InternalURL: cfg.OIDCInternalURL, ClientID: cfg.OIDCClientID,
		Scopes: cfg.OIDCScopes, UsernameClaim: cfg.UsernameClaim, ClockSkew: cfg.ClockSkew,
		RedirectURL: cfg.PublicPath("/auth/callback"),
	})
	if err != nil {
		t.Fatal(err)
	}
	sealer, _ := seal.New(cfg.CookieSecret)
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	cc := consul.New(cfg.ConsulAddr, cfg.ConsulAuthMethod, http.DefaultClient)
	srv := New(cfg, provider, cc, sealer, http.DefaultTransport, log)
	return &harness{t: t, idp: fi, consul: fc, cfg: cfg, srv: srv, handler: srv.Handler(), jar: map[string]*http.Cookie{}}
}

// do sends a request with the jar cookies and stores the cookies of the response.
func (h *harness) do(req *http.Request) *http.Response {
	for _, c := range h.jar {
		if strings.HasPrefix(req.URL.Path, c.Path) {
			req.AddCookie(&http.Cookie{Name: c.Name, Value: c.Value})
		}
	}
	rec := httptest.NewRecorder()
	h.handler.ServeHTTP(rec, req)
	resp := rec.Result()
	for _, c := range resp.Cookies() {
		if c.MaxAge < 0 {
			delete(h.jar, c.Name)
		} else {
			h.jar[c.Name] = c
		}
	}
	return resp
}

func (h *harness) get(path string, navigate bool) *http.Response {
	req := httptest.NewRequest(http.MethodGet, publicURL+path, nil)
	if navigate {
		req.Header.Set("Sec-Fetch-Mode", "navigate")
	}
	return h.do(req)
}

// login runs the browser flow: /ui/ -> /auth/login -> IdP form -> /auth/callback -> /ui/.
func (h *harness) login() {
	t := h.t
	t.Helper()
	resp := h.get("/ui/services", true)
	if resp.StatusCode != http.StatusFound || !strings.HasPrefix(resp.Header.Get("Location"), "/auth/login?") {
		t.Fatalf("expected redirect to the login, got %d %s", resp.StatusCode, resp.Header.Get("Location"))
	}
	resp = h.get(resp.Header.Get("Location"), true)
	if resp.StatusCode != http.StatusFound {
		t.Fatalf("login: %d", resp.StatusCode)
	}
	code, state := h.idp.authorize(t, resp.Header.Get("Location"))
	resp = h.get("/auth/callback?code="+url.QueryEscape(code)+"&state="+url.QueryEscape(state), true)
	if resp.StatusCode != http.StatusFound || resp.Header.Get("Location") != "/ui/services" {
		b, _ := io.ReadAll(resp.Body)
		t.Fatalf("callback: %d %s %s", resp.StatusCode, resp.Header.Get("Location"), b)
	}
	for _, name := range []string{cookieSession, cookieRefresh, cookieToken} {
		c := h.jar[name]
		if c == nil || !c.HttpOnly || !c.Secure || c.SameSite != http.SameSiteLaxMode {
			t.Fatalf("cookie %s missing or without HttpOnly/Secure/Lax: %+v", name, c)
		}
	}
}

func (h *harness) session() map[string]any {
	resp := h.get("/auth/session", false)
	var m map[string]any
	_ = json.NewDecoder(resp.Body).Decode(&m)
	return m
}

// ---------------------------------------------------------------- tests

func TestNoCredentials(t *testing.T) {
	h := newHarness(t)
	resp := h.get("/v1/kv/demo", false)
	if resp.StatusCode != http.StatusUnauthorized || resp.Header.Get("WWW-Authenticate") == "" {
		t.Fatalf("expected 401 with WWW-Authenticate, got %d", resp.StatusCode)
	}
	if _, _, hits := h.consul.last(); hits != 0 {
		t.Fatal("request without credentials reached Consul")
	}
}

func TestBrowserLoginStoresEverythingInCookies(t *testing.T) {
	h := newHarness(t)
	h.login()
	if h.consul.logins.Load() != 1 {
		t.Fatalf("expected one Consul login, got %d", h.consul.logins.Load())
	}
	// The refresh token and the Consul SecretID are not readable from the cookies.
	for _, c := range h.jar {
		if strings.Contains(c.Value, "rt-") || strings.Contains(c.Value, "sec-") {
			t.Fatalf("cookie %s is not encrypted", c.Name)
		}
	}

	req := httptest.NewRequest(http.MethodGet, publicURL+"/v1/kv/demo?token=evil", nil)
	req.Header.Set("X-Consul-Token", "evil")
	resp := h.do(req)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("proxied request: %d", resp.StatusCode)
	}
	tok, hdr, _ := h.consul.last()
	if tok != "sec-1" {
		t.Fatalf("X-Consul-Token = %q, want sec-1", tok)
	}
	if hdr.Get("Cookie") != "" || hdr.Get("Authorization") != "" {
		t.Fatal("cookies or Authorization leaked to Consul")
	}
	if h.consul.logins.Load() != 1 || h.idp.refreshes.Load() != 0 {
		t.Fatal("a fresh session must not refresh or log in again")
	}

	s := h.session()
	if s["authenticated"] != true || s["username"] != "alice" || s["idpSession"] != "kc-session-1" {
		t.Fatalf("unexpected session: %v", s)
	}
}

func TestConsulTokenAboutToExpire(t *testing.T) {
	h := newHarness(t)
	h.consul.tokenTTL.Store(60) // below CONSUL_TOKEN_REFRESH_BEFORE (2m)
	h.login()
	h.get("/v1/kv/demo", false)
	if h.idp.refreshes.Load() != 1 || h.consul.logins.Load() != 2 {
		t.Fatalf("expected refresh + new Consul login, got refreshes=%d logins=%d",
			h.idp.refreshes.Load(), h.consul.logins.Load())
	}
	if tok, _, _ := h.consul.last(); tok != "sec-2" {
		t.Fatalf("expected the new Consul token, got %q", tok)
	}
	if got := h.idp.lastRefresh.Load(); got != "rt-0" {
		t.Fatalf("refresh used %v, want the refresh token from the login", got)
	}
}

func TestRefreshTokenAboutToExpire(t *testing.T) {
	h := newHarness(t)
	h.idp.refreshTTL.Store(600)
	h.login()

	// Right after the login the refresh token has 10 minutes left: nothing to do.
	h.get("/v1/kv/demo", false)
	if h.idp.refreshes.Load() != 0 {
		t.Fatal("unexpected refresh")
	}

	// 9m10s later less than REFRESH_TOKEN_LOGIN_BEFORE (1m) is left: no refresh, the user is sent
	// to the IdP login form and the obsolete Consul token is revoked.
	h.srv.now = func() time.Time { return time.Now().Add(9*time.Minute + 10*time.Second) }
	resp := h.get("/ui/", true)
	if resp.StatusCode != http.StatusFound || !strings.HasPrefix(resp.Header.Get("Location"), "/auth/login") {
		t.Fatalf("expected redirect to the login, got %d", resp.StatusCode)
	}
	if h.idp.refreshes.Load() != 0 {
		t.Fatal("a refresh token that is about to expire must not be used")
	}
	if h.consul.logouts.Load() != 1 {
		t.Fatal("the obsolete Consul token must be logged out")
	}
	if len(h.jar) != 0 {
		t.Fatalf("session cookies must be cleared, left: %v", h.jar)
	}
}

func TestRefreshTokenIsReused(t *testing.T) {
	h := newHarness(t)
	h.idp.refreshTTL.Store(600)
	h.login()
	// The Consul token is gone -> refresh with the refresh token from the login.
	delete(h.jar, cookieToken)
	h.get("/v1/kv/demo", false)
	if got := h.idp.lastRefresh.Load(); got != "rt-0" {
		t.Fatalf("refresh used %v, want rt-0", got)
	}
	// The IdP rotated it; the next refresh must use the new one.
	delete(h.jar, cookieToken)
	h.get("/v1/kv/demo", false)
	if got := h.idp.lastRefresh.Load(); got != "rt-1" {
		t.Fatalf("refresh used %v, want the rotated rt-1", got)
	}
}

func TestRefreshRejectedEndsSession(t *testing.T) {
	h := newHarness(t)
	h.consul.tokenTTL.Store(60)
	h.login()
	h.idp.refreshFails.Store(true)
	resp := h.get("/ui/", true)
	if resp.StatusCode != http.StatusFound || !strings.HasPrefix(resp.Header.Get("Location"), "/auth/login") {
		t.Fatalf("expected redirect to the login, got %d", resp.StatusCode)
	}
	if h.consul.logouts.Load() != 1 {
		t.Fatal("the obsolete Consul token must be logged out")
	}
	if len(h.jar) != 0 {
		t.Fatalf("session cookies must be cleared, left: %v", h.jar)
	}
}

func TestRefreshTokenExpiredEndsSession(t *testing.T) {
	h := newHarness(t)
	h.idp.refreshTTL.Store(1)
	h.login()
	time.Sleep(1100 * time.Millisecond)
	resp := h.get("/v1/kv/demo", false)
	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", resp.StatusCode)
	}
	if h.idp.refreshes.Load() != 0 {
		t.Fatal("an expired refresh token must not be used")
	}
}

func TestTamperedCookieRejected(t *testing.T) {
	h := newHarness(t)
	h.login()
	c := h.jar[cookieSession]
	c.Value = c.Value[:len(c.Value)-2] + "AA"
	if resp := h.get("/v1/kv/demo", false); resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("expected 401 for a tampered cookie, got %d", resp.StatusCode)
	}
}

func TestCookieWritesRequireSameOrigin(t *testing.T) {
	h := newHarness(t)
	h.login()
	req := httptest.NewRequest(http.MethodPut, publicURL+"/v1/kv/x", strings.NewReader("v"))
	req.Header.Set("Origin", "https://evil.example.com")
	if resp := h.do(req); resp.StatusCode != http.StatusForbidden {
		t.Fatalf("cross-origin write: %d", resp.StatusCode)
	}
	req = httptest.NewRequest(http.MethodPut, publicURL+"/v1/kv/x", strings.NewReader("v"))
	req.Header.Set("Origin", publicURL)
	if resp := h.do(req); resp.StatusCode != http.StatusOK {
		t.Fatalf("same-origin write: %d", resp.StatusCode)
	}
}

func TestACLNotFoundDropsTokenCookie(t *testing.T) {
	h := newHarness(t)
	h.login()
	if resp := h.get("/v1/kv/revoked", false); resp.StatusCode != http.StatusForbidden {
		t.Fatalf("expected 403 passthrough, got %d", resp.StatusCode)
	}
	if _, ok := h.jar[cookieToken]; ok {
		t.Fatal("token cookie must be dropped")
	}
	h.get("/v1/kv/demo", false)
	if h.consul.logins.Load() != 2 {
		t.Fatal("the next request must log in to Consul again")
	}
}

func TestNoRoleMapped(t *testing.T) {
	h := newHarness(t)
	h.idp.user = "nobody"
	resp := h.get("/auth/login", true)
	code, state := h.idp.authorize(t, resp.Header.Get("Location"))
	resp = h.get("/auth/callback?code="+code+"&state="+state, true)
	if resp.StatusCode != http.StatusForbidden {
		t.Fatalf("expected 403, got %d", resp.StatusCode)
	}
	if h.idp.logouts.Load() != 1 {
		t.Fatal("the IdP session must be ended")
	}
	if _, ok := h.jar[cookieSession]; ok {
		t.Fatal("no session cookie expected")
	}
}

func TestCallbackRejectsBadState(t *testing.T) {
	h := newHarness(t)
	resp := h.get("/auth/login", true)
	code, _ := h.idp.authorize(t, resp.Header.Get("Location"))
	if resp := h.get("/auth/callback?code="+code+"&state=forged", true); resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", resp.StatusCode)
	}
}

func TestLogout(t *testing.T) {
	h := newHarness(t)
	h.login()
	resp := h.get("/auth/logout", true)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("logout: %d", resp.StatusCode)
	}
	if h.consul.logouts.Load() != 1 || h.idp.logouts.Load() != 1 {
		t.Fatalf("expected Consul and IdP logout, got %d %d", h.consul.logouts.Load(), h.idp.logouts.Load())
	}
	if len(h.jar) != 0 {
		t.Fatal("cookies must be cleared")
	}
}

func TestBearer(t *testing.T) {
	h := newHarness(t)
	now := time.Now()
	jwt := h.idp.sign(map[string]any{"iss": h.idp.issuer(), "sub": "svc", "aud": "account",
		"exp": now.Add(time.Minute).Unix(), "iat": now.Unix(), "preferred_username": "svc"})
	req := httptest.NewRequest(http.MethodGet, publicURL+"/v1/kv/demo", nil)
	req.Header.Set("Authorization", "Bearer "+jwt)
	if resp := h.do(req); resp.StatusCode != http.StatusOK {
		t.Fatalf("bearer: %d", resp.StatusCode)
	}
	if tok, hdr, _ := h.consul.last(); tok != "sec-1" || hdr.Get("Authorization") != "" {
		t.Fatal("bearer request must carry the Consul token only")
	}
	if h.consul.logouts.Load() != 1 {
		t.Fatal("one-shot Consul token must be logged out after the request")
	}

	expired := h.idp.sign(map[string]any{"iss": h.idp.issuer(), "sub": "svc",
		"exp": now.Add(-time.Hour).Unix(), "iat": now.Add(-2 * time.Hour).Unix()})
	req = httptest.NewRequest(http.MethodGet, publicURL+"/v1/kv/demo", nil)
	req.Header.Set("Authorization", "Bearer "+expired)
	if resp := h.do(req); resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("expired bearer: %d", resp.StatusCode)
	}
	if h.consul.logins.Load() != 1 {
		t.Fatal("an expired JWT must not reach Consul")
	}
}

func TestInternalURLSendsForwardedHost(t *testing.T) {
	fi := newFakeIdP(t)
	// The fake publishes endpoints under its own URL; use it as both issuer and internal URL and
	// check the header only.
	p, err := idp.New(context.Background(), idp.Options{
		Issuer: fi.issuer(), InternalURL: fi.issuer(), ClientID: clientID, RedirectURL: publicURL + "/auth/callback",
	})
	if err != nil {
		t.Fatal(err)
	}
	_, _ = p.Refresh(context.Background(), "rt")
	u, _ := url.Parse(fi.srv.URL)
	if fi.forwarded.Load() != u.Host {
		t.Fatalf("X-Forwarded-Host = %v, want %s", fi.forwarded.Load(), u.Host)
	}
}

func TestSafeReturnTo(t *testing.T) {
	for in, want := range map[string]string{
		"":                  defaultHome,
		"/ui/kv":            "/ui/kv",
		"//evil.com":        defaultHome,
		"https://evil.com":  defaultHome,
		"/auth/logout":      defaultHome,
		"/\\evil.com":       defaultHome,
		"/v1/kv/a?recurse=": "/v1/kv/a?recurse=",
	} {
		if got := safeReturnTo(in); got != want {
			t.Errorf("safeReturnTo(%q) = %q, want %q", in, got, want)
		}
	}
}
