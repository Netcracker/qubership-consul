package proxy

import (
	"crypto/sha256"
	"encoding/base64"
	"fmt"
	"html"
	"net/http"
)

// pageStyle is the only stylesheet of the service pages. It is allowed by its hash in the
// Content-Security-Policy, so the pages need neither 'unsafe-inline' nor external files.
const pageStyle = `
:root{--bg:#f4f5f8;--card:#fff;--text:#1f2430;--muted:#5d6577;--accent:#ca2171;--accent-hover:#a81a5c;--ok:#1a8f4d;--ok-bg:#e4f5ec;--err:#c0392b;--err-bg:#fbeaea;--shadow:0 10px 30px rgba(20,25,45,.10)}
@media (prefers-color-scheme:dark){:root{--bg:#14161c;--card:#1e222b;--text:#eceef3;--muted:#9aa2b5;--accent:#e04a8a;--accent-hover:#ec6aa0;--ok:#4cc38a;--ok-bg:#16322a;--err:#ff7b6b;--err-bg:#3a1f1e;--shadow:0 10px 30px rgba(0,0,0,.45)}}
*{box-sizing:border-box}
body{margin:0;min-height:100vh;display:flex;align-items:center;justify-content:center;padding:16px;background:var(--bg);color:var(--text);font:16px/1.5 system-ui,-apple-system,"Segoe UI",Roboto,Helvetica,Arial,sans-serif}
.card{width:100%;max-width:400px;padding:40px 32px 32px;text-align:center;background:var(--card);border-radius:16px;box-shadow:var(--shadow)}
.icon{display:inline-flex;align-items:center;justify-content:center;width:64px;height:64px;margin-bottom:20px;border-radius:50%}
.icon svg{width:32px;height:32px}
.icon.ok{background:var(--ok-bg);color:var(--ok)}
.icon.err{background:var(--err-bg);color:var(--err)}
h1{margin:0 0 8px;font-size:22px;font-weight:600}
p{margin:0 0 28px;color:var(--muted)}
.btn{display:block;padding:12px 24px;border-radius:10px;background:var(--accent);color:#fff;font-weight:600;text-decoration:none;transition:background .15s}
.btn:hover,.btn:focus-visible{background:var(--accent-hover)}
.btn:focus-visible{outline:3px solid var(--accent);outline-offset:2px}
.brand{margin-top:24px;font-size:13px;color:var(--muted)}
`

const (
	iconOK  = `<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2.4" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true"><path d="M5 12.5l4.5 4.5L19 7.5"/></svg>`
	iconErr = `<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2.4" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true"><path d="M12 7v6.5"/><path d="M12 17.2v.1"/></svg>`
)

// pageCSP allows only the stylesheet above.
var pageCSP = func() string {
	sum := sha256.Sum256([]byte(pageStyle))
	return "default-src 'none'; style-src 'sha256-" + base64.StdEncoding.EncodeToString(sum[:]) +
		"'; base-uri 'none'; form-action 'none'; frame-ancestors 'none'"
}()

// writeHTML writes a small status page. A 200 status is shown as a success (signed out),
// 403 as denied access and the other statuses as a failed sign-in.
func writeHTML(w http.ResponseWriter, status int, msg string, loginLink bool) {
	h := w.Header()
	h.Set("Content-Type", "text/html; charset=utf-8")
	h.Set("Cache-Control", "no-store")
	h.Set("X-Content-Type-Options", "nosniff")
	h.Set("Content-Security-Policy", pageCSP)
	w.WriteHeader(status)

	title, class, icon := "Sign-in failed", "err", iconErr
	switch status {
	case http.StatusOK:
		title, class, icon = "You are signed out", "ok", iconOK
	case http.StatusForbidden:
		title = "Access denied"
	}
	button := ""
	if loginLink {
		button = `<a class="btn" href="/auth/login">Log in</a>`
	}
	_, _ = fmt.Fprintf(w, `<!doctype html>
<html lang="en">
<head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width,initial-scale=1">
<meta name="color-scheme" content="light dark">
<title>%s · Consul</title>
<style>%s</style>
</head>
<body>
<main class="card">
<div class="icon %s">%s</div>
<h1>%s</h1>
<p>%s</p>
%s
<div class="brand">Consul</div>
</main>
</body>
</html>
`, html.EscapeString(title), pageStyle, class, icon, html.EscapeString(title), html.EscapeString(msg), button)
}
