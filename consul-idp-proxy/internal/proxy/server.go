// Package proxy is the authenticating reverse proxy in front of the Consul HTTP API and UI.
//
// The proxy keeps no state. Everything it needs between requests lives in encrypted cookies:
//
//	consul_idp_session  IdP session: subject, username, session id, refresh token expiry, timestamps
//	consul_idp_refresh  IdP refresh token
//	consul_idp_token    Consul ACL token (SecretID, AccessorID, expiry) obtained via /v1/acl/login
//	consul_idp_login    PKCE verifier, state and return path while the login is in progress
//
// On every browser request the proxy follows the design diagram: check the refresh token TTL, then
// the Consul token TTL, refresh both through the IdP and /v1/acl/login when one of them is about
// to expire, and forward the request with X-Consul-Token.
package proxy

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/subtle"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"html"
	"io"
	"log/slog"
	"net/http"
	"net/http/httputil"
	"net/url"
	"strings"
	"time"

	"github.com/Netcracker/qubership-consul/consul-idp-proxy/internal/config"
	"github.com/Netcracker/qubership-consul/consul-idp-proxy/internal/consul"
	"github.com/Netcracker/qubership-consul/consul-idp-proxy/internal/idp"
	"github.com/Netcracker/qubership-consul/consul-idp-proxy/internal/seal"
)

const (
	cookieSession = "consul_idp_session"
	cookieRefresh = "consul_idp_refresh"
	cookieToken   = "consul_idp_token"
	cookieLogin   = "consul_idp_login"

	loginTTL    = 10 * time.Minute
	callTimeout = 15 * time.Second
	defaultHome = "/ui/"
)

// IdP is the part of the IdP client the proxy uses.
type IdP interface {
	AuthCodeURL(state, verifier string) string
	Exchange(ctx context.Context, code, verifier string) (*idp.Tokens, error)
	Refresh(ctx context.Context, refreshToken string) (*idp.Tokens, error)
	Logout(ctx context.Context, refreshToken string) error
	Verify(ctx context.Context, raw string) (*idp.Identity, error)
}

// Consul is the part of the Consul client the proxy uses.
type Consul interface {
	Login(ctx context.Context, jwt string, meta map[string]string) (*consul.Token, error)
	Logout(ctx context.Context, secretID string) error
}

// Cookie payloads. Short JSON names keep the cookies small.
type sessionData struct {
	Subject     string `json:"sub"`
	Username    string `json:"usr"`
	SID         string `json:"sid,omitempty"`
	RefreshExp  int64  `json:"rx,omitempty"` // 0: the IdP did not report it
	RefreshedAt int64  `json:"ra"`
	CreatedAt   int64  `json:"ca"`
}

type refreshData struct {
	Token string `json:"rt"`
}

type tokenData struct {
	SecretID   string `json:"s"`
	AccessorID string `json:"a"`
	Exp        int64  `json:"x"`
}

type loginData struct {
	State    string `json:"s"`
	Verifier string `json:"v"`
	ReturnTo string `json:"r"`
}

type cookieAuthCtx struct{}

// Server is the proxy HTTP server.
type Server struct {
	cfg    *config.Config
	idp    IdP
	consul Consul
	seal   *seal.Sealer
	proxy  *httputil.ReverseProxy
	log    *slog.Logger
	now    func() time.Time
}

// New builds the server. transport is used for requests to Consul.
func New(cfg *config.Config, p IdP, c Consul, s *seal.Sealer, transport http.RoundTripper, log *slog.Logger) *Server {
	srv := &Server{cfg: cfg, idp: p, consul: c, seal: s, log: log, now: time.Now}
	upstream := cfg.ConsulAddr
	srv.proxy = &httputil.ReverseProxy{
		Rewrite: func(pr *httputil.ProxyRequest) {
			pr.SetURL(upstream)
			pr.SetXForwarded()
		},
		Transport:      transport,
		FlushInterval:  -1, // blocking queries and streams must not be buffered
		ModifyResponse: srv.modifyResponse,
		ErrorHandler: func(w http.ResponseWriter, r *http.Request, err error) {
			srv.log.Warn("upstream request failed", "path", r.URL.Path, "error", err)
			writeError(w, http.StatusBadGateway, "Consul is not reachable")
		},
	}
	return srv
}

// Handler returns the HTTP handler.
func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusOK) })
	mux.HandleFunc("GET /auth/login", s.handleLogin)
	mux.HandleFunc("GET /auth/callback", s.handleCallback)
	mux.HandleFunc("/auth/logout", s.handleLogout)
	mux.HandleFunc("GET /auth/session", s.handleSession)
	mux.HandleFunc("GET /{$}", func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, defaultHome, http.StatusFound)
	})
	mux.HandleFunc("/", s.handleProxy)
	return mux
}

// ---------------------------------------------------------------- login / logout

func (s *Server) handleLogin(w http.ResponseWriter, r *http.Request) {
	ld := loginData{State: randomString(), Verifier: idp.NewPKCE(), ReturnTo: safeReturnTo(r.URL.Query().Get("return_to"))}
	if err := s.setCookie(w, cookieLogin, "/auth/callback", ld, s.now().Add(loginTTL)); err != nil {
		s.log.Error("cannot seal login cookie", "error", err)
		writeError(w, http.StatusInternalServerError, "cannot start the login")
		return
	}
	http.Redirect(w, r, s.idp.AuthCodeURL(ld.State, ld.Verifier), http.StatusFound)
}

func (s *Server) handleCallback(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	var ld loginData
	if !s.readCookie(r, cookieLogin, &ld) || q.Get("state") == "" ||
		subtle.ConstantTimeCompare([]byte(ld.State), []byte(q.Get("state"))) != 1 {
		writeHTML(w, http.StatusBadRequest, "Login state is missing or expired.", true)
		return
	}
	s.clearCookie(w, cookieLogin, "/auth/callback")
	if e := q.Get("error"); e != "" {
		s.log.Info("IdP returned an error", "error", e, "description", q.Get("error_description"))
		writeHTML(w, http.StatusUnauthorized, "Login failed: "+e, true)
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), callTimeout)
	defer cancel()
	tokens, err := s.idp.Exchange(ctx, q.Get("code"), ld.Verifier)
	if err != nil {
		s.log.Warn("code exchange failed", "error", err)
		writeHTML(w, http.StatusUnauthorized, "Login failed: the IdP did not issue tokens.", true)
		return
	}
	// The IdP session is created now; the diagram step "/consul/login" follows.
	sess := sessionData{CreatedAt: s.now().Unix()}
	tok, err := s.establish(ctx, w, &sess, tokens)
	if err != nil {
		s.writeEstablishError(w, r, err, tokens.RefreshToken, sess.Username)
		return
	}
	s.log.Info("user logged in", "username", sess.Username, "subject", sess.Subject, "accessor", tok.AccessorID)
	http.Redirect(w, r, ld.ReturnTo, http.StatusFound)
}

func (s *Server) handleLogout(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet && r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	if r.Method == http.MethodPost && !s.sameOrigin(r) {
		writeError(w, http.StatusForbidden, "cross-origin request rejected")
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), callTimeout)
	defer cancel()
	var sess sessionData
	_ = s.readCookie(r, cookieSession, &sess)
	s.revokeConsulToken(ctx, r)
	var rd refreshData
	if s.readCookie(r, cookieRefresh, &rd) {
		if err := s.idp.Logout(ctx, rd.Token); err != nil {
			s.log.Warn("IdP logout failed", "error", err)
		}
	}
	s.clearSession(w)
	s.log.Info("user logged out", "username", sess.Username)
	writeHTML(w, http.StatusOK, "You are logged out.", true)
}

// handleSession reports the session state; it also runs the refresh logic, so it is a convenient
// way to check the flow by hand.
func (s *Server) handleSession(w http.ResponseWriter, r *http.Request) {
	st := s.ensure(w, r)
	resp := map[string]any{"authenticated": st.outcome == outcomeOK}
	if st.outcome == outcomeOK {
		resp["username"] = st.sess.Username
		resp["subject"] = st.sess.Subject
		resp["idpSession"] = st.sess.SID
		resp["refreshTokenExpiresAt"] = unixOrNil(st.sess.RefreshExp)
		resp["refreshedAt"] = time.Unix(st.sess.RefreshedAt, 0).UTC()
		resp["consulAccessorID"] = st.tok.AccessorID
		resp["consulTokenExpiresAt"] = time.Unix(st.tok.Exp, 0).UTC()
	} else if st.err != nil {
		resp["error"] = st.err.Error()
	}
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	_ = json.NewEncoder(w).Encode(resp)
}

// ---------------------------------------------------------------- per-request session logic

type outcome int

const (
	outcomeOK          outcome = iota
	outcomeNoSession           // no or unreadable session cookies
	outcomeExpired             // the refresh token expired or the IdP rejected it
	outcomeDenied              // no Consul role is mapped to the user
	outcomeUnavailable         // the IdP or Consul failed and there is no valid Consul token
)

type state struct {
	outcome outcome
	sess    sessionData
	tok     tokenData
	err     error
}

// ensure implements the diagram for a cookie-authenticated request and writes updated cookies.
func (s *Server) ensure(w http.ResponseWriter, r *http.Request) state {
	var sess sessionData
	var rd refreshData
	if !s.readCookie(r, cookieSession, &sess) || !s.readCookie(r, cookieRefresh, &rd) {
		if hasCookie(r, cookieSession) || hasCookie(r, cookieRefresh) || hasCookie(r, cookieToken) {
			s.endSession(w, r, "session cookies are missing or expired")
		}
		return state{outcome: outcomeNoSession}
	}
	now := s.now()

	// Refresh Token TTL: expired -> remove the obsolete Consul token -> IdP login form.
	if (sess.RefreshExp != 0 && now.Unix() >= sess.RefreshExp) ||
		now.Sub(time.Unix(sess.CreatedAt, 0)) >= s.cfg.SessionMaxAge {
		s.endSession(w, r, "refresh token or session expired")
		return state{outcome: outcomeExpired}
	}

	var tok tokenData
	haveTok := s.readCookie(r, cookieToken, &tok)

	refreshSoon := sess.RefreshExp != 0 &&
		time.Unix(sess.RefreshExp, 0).Sub(now) <= s.cfg.RefreshTokenRefreshBefore &&
		// Near the IdP session max lifetime a refresh does not move the expiry: do not refresh on
		// every request, once per window is enough.
		now.Sub(time.Unix(sess.RefreshedAt, 0)) > s.cfg.RefreshTokenRefreshBefore
	consulSoon := !haveTok || time.Unix(tok.Exp, 0).Sub(now) <= s.cfg.ConsulTokenRefreshBefore

	if !refreshSoon && !consulSoon {
		return state{outcome: outcomeOK, sess: sess, tok: tok}
	}

	// Get a new JWT by the refresh token, then /consul/login.
	ctx, cancel := context.WithTimeout(context.WithoutCancel(r.Context()), callTimeout)
	defer cancel()
	tokens, err := s.idp.Refresh(ctx, rd.Token)
	if err == nil {
		if tokens.RefreshToken == "" {
			tokens.RefreshToken = rd.Token // the IdP does not rotate refresh tokens
		}
		var newTok tokenData
		newTok, err = s.establish(ctx, w, &sess, tokens)
		if err == nil {
			s.log.Debug("session refreshed", "username", sess.Username, "accessor", newTok.AccessorID,
				"refresh_soon", refreshSoon, "consul_soon", consulSoon)
			// The old Consul token is not logged out: requests in flight (blocking queries) still
			// use it, and it expires on its own after the auth method MaxTokenTTL.
			return state{outcome: outcomeOK, sess: sess, tok: newTok}
		}
	}

	switch {
	case errors.Is(err, idp.ErrInvalidGrant):
		s.endSession(w, r, err.Error())
		return state{outcome: outcomeExpired, err: err}
	case errors.Is(err, consul.ErrLoginDenied):
		s.clearCookie(w, cookieToken, "/")
		return state{outcome: outcomeDenied, sess: sess, err: err}
	case haveTok:
		// The IdP or Consul is temporarily unavailable but the current Consul token is still valid.
		s.log.Warn("refresh failed, the current Consul token is still valid", "username", sess.Username, "error", err)
		return state{outcome: outcomeOK, sess: sess, tok: tok}
	default:
		s.log.Warn("refresh failed", "username", sess.Username, "error", err)
		return state{outcome: outcomeUnavailable, sess: sess, err: err}
	}
}

// establish validates the IdP access token, logs in to Consul with it and writes the session,
// refresh token and Consul token cookies. sess is updated in place.
func (s *Server) establish(ctx context.Context, w http.ResponseWriter, sess *sessionData, tokens *idp.Tokens) (tokenData, error) {
	id, err := s.idp.Verify(ctx, tokens.AccessToken)
	if err != nil {
		return tokenData{}, err
	}
	sess.Subject, sess.Username = id.Subject, id.Username
	if tokens.SessionState != "" {
		sess.SID = tokens.SessionState
	}
	if !tokens.RefreshExpiry.IsZero() {
		sess.RefreshExp = tokens.RefreshExpiry.Unix()
	}
	sess.RefreshedAt = s.now().Unix()
	if tokens.RefreshToken == "" {
		return tokenData{}, errors.New("the IdP did not issue a refresh token")
	}

	ct, err := s.consul.Login(ctx, tokens.AccessToken, map[string]string{
		"source": "consul-idp-proxy", "username": id.Username,
	})
	if err != nil {
		return tokenData{}, err
	}
	exp := s.now().Add(time.Hour)
	if ct.ExpirationTime != nil {
		exp = *ct.ExpirationTime
	}
	tok := tokenData{SecretID: ct.SecretID, AccessorID: ct.AccessorID, Exp: exp.Unix()}

	sessionEnd := time.Unix(sess.CreatedAt, 0).Add(s.cfg.SessionMaxAge)
	if sess.RefreshExp != 0 && time.Unix(sess.RefreshExp, 0).Before(sessionEnd) {
		sessionEnd = time.Unix(sess.RefreshExp, 0)
	}
	if exp.After(sessionEnd) {
		exp = sessionEnd
	}
	if err := errors.Join(
		s.setCookie(w, cookieSession, "/", sess, sessionEnd),
		s.setCookie(w, cookieRefresh, "/", refreshData{Token: tokens.RefreshToken}, sessionEnd),
		s.setCookie(w, cookieToken, "/", tok, exp),
	); err != nil {
		return tokenData{}, err
	}
	return tok, nil
}

func (s *Server) writeEstablishError(w http.ResponseWriter, r *http.Request, err error, refreshToken, username string) {
	ctx, cancel := context.WithTimeout(context.WithoutCancel(r.Context()), callTimeout)
	defer cancel()
	s.clearSession(w)
	switch {
	case errors.Is(err, consul.ErrLoginDenied):
		s.log.Info("consul login denied", "username", username, "error", err)
		// End the IdP session, so the user can log in as somebody else.
		_ = s.idp.Logout(ctx, refreshToken)
		writeHTML(w, http.StatusForbidden, fmt.Sprintf("No Consul role is mapped to the user %q.", username), true)
	case errors.Is(err, idp.ErrInvalidToken):
		s.log.Warn("access token from the IdP failed validation", "error", err)
		writeHTML(w, http.StatusUnauthorized, "Login failed: the access token was rejected.", true)
	default:
		s.log.Warn("login failed", "error", err)
		writeHTML(w, http.StatusBadGateway, "Login failed: Consul or the IdP is not available.", true)
	}
}

// endSession removes the obsolete Consul token and all session cookies.
func (s *Server) endSession(w http.ResponseWriter, r *http.Request, reason string) {
	ctx, cancel := context.WithTimeout(context.WithoutCancel(r.Context()), callTimeout)
	defer cancel()
	s.revokeConsulToken(ctx, r)
	s.clearSession(w)
	s.log.Info("session ended", "reason", reason)
}

func (s *Server) revokeConsulToken(ctx context.Context, r *http.Request) {
	var tok tokenData
	if s.readCookie(r, cookieToken, &tok) {
		if err := s.consul.Logout(ctx, tok.SecretID); err != nil {
			s.log.Debug("consul logout failed", "accessor", tok.AccessorID, "error", err)
		}
	}
}

// ---------------------------------------------------------------- proxy

func (s *Server) handleProxy(w http.ResponseWriter, r *http.Request) {
	if r.Header.Get("Authorization") != "" {
		s.proxyBearer(w, r)
		return
	}
	st := s.ensure(w, r)
	switch st.outcome {
	case outcomeOK:
	case outcomeNoSession, outcomeExpired:
		if isNavigation(r) {
			http.Redirect(w, r, "/auth/login?return_to="+url.QueryEscape(r.URL.RequestURI()), http.StatusFound)
			return
		}
		unauthorized(w, "", "authentication required")
		return
	case outcomeDenied:
		writeError(w, http.StatusForbidden, "no Consul role is mapped to this user")
		return
	default:
		writeError(w, http.StatusServiceUnavailable, "IdP or Consul is not available")
		return
	}
	if !s.sameOrigin(r) {
		writeError(w, http.StatusForbidden, "cross-origin request rejected")
		return
	}
	s.forward(w, r.WithContext(context.WithValue(r.Context(), cookieAuthCtx{}, true)), st.tok.SecretID)
}

// proxyBearer serves API clients with Authorization: Bearer <JWT>. Nothing is stored: the Consul
// token lives only for the duration of the request.
func (s *Server) proxyBearer(w http.ResponseWriter, r *http.Request) {
	scheme, raw, _ := strings.Cut(r.Header.Get("Authorization"), " ")
	raw = strings.TrimSpace(raw)
	if !strings.EqualFold(scheme, "Bearer") || raw == "" {
		unauthorized(w, "invalid_request", "a Bearer token is required")
		return
	}
	id, err := s.idp.Verify(r.Context(), raw)
	if err != nil {
		s.log.Info("bearer token rejected", "error", err)
		unauthorized(w, "invalid_token", "the access token is invalid or expired")
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), callTimeout)
	ct, err := s.consul.Login(ctx, raw, map[string]string{"source": "consul-idp-proxy", "username": id.Username})
	cancel()
	if err != nil {
		if errors.Is(err, consul.ErrLoginDenied) {
			writeError(w, http.StatusForbidden, "no Consul role is mapped to this identity")
		} else {
			s.log.Warn("consul login failed", "error", err)
			writeError(w, http.StatusServiceUnavailable, "Consul login failed")
		}
		return
	}
	defer func() {
		lctx, cancel := context.WithTimeout(context.WithoutCancel(r.Context()), callTimeout)
		defer cancel()
		if err := s.consul.Logout(lctx, ct.SecretID); err != nil {
			s.log.Debug("consul logout failed", "accessor", ct.AccessorID, "error", err)
		}
	}()
	s.forward(w, r, ct.SecretID)
}

func (s *Server) forward(w http.ResponseWriter, r *http.Request, secretID string) {
	out := r.Clone(r.Context())
	// The client must not pick its own Consul token or leak IdP credentials upstream.
	out.Header.Del("Authorization")
	out.Header.Del("Cookie")
	out.Header.Del("X-Consul-Token")
	if q := out.URL.Query(); q.Has("token") {
		q.Del("token")
		out.URL.RawQuery = q.Encode()
	}
	out.Header.Set("X-Consul-Token", secretID)
	s.proxy.ServeHTTP(w, out)
}

func (s *Server) modifyResponse(resp *http.Response) error {
	resp.Header.Del("Set-Cookie")
	if resp.StatusCode != http.StatusForbidden {
		return nil
	}
	if v, _ := resp.Request.Context().Value(cookieAuthCtx{}).(bool); !v {
		return nil
	}
	head, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
	resp.Body = readCloser{io.MultiReader(bytes.NewReader(head), resp.Body), resp.Body}
	if bytes.Contains(head, []byte("ACL not found")) {
		// The token was deleted in Consul: drop the cookie, the next request logs in again.
		resp.Header.Add("Set-Cookie", s.expiredCookie(cookieToken, "/").String())
	}
	return nil
}

type readCloser struct {
	io.Reader
	io.Closer
}

// ---------------------------------------------------------------- cookies

func (s *Server) setCookie(w http.ResponseWriter, name, path string, v any, expires time.Time) error {
	val, err := s.seal.Seal(name, v, expires)
	if err != nil {
		return err
	}
	if len(val) > 3800 {
		s.log.Warn("cookie is close to the browser limit", "cookie", name, "bytes", len(val))
	}
	http.SetCookie(w, &http.Cookie{
		Name: name, Value: val, Path: path,
		MaxAge:   max(1, int(time.Until(expires).Seconds())),
		HttpOnly: true, Secure: s.cfg.CookieSecure,
		// Lax: the IdP redirect to /auth/callback is a cross-site top-level navigation.
		SameSite: http.SameSiteLaxMode,
	})
	return nil
}

func (s *Server) readCookie(r *http.Request, name string, v any) bool {
	c, err := r.Cookie(name)
	if err != nil || c.Value == "" {
		return false
	}
	return s.seal.Open(name, c.Value, v) == nil
}

func (s *Server) expiredCookie(name, path string) *http.Cookie {
	return &http.Cookie{Name: name, Value: "", Path: path, MaxAge: -1, HttpOnly: true,
		Secure: s.cfg.CookieSecure, SameSite: http.SameSiteLaxMode}
}

func (s *Server) clearCookie(w http.ResponseWriter, name, path string) {
	http.SetCookie(w, s.expiredCookie(name, path))
}

func (s *Server) clearSession(w http.ResponseWriter) {
	s.clearCookie(w, cookieSession, "/")
	s.clearCookie(w, cookieRefresh, "/")
	s.clearCookie(w, cookieToken, "/")
}

func hasCookie(r *http.Request, name string) bool {
	_, err := r.Cookie(name)
	return err == nil
}

// ---------------------------------------------------------------- helpers

// sameOrigin rejects state-changing cookie-authenticated requests from other origins (CSRF).
func (s *Server) sameOrigin(r *http.Request) bool {
	switch r.Method {
	case http.MethodGet, http.MethodHead, http.MethodOptions:
		return true
	}
	if o := r.Header.Get("Origin"); o != "" {
		return o == s.cfg.PublicOrigin()
	}
	return r.Header.Get("Sec-Fetch-Site") == "same-origin"
}

func isNavigation(r *http.Request) bool {
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		return false
	}
	if mode := r.Header.Get("Sec-Fetch-Mode"); mode != "" {
		return mode == "navigate"
	}
	return strings.Contains(r.Header.Get("Accept"), "text/html")
}

// safeReturnTo accepts only local paths, so the login cannot redirect to another site.
func safeReturnTo(p string) string {
	if p == "" || !strings.HasPrefix(p, "/") || strings.HasPrefix(p, "//") ||
		strings.ContainsAny(p, "\\\r\n") || strings.HasPrefix(p, "/auth/") {
		return defaultHome
	}
	if u, err := url.Parse(p); err != nil || u.IsAbs() || u.Host != "" {
		return defaultHome
	}
	return p
}

func randomString() string {
	b := make([]byte, 24)
	_, _ = rand.Read(b)
	return base64.RawURLEncoding.EncodeToString(b)
}

func unixOrNil(v int64) any {
	if v == 0 {
		return nil
	}
	return time.Unix(v, 0).UTC()
}

func unauthorized(w http.ResponseWriter, code, msg string) {
	v := `Bearer realm="consul"`
	if code != "" {
		v += `, error="` + code + `"`
	}
	w.Header().Set("WWW-Authenticate", v)
	writeError(w, http.StatusUnauthorized, msg)
}

func writeError(w http.ResponseWriter, status int, msg string) {
	h := w.Header()
	h.Set("Content-Type", "text/plain; charset=utf-8")
	h.Set("X-Content-Type-Options", "nosniff")
	h.Set("Cache-Control", "no-store")
	w.WriteHeader(status)
	_, _ = io.WriteString(w, msg+"\n")
}

func writeHTML(w http.ResponseWriter, status int, msg string, loginLink bool) {
	h := w.Header()
	h.Set("Content-Type", "text/html; charset=utf-8")
	h.Set("Cache-Control", "no-store")
	h.Set("Content-Security-Policy", "default-src 'none'")
	w.WriteHeader(status)
	link := ""
	if loginLink {
		link = `<p><a href="/auth/login">Log in</a></p>`
	}
	_, _ = fmt.Fprintf(w, "<!doctype html><title>Consul</title><p>%s</p>%s\n", html.EscapeString(msg), link)
}
