package api

import (
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/yegors/co-atc/internal/auth"
)

// Every sign-in, refused or not, and every sign-out is traced with the
// account, the browser's address as the proxy gives it, the proxy, the
// browser and the time; the owner reads the last ones, the newest first.
// A name that is no account is not written down: it may be a password typed
// in the wrong field.
func TestTheOwnerReadsWhoSignedInFromWhere(t *testing.T) {
	s := publicStack(t, stackOptions{})
	agent := func(a string) func(*http.Request) { return func(r *http.Request) { r.Header.Set("User-Agent", a) } }
	start := time.Now().Add(-time.Second)

	s.do("POST", "/api/v1/auth/login", `{"name":"`+testUser+`","password":"wrong"}`, throughTheProxy("203.0.113.5"), agent("Guesser/1.0"))
	s.do("POST", "/api/v1/auth/login", `{"name":"my secret password","password":"x"}`, throughTheProxy("203.0.113.5"), agent("Guesser/1.0"))
	rec := s.do("POST", "/api/v1/auth/login", `{"name":"`+viewerUser+`","password":"`+testPassword+`"}`,
		throughTheProxy("203.0.113.6"), agent("Friend's Safari"))
	s.cookie = rec.Result().Cookies()[0]
	s.do("POST", "/api/v1/auth/logout", "", throughTheProxy("203.0.113.6"), agent("Friend's Safari"))

	s.signInAs(testUser) // from 192.0.2.1 directly: the owner on the network
	rec = s.do("GET", "/api/v1/signins", "")
	if rec.Code != http.StatusOK {
		t.Fatalf("the owner reads the trace: %d %s", rec.Code, rec.Body.String())
	}
	if strings.Contains(rec.Body.String(), "my secret password") {
		t.Error("a name that is no account was written down")
	}
	var out struct {
		SignIns []auth.SignInEvent `json:"signins"`
	}
	decode(t, rec, &out)
	want := []auth.SignInEvent{
		{Event: auth.EventSignedIn, User: testUser, From: proxyAddr},
		{Event: auth.EventSignedOut, User: viewerUser, From: "203.0.113.6", Via: proxyAddr, Agent: "Friend's Safari"},
		{Event: auth.EventSignedIn, User: viewerUser, From: "203.0.113.6", Via: proxyAddr, Agent: "Friend's Safari"},
		{Event: auth.EventRefused, Reason: auth.ReasonUnknownName, From: "203.0.113.5", Via: proxyAddr, Agent: "Guesser/1.0"},
		{Event: auth.EventRefused, User: testUser, Reason: auth.ReasonWrongPassword, From: "203.0.113.5", Via: proxyAddr, Agent: "Guesser/1.0"},
	}
	if len(out.SignIns) != len(want) {
		t.Fatalf("%d events, want %d: %+v", len(out.SignIns), len(want), out.SignIns)
	}
	for i, w := range want {
		got := out.SignIns[i]
		if got.At.Before(start) || got.At.After(time.Now()) {
			t.Errorf("event %d dated %v", i, got.At)
		}
		got.At = time.Time{}
		if got != w {
			t.Errorf("event %d: %+v, want %+v", i, got, w)
		}
	}

	// Kept beside the accounts, for the next start.
	if _, err := os.Stat(filepath.Join(s.dir, auth.TraceFile)); err != nil {
		t.Errorf("the trace file: %v", err)
	}
}

// Nothing of the trace reaches a viewer: the route is refused, and the
// routes a viewer reads say nothing of who else signed in or from where.
func TestAViewerSeesNothingOfTheTrace(t *testing.T) {
	s := publicStack(t, stackOptions{})
	s.do("POST", "/api/v1/auth/login", `{"name":"`+testUser+`","password":"wrong"}`, throughTheProxy("203.0.113.5"))
	s.signInAs(testUser)

	s.cookie = nil
	if rec := s.do("GET", "/api/v1/signins", "", throughTheProxy("203.0.113.6")); rec.Code != http.StatusUnauthorized {
		t.Errorf("anyone: %d, want 401", rec.Code)
	}
	s.signInAs(viewerUser)
	if rec := s.do("GET", "/api/v1/signins", "", throughTheProxy("203.0.113.6")); rec.Code != http.StatusForbidden {
		t.Errorf("a viewer: %d, want 403", rec.Code)
	}
	if rec := s.do("GET", "/api/v1/signins?limit=500", "", throughTheProxy("203.0.113.6")); rec.Code != http.StatusForbidden {
		t.Errorf("a viewer, with a limit: %d, want 403", rec.Code)
	}
	for _, path := range []string{"/api/v1/server", "/api/v1/auth/status", "/api/v1/status", "/api/v1/config"} {
		body := s.do("GET", path, "", throughTheProxy("203.0.113.6")).Body.String()
		if strings.Contains(body, "203.0.113.5") || strings.Contains(body, "signins") || strings.Contains(body, auth.TraceFile) {
			t.Errorf("a viewer's %s tells of the trace: %s", path, body)
		}
	}
}
