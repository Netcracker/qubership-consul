// Package config reads the proxy configuration from environment variables.
package config

import (
	"errors"
	"fmt"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"
)

// Config is the proxy configuration.
type Config struct {
	ListenAddr string
	// PublicURL is the external URL of the proxy as the browser sees it (no path).
	PublicURL *url.URL

	ConsulAddr       *url.URL
	ConsulCAFile     string
	ConsulAuthMethod string

	// OIDCIssuer is the external issuer; it must equal the iss claim of the IdP tokens.
	OIDCIssuer string
	// OIDCInternalURL is an optional in-cluster base URL of the same realm for server-side calls.
	OIDCInternalURL  string
	OIDCClientID     string
	OIDCClientSecret string
	OIDCScopes       []string
	// OIDCAudiences, when not empty, must intersect the aud claim.
	OIDCAudiences []string
	OIDCCAFile    string
	// OIDCInsecureSkipVerify disables TLS verification towards the IdP (dev stands only).
	OIDCInsecureSkipVerify bool
	UsernameClaim          string
	ClockSkew              time.Duration

	// CookieSecret is the key material for cookie encryption; all replicas must share it.
	CookieSecret []byte
	// CookieSecretGenerated is true when no secret was configured and a random one is used.
	CookieSecretGenerated bool
	CookieSecure          bool

	// RefreshTokenLoginBefore: send the user to the IdP login form when the refresh token expires sooner.
	RefreshTokenLoginBefore time.Duration
	// ConsulTokenRefreshBefore: get a new Consul token when the current one expires sooner.
	ConsulTokenRefreshBefore time.Duration
	// SessionMaxAge caps the browser session regardless of the IdP refresh token lifetime.
	SessionMaxAge time.Duration
	// UpstreamTimeout is the response header timeout towards Consul (must exceed blocking query wait).
	UpstreamTimeout time.Duration
	LogLevel        string
}

// Load reads the configuration. getenv is os.Getenv in production.
func Load(getenv func(string) string) (*Config, error) {
	var errs []error
	get := func(key, def string) string {
		if v := strings.TrimSpace(getenv(key)); v != "" {
			return v
		}
		return def
	}
	required := func(key string) string {
		v := get(key, "")
		if v == "" {
			errs = append(errs, fmt.Errorf("%s is required", key))
		}
		return v
	}
	absURL := func(key, raw string) *url.URL {
		if raw == "" {
			return nil
		}
		u, err := url.Parse(raw)
		if err != nil || u.Scheme == "" || u.Host == "" {
			errs = append(errs, fmt.Errorf("%s must be an absolute URL, got %q", key, raw))
			return nil
		}
		return u
	}
	duration := func(key string, def time.Duration) time.Duration {
		raw := get(key, "")
		if raw == "" {
			return def
		}
		d, err := time.ParseDuration(raw)
		if err != nil || d < 0 {
			errs = append(errs, fmt.Errorf("%s must be a non-negative duration, got %q", key, raw))
			return def
		}
		return d
	}
	boolean := func(key string, def bool) bool {
		raw := get(key, "")
		if raw == "" {
			return def
		}
		b, err := strconv.ParseBool(raw)
		if err != nil {
			errs = append(errs, fmt.Errorf("%s must be a boolean, got %q", key, raw))
			return def
		}
		return b
	}
	secret := func(key string) string {
		v := get(key, "")
		if file := get(key+"_FILE", ""); file != "" {
			b, err := os.ReadFile(file)
			if err != nil {
				errs = append(errs, fmt.Errorf("cannot read %s_FILE: %w", key, err))
			}
			v = strings.TrimSpace(string(b))
		}
		return v
	}

	cfg := &Config{
		ListenAddr:               get("LISTEN_ADDR", ":8080"),
		ConsulCAFile:             get("CONSUL_CA_FILE", ""),
		ConsulAuthMethod:         get("CONSUL_AUTH_METHOD", "platform-idp"),
		OIDCIssuer:               strings.TrimSuffix(required("OIDC_ISSUER"), "/"),
		OIDCInternalURL:          strings.TrimSuffix(get("OIDC_INTERNAL_URL", ""), "/"),
		OIDCClientID:             required("OIDC_CLIENT_ID"),
		OIDCClientSecret:         secret("OIDC_CLIENT_SECRET"),
		OIDCScopes:               splitList(get("OIDC_SCOPES", "openid profile email")),
		OIDCAudiences:            splitList(get("OIDC_AUDIENCES", "")),
		OIDCCAFile:               get("OIDC_CA_FILE", ""),
		OIDCInsecureSkipVerify:   boolean("OIDC_INSECURE_SKIP_VERIFY", false),
		UsernameClaim:            get("USERNAME_CLAIM", "preferred_username"),
		ClockSkew:                duration("CLOCK_SKEW", 30*time.Second),
		RefreshTokenLoginBefore:  duration("REFRESH_TOKEN_LOGIN_BEFORE", time.Minute),
		ConsulTokenRefreshBefore: duration("CONSUL_TOKEN_REFRESH_BEFORE", 2*time.Minute),
		SessionMaxAge:            duration("SESSION_MAX_AGE", 12*time.Hour),
		UpstreamTimeout:          duration("UPSTREAM_TIMEOUT", 11*time.Minute),
		LogLevel:                 strings.ToLower(get("LOG_LEVEL", "info")),
	}
	cfg.PublicURL = absURL("PUBLIC_URL", required("PUBLIC_URL"))
	if cfg.PublicURL != nil && strings.Trim(cfg.PublicURL.Path, "/") != "" {
		errs = append(errs, fmt.Errorf("PUBLIC_URL must not contain a path, got %q", cfg.PublicURL.Path))
	}
	cfg.ConsulAddr = absURL("CONSUL_ADDR", get("CONSUL_ADDR", "http://localhost:8500"))
	absURL("OIDC_ISSUER", cfg.OIDCIssuer)
	absURL("OIDC_INTERNAL_URL", cfg.OIDCInternalURL)

	if s := secret("COOKIE_SECRET"); s != "" {
		if len(s) < 32 {
			errs = append(errs, errors.New("COOKIE_SECRET must be at least 32 characters"))
		}
		cfg.CookieSecret = []byte(s)
	} else {
		cfg.CookieSecretGenerated = true
	}
	cfg.CookieSecure = boolean("COOKIE_SECURE", cfg.PublicURL != nil && cfg.PublicURL.Scheme == "https")

	if err := errors.Join(errs...); err != nil {
		return nil, err
	}
	return cfg, nil
}

// PublicOrigin returns scheme://host of the public URL.
func (c *Config) PublicOrigin() string {
	return c.PublicURL.Scheme + "://" + c.PublicURL.Host
}

// PublicPath returns the public URL joined with an absolute path.
func (c *Config) PublicPath(p string) string {
	return c.PublicOrigin() + p
}

func splitList(s string) []string {
	return strings.FieldsFunc(s, func(r rune) bool { return r == ',' || r == ' ' })
}
