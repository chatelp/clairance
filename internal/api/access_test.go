package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"github.com/yegors/co-atc/internal/auth"
	"github.com/yegors/co-atc/internal/config"
	"github.com/yegors/co-atc/pkg/logger"
)

// server starts the real routes over the accounts file in dir, with only what
// the first-run page and the access switch touch. Calling it twice on one dir
// is a restart.
type server struct {
	t      *testing.T
	routes http.Handler
	cookie *http.Cookie
}

func startServer(t *testing.T, dir, host string, proxies ...string) *server {
	t.Helper()
	log, err := logger.New(logger.Config{Level: "error", Format: "console"})
	if err != nil {
		t.Fatal(err)
	}
	cfg := &config.Config{ConfigPath: filepath.Join(dir, "config.toml")}
	cfg.Server.Host = host
	cfg.Server.TrustedProxies = proxies

	f, err := auth.LoadUserFile(cfg.ConfigPath)
	if err != nil {
		t.Fatal(err)
	}
	svc, err := auth.NewService(auth.Config{
		UserFile: f, Loopback: isLoopback(host), TrustedProxies: proxies,
	})
	if err != nil {
		t.Fatal(err)
	}
	h := &Handler{auth: svc, config: cfg, logger: log}
	r := &Router{handler: h, middleware: NewMiddleware(log), config: cfg, logger: log}
	return &server{t: t, routes: r.Routes()}
}

func (s *server) do(method, path, body string) *httptest.ResponseRecorder {
	s.t.Helper()
	req := httptest.NewRequest(method, path, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	if s.cookie != nil {
		req.AddCookie(s.cookie)
	}
	rec := httptest.NewRecorder()
	s.routes.ServeHTTP(rec, req)
	return rec
}

func (s *server) setupNeeded() bool {
	s.t.Helper()
	rec := s.do("GET", "/api/v1/setup/status", "")
	var st SetupState
	if err := json.NewDecoder(rec.Body).Decode(&st); err != nil {
		s.t.Fatalf("setup status: %v", err)
	}
	return st.Needed
}

func (s *server) signIn(name, password string) {
	s.t.Helper()
	rec := s.do("POST", "/api/v1/auth/login", `{"name":"`+name+`","password":"`+password+`"}`)
	for _, c := range rec.Result().Cookies() {
		if c.Name == auth.CookieName {
			s.cookie = c
			return
		}
	}
	s.t.Fatalf("sign-in failed: %d %s", rec.Code, rec.Body.String())
}

// locked says whether the data routes turn away a request without a session.
// The probe is the access switch itself with an empty body: past the lock it
// answers 400, in front of it 401.
func (s *server) locked() bool {
	s.t.Helper()
	saved := s.cookie
	s.cookie = nil
	defer func() { s.cookie = saved }()
	switch code := s.do("PUT", "/api/v1/access", `{}`).Code; code {
	case http.StatusUnauthorized:
		return true
	case http.StatusBadRequest:
		return false
	default:
		s.t.Fatalf("probe answered %d", code)
		return false
	}
}

// The bug, as the browser met it: "This machine only" on the first-run page,
// the page reloads, asks again -- forever.
func TestFirstRunLocalIsNotAskedAgain(t *testing.T) {
	dir := t.TempDir()
	s := startServer(t, dir, "127.0.0.1")
	if !s.setupNeeded() {
		t.Fatal("precondition: a fresh server asks the first-run question")
	}
	if rec := s.do("POST", "/api/v1/setup", `{"mode":"local"}`); rec.Code != http.StatusOK {
		t.Fatalf("choosing local: %d %s", rec.Code, rec.Body.String())
	}
	if s.setupNeeded() {
		t.Error("after choosing local the page asks again: the choice was not kept")
	}
	if startServer(t, dir, "127.0.0.1").setupNeeded() {
		t.Error("after a restart the page asks again: the choice was not recorded")
	}
	if s.locked() {
		t.Error("local-only, and the data asks for a session")
	}
}

// Both ways from the settings panel, and what each way requires.
func TestThePanelSwitchesAccessBothWays(t *testing.T) {
	dir := t.TempDir()
	s := startServer(t, dir, "127.0.0.1")
	if rec := s.do("POST", "/api/v1/setup", `{"mode":"account","username":"pierre","password":"correcthorsebattery"}`); rec.Code != http.StatusOK {
		t.Fatalf("creating the account: %d %s", rec.Code, rec.Body.String())
	}

	// Turning sign-in off takes a session.
	if rec := s.do("PUT", "/api/v1/access", `{"mode":"local"}`); rec.Code != http.StatusUnauthorized {
		t.Fatalf("switching to local without a session: got %d, want 401", rec.Code)
	}
	s.signIn("pierre", "correcthorsebattery")
	rec := s.do("PUT", "/api/v1/access", `{"mode":"local"}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("switching to local: %d %s", rec.Code, rec.Body.String())
	}
	var st auth.AccessState
	json.NewDecoder(rec.Body).Decode(&st)
	if st.Mode != auth.AccessLocal || len(st.Accounts) != 1 {
		t.Errorf("after switching to local: %+v, want mode local with the account kept", st)
	}
	if s.locked() || startServer(t, dir, "127.0.0.1").locked() {
		t.Error("local-only, and the data asks for a session (now or after a restart)")
	}

	// Turning it back on takes none: whoever reaches a local-only server is on
	// the machine already.
	s.cookie = nil
	if rec := s.do("PUT", "/api/v1/access", `{"mode":"account"}`); rec.Code != http.StatusOK {
		t.Fatalf("switching back to an account: %d %s", rec.Code, rec.Body.String())
	}
	if !s.locked() || !startServer(t, dir, "127.0.0.1").locked() {
		t.Error("sign-in chosen again, and the data is open (now or after a restart)")
	}
	s.signIn("pierre", "correcthorsebattery") // the kept account, its password unchanged
}

// Found by the owner on the first try: signed in, switched to local, switched
// back -- and no sign-in form. The session from before the switch was still
// valid, so requiring sign-in again asked nobody to sign in.
func TestRequiringSignInAgainAsksEveryoneToSignIn(t *testing.T) {
	dir := t.TempDir()
	s := startServer(t, dir, "127.0.0.1")
	s.do("POST", "/api/v1/setup", `{"mode":"account","username":"pierre","password":"correcthorsebattery"}`)
	s.signIn("pierre", "correcthorsebattery")
	if rec := s.do("PUT", "/api/v1/access", `{"mode":"local"}`); rec.Code != http.StatusOK {
		t.Fatalf("switching to local: %d %s", rec.Code, rec.Body.String())
	}
	if rec := s.do("PUT", "/api/v1/access", `{"mode":"account"}`); rec.Code != http.StatusOK {
		t.Fatalf("switching back: %d %s", rec.Code, rec.Body.String())
	}

	// The browser still sends the cookie it had before the switch.
	if rec := s.do("PUT", "/api/v1/access", `{}`); rec.Code != http.StatusUnauthorized {
		t.Errorf("the session from before local-only still lets requests through (%d): "+
			"sign-in is required again and nobody is asked to sign in", rec.Code)
	}
	var st struct{ Authenticated bool }
	json.NewDecoder(s.do("GET", "/api/v1/auth/status", "").Body).Decode(&st)
	if st.Authenticated {
		t.Error("auth/status says authenticated, so the page shows no sign-in form")
	}
	s.signIn("pierre", "correcthorsebattery") // and signing in again works
}

// From local-only with no account, the panel creates the first one.
func TestThePanelCreatesTheFirstAccountFromLocal(t *testing.T) {
	dir := t.TempDir()
	s := startServer(t, dir, "127.0.0.1")
	s.do("POST", "/api/v1/setup", `{"mode":"local"}`)

	if rec := s.do("PUT", "/api/v1/access", `{"mode":"account"}`); rec.Code != http.StatusConflict {
		t.Errorf("an account required with nobody to sign in as: got %d, want 409", rec.Code)
	}
	if rec := s.do("PUT", "/api/v1/access", `{"mode":"account","username":"pierre","password":"court"}`); rec.Code != http.StatusBadRequest {
		t.Errorf("a short password: got %d, want 400", rec.Code)
	}
	if s.locked() {
		t.Fatal("a refused account locked the server anyway")
	}
	if rec := s.do("PUT", "/api/v1/access", `{"mode":"account","username":"pierre","password":"correcthorsebattery"}`); rec.Code != http.StatusOK {
		t.Fatalf("creating the account: %d %s", rec.Code, rec.Body.String())
	}
	if !startServer(t, dir, "127.0.0.1").locked() {
		t.Error("after a restart the data is open: local-only still recorded beside the account")
	}
}

// Where others can reach the server, neither page may turn sign-in off.
func TestLocalIsRefusedWhereOthersCanReachTheServer(t *testing.T) {
	// Behind a declared proxy, the first-run page must not offer an open server.
	proxied := startServer(t, t.TempDir(), "127.0.0.1", "192.0.2.10")
	if rec := proxied.do("POST", "/api/v1/setup", `{"mode":"local"}`); rec.Code != http.StatusConflict {
		t.Errorf("first-run local behind a proxy: got %d, want 409", rec.Code)
	}
	if !proxied.setupNeeded() {
		t.Error("the refused answer counted as an answer")
	}

	// Bound to every interface, with an account, the panel must not remove it.
	dir := t.TempDir()
	s := startServer(t, dir, "0.0.0.0")
	s.do("POST", "/api/v1/setup", `{"mode":"account","username":"pierre","password":"correcthorsebattery"}`)
	s.signIn("pierre", "correcthorsebattery")
	if rec := s.do("PUT", "/api/v1/access", `{"mode":"local"}`); rec.Code != http.StatusConflict {
		t.Errorf("switching to local on 0.0.0.0: got %d, want 409", rec.Code)
	}
	if !s.locked() || !startServer(t, dir, "0.0.0.0").locked() {
		t.Error("the refusal left the data open")
	}
}

// The first-run page answers nobody who comes through a proxy, declared or
// not: on a server bound to 127.0.0.1 with a proxy on the same machine, the
// first visitor from the Internet would otherwise have created the owner's
// account -- or, the proxy undeclared, chosen to run without sign-in. From
// the machine itself it answers as before.
func TestTheFirstRunPageAnswersNobodyThroughAProxy(t *testing.T) {
	post := func(s *server, body string, forwarded bool) int {
		req := httptest.NewRequest("POST", "/api/v1/setup", strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		req.RemoteAddr, req.Host = "127.0.0.1:50000", "127.0.0.1:8000"
		if forwarded {
			req.Header.Set("X-Forwarded-For", "203.0.113.9")
			req.Header.Set("X-Forwarded-Proto", "https")
		}
		rec := httptest.NewRecorder()
		s.routes.ServeHTTP(rec, req)
		return rec.Code
	}
	account := `{"mode":"account","username":"intruder","password":"0123456789abc"}`

	for _, proxies := range [][]string{{"127.0.0.1"}, nil} { // declared, and not
		s := startServer(t, t.TempDir(), "127.0.0.1", proxies...)
		for _, body := range []string{`{"mode":"local"}`, account} {
			if code := post(s, body, true); code != http.StatusForbidden {
				t.Errorf("proxies %v, %s through the proxy: %d, want 403", proxies, body, code)
			}
		}
		if !s.setupNeeded() {
			t.Errorf("proxies %v: an answer through the proxy was taken", proxies)
		}
		if code := post(s, `{"mode":"account","username":"pierre","password":"correcthorsebattery"}`, false); code != http.StatusOK {
			t.Errorf("proxies %v, from the machine itself: %d, want 200", proxies, code)
		}
	}
}
