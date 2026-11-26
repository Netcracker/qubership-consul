package proxy

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestWriteHTMLSignedOut(t *testing.T) {
	rec := httptest.NewRecorder()
	writeHTML(rec, http.StatusOK, "bye", true)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d", rec.Code)
	}
	body := rec.Body.String()
	for _, want := range []string{"You are signed out", `href="/auth/login"`, "Log in", "bye", `class="icon ok"`} {
		if !strings.Contains(body, want) {
			t.Errorf("body does not contain %q", want)
		}
	}
	csp := rec.Header().Get("Content-Security-Policy")
	if !strings.Contains(csp, "style-src 'sha256-") || strings.Contains(csp, "unsafe-inline") {
		t.Errorf("unexpected CSP %q", csp)
	}
	if rec.Header().Get("Cache-Control") != "no-store" {
		t.Error("the page must not be cached")
	}
}

func TestWriteHTMLErrors(t *testing.T) {
	cases := []struct {
		status int
		title  string
	}{
		{http.StatusForbidden, "Access denied"},
		{http.StatusUnauthorized, "Sign-in failed"},
		{http.StatusBadGateway, "Sign-in failed"},
	}
	for _, tc := range cases {
		rec := httptest.NewRecorder()
		writeHTML(rec, tc.status, "msg", false)
		if rec.Code != tc.status {
			t.Errorf("status = %d, want %d", rec.Code, tc.status)
		}
		body := rec.Body.String()
		if !strings.Contains(body, tc.title) || !strings.Contains(body, `class="icon err"`) {
			t.Errorf("status %d: unexpected body %q", tc.status, body)
		}
		if strings.Contains(body, "/auth/login") {
			t.Errorf("status %d: the login button must be absent", tc.status)
		}
	}
}

func TestWriteHTMLEscapesMessage(t *testing.T) {
	rec := httptest.NewRecorder()
	writeHTML(rec, http.StatusForbidden, `<script>alert(1)</script>`, true)
	body := rec.Body.String()
	if strings.Contains(body, "<script>") {
		t.Fatalf("the message is not escaped: %q", body)
	}
}
