// Command consul-idp-proxy logs users in at the platform IdP and forwards their requests to Consul
// with a Consul ACL token obtained through the JWT auth method. The proxy is stateless: the IdP
// session, the refresh token and the Consul token are kept in encrypted browser cookies.
package main

import (
	"context"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/Netcracker/qubership-consul/consul-idp-proxy/internal/config"
	"github.com/Netcracker/qubership-consul/consul-idp-proxy/internal/consul"
	"github.com/Netcracker/qubership-consul/consul-idp-proxy/internal/idp"
	"github.com/Netcracker/qubership-consul/consul-idp-proxy/internal/proxy"
	"github.com/Netcracker/qubership-consul/consul-idp-proxy/internal/seal"
)

func main() {
	if err := run(); err != nil {
		slog.Error("consul-idp-proxy failed", "error", err)
		os.Exit(1)
	}
}

func run() error {
	cfg, err := config.Load(os.Getenv)
	if err != nil {
		return err
	}
	log := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: logLevel(cfg.LogLevel)}))
	slog.SetDefault(log)

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	if cfg.CookieSecretGenerated {
		cfg.CookieSecret = make([]byte, 32)
		_, _ = rand.Read(cfg.CookieSecret)
		log.Warn("COOKIE_SECRET is not set: using a random key, sessions end on restart and do not work with several replicas")
	}
	sealer, err := seal.New(cfg.CookieSecret)
	if err != nil {
		return err
	}

	idpTransport, err := transport(cfg.OIDCCAFile, cfg.OIDCInsecureSkipVerify, 0)
	if err != nil {
		return fmt.Errorf("IdP TLS: %w", err)
	}
	if cfg.OIDCInsecureSkipVerify {
		log.Warn("TLS verification towards the IdP is disabled (OIDC_INSECURE_SKIP_VERIFY)")
	}
	var provider *idp.Client
	// The IdP may start later than the proxy: retry discovery for a while.
	for attempt := 1; ; attempt++ {
		provider, err = idp.New(ctx, idp.Options{
			Issuer:        cfg.OIDCIssuer,
			InternalURL:   cfg.OIDCInternalURL,
			ClientID:      cfg.OIDCClientID,
			ClientSecret:  cfg.OIDCClientSecret,
			Scopes:        cfg.OIDCScopes,
			Audiences:     cfg.OIDCAudiences,
			UsernameClaim: cfg.UsernameClaim,
			ClockSkew:     cfg.ClockSkew,
			RedirectURL:   cfg.PublicPath("/auth/callback"),
			HTTPClient:    &http.Client{Transport: idpTransport, Timeout: 30 * time.Second},
		})
		if err == nil {
			break
		}
		if attempt == 30 || ctx.Err() != nil {
			return fmt.Errorf("OIDC discovery: %w", err)
		}
		log.Warn("OIDC discovery failed, retrying", "error", err)
		time.Sleep(2 * time.Second)
	}

	consulTransport, err := transport(cfg.ConsulCAFile, false, cfg.UpstreamTimeout)
	if err != nil {
		return fmt.Errorf("Consul TLS: %w", err)
	}
	cc := consul.New(cfg.ConsulAddr, cfg.ConsulAuthMethod, &http.Client{Transport: consulTransport, Timeout: 30 * time.Second})

	srv := &http.Server{
		Addr:              cfg.ListenAddr,
		Handler:           proxy.New(cfg, provider, cc, sealer, consulTransport, log).Handler(),
		ReadHeaderTimeout: 10 * time.Second,
	}
	errc := make(chan error, 1)
	go func() {
		log.Info("listening", "addr", cfg.ListenAddr, "public_url", cfg.PublicURL.String(),
			"consul", cfg.ConsulAddr.String(), "issuer", cfg.OIDCIssuer)
		errc <- srv.ListenAndServe()
	}()
	select {
	case err := <-errc:
		if !errors.Is(err, http.ErrServerClosed) {
			return err
		}
	case <-ctx.Done():
		log.Info("shutting down")
		sctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()
		return srv.Shutdown(sctx)
	}
	return nil
}

func transport(caFile string, insecure bool, responseTimeout time.Duration) (*http.Transport, error) {
	t := http.DefaultTransport.(*http.Transport).Clone()
	t.ResponseHeaderTimeout = responseTimeout
	if caFile == "" && !insecure {
		return t, nil
	}
	tc := &tls.Config{MinVersion: tls.VersionTLS12, InsecureSkipVerify: insecure} //nolint:gosec // dev stands only
	if caFile != "" {
		pem, err := os.ReadFile(caFile)
		if err != nil {
			return nil, err
		}
		pool := x509.NewCertPool()
		if !pool.AppendCertsFromPEM(pem) {
			return nil, fmt.Errorf("no certificates in %s", caFile)
		}
		tc.RootCAs = pool
	}
	t.TLSClientConfig = tc
	return t, nil
}

func logLevel(s string) slog.Level {
	switch s {
	case "debug":
		return slog.LevelDebug
	case "warn":
		return slog.LevelWarn
	case "error":
		return slog.LevelError
	default:
		return slog.LevelInfo
	}
}
