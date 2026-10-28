package auth

import (
	"encoding/base64"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestPasswordRoundTrip(t *testing.T) {
	h, err := HashPassword("un mot de passe correct")
	if err != nil {
		t.Fatal(err)
	}
	if !VerifyPassword("un mot de passe correct", h) {
		t.Error("the right password was refused")
	}
	if VerifyPassword("un mot de passe incorrect", h) {
		t.Error("the wrong password was accepted")
	}
}

// Two hashes of the same password must differ: a salt that repeated would let one
// stolen file show which accounts share a password.
func TestHashesAreSalted(t *testing.T) {
	a, _ := HashPassword("le meme mot de passe")
	b, _ := HashPassword("le meme mot de passe")
	if a == b {
		t.Error("two hashes of one password are identical: the salt is not random")
	}
}

func TestShortPasswordRefused(t *testing.T) {
	if _, err := HashPassword("court"); err == nil {
		t.Error("a five-character password was accepted")
	}
}

// A malformed hash must be refused, not crash and not accidentally match.
func TestGarbageHashRefused(t *testing.T) {
	for _, bad := range []string{"", "$argon2id$", "not a hash", "$argon2id$v=19$m=x,t=y,p=z$AA$BB"} {
		if VerifyPassword("anything", bad) {
			t.Errorf("garbage hash %q was accepted", bad)
		}
	}
}

func TestSessionLifecycle(t *testing.T) {
	s := NewStore(time.Hour)
	sess, err := s.Create("pierre", RoleOwner)
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := s.Lookup(sess.Token); !ok {
		t.Fatal("a fresh session was not found")
	}
	s.Revoke(sess.Token)
	if _, ok := s.Lookup(sess.Token); ok {
		t.Error("a revoked session is still valid")
	}
}

func TestExpiredSessionRefused(t *testing.T) {
	s := NewStore(time.Millisecond)
	sess, _ := s.Create("pierre", RoleOwner)
	time.Sleep(5 * time.Millisecond)
	if _, ok := s.Lookup(sess.Token); ok {
		t.Error("an expired session is still valid")
	}
}

// The trap this guards: a forwarded header believed from anyone lets any client
// declare itself already on HTTPS, or wear someone else's address.
func TestForwardedHeadersIgnoredFromUntrustedSource(t *testing.T) {
	p, _ := NewTrustedProxies(nil)
	r := httptest.NewRequest("GET", "/", nil)
	r.RemoteAddr = "192.168.1.99:5000"
	r.Header.Set("X-Forwarded-Proto", "https")
	r.Header.Set("X-Forwarded-For", "10.0.0.1")

	if got := p.Scheme(r); got != "http" {
		t.Errorf("scheme = %q from an untrusted source, want http", got)
	}
	if got := p.ClientIP(r); got != "192.168.1.99" {
		t.Errorf("client IP = %q from an untrusted source, want the real peer", got)
	}
}

func TestForwardedHeadersHonouredFromTrustedProxy(t *testing.T) {
	p, err := NewTrustedProxies([]string{"172.18.0.0/16"})
	if err != nil {
		t.Fatal(err)
	}
	r := httptest.NewRequest("GET", "/", nil)
	r.RemoteAddr = "172.18.0.4:5000"
	r.Header.Set("X-Forwarded-Proto", "https")
	r.Header.Set("X-Forwarded-For", "10.0.0.1")

	if got := p.Scheme(r); got != "https" {
		t.Errorf("scheme = %q from a trusted proxy, want https", got)
	}
	if got := p.ClientIP(r); got != "10.0.0.1" {
		t.Errorf("client IP = %q from a trusted proxy, want the forwarded one", got)
	}
}

func TestLimiterStopsGuessing(t *testing.T) {
	l := NewLimiter(3, time.Minute)
	for i := 0; i < 3; i++ {
		if !l.Allow("1.2.3.4") {
			t.Fatalf("attempt %d refused too early", i+1)
		}
	}
	if l.Allow("1.2.3.4") {
		t.Error("a fourth attempt was allowed")
	}
	if !l.Allow("5.6.7.8") {
		t.Error("another address was caught by the first one's limit")
	}
	l.Succeeded("1.2.3.4")
	if !l.Allow("1.2.3.4") {
		t.Error("a correct password did not clear the count")
	}
}

func TestDisabledServiceLetsEverythingThrough(t *testing.T) {
	s, err := NewService(Config{Enabled: false})
	if err != nil {
		t.Fatal(err)
	}
	reached := false
	h := s.Require(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { reached = true }))
	h.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest("GET", "/api/v1/aircraft", nil))
	if !reached {
		t.Error("authentication is disabled but the request was blocked")
	}
}

func TestEnabledServiceBlocksAnonymous(t *testing.T) {
	hash, _ := HashPassword("un mot de passe correct")
	s, err := NewService(Config{Enabled: true, Users: []User{{Name: "pierre", PasswordHash: hash}}})
	if err != nil {
		t.Fatal(err)
	}
	rec := httptest.NewRecorder()
	s.Require(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		t.Error("an anonymous request reached the handler")
	})).ServeHTTP(rec, httptest.NewRequest("GET", "/api/v1/aircraft", nil))
	if rec.Code != http.StatusUnauthorized {
		t.Errorf("status = %d, want 401", rec.Code)
	}
}

// Enabling authentication without an account would lock the owner out of their own
// receiver, so it is refused at startup rather than at the first request.
func TestEnabledWithoutUsersIsRefused(t *testing.T) {
	if _, err := NewService(Config{Enabled: true}); err == nil {
		t.Error("auth.enabled with no user was accepted")
	}
}

func TestSignInIssuesASession(t *testing.T) {
	hash, _ := HashPassword("un mot de passe correct")
	s, _ := NewService(Config{Enabled: true, Users: []User{{Name: "pierre", PasswordHash: hash}}})
	r := httptest.NewRequest("POST", "/api/v1/auth/login", nil)
	r.RemoteAddr = "192.0.2.28:5000"

	if _, err := s.SignIn(r, "pierre", "un mauvais mot de passe"); err == nil {
		t.Error("a wrong password was accepted")
	}
	sess, err := s.SignIn(r, "pierre", "un mot de passe correct")
	if err != nil {
		t.Fatalf("the right password was refused: %v", err)
	}
	rec := httptest.NewRecorder()
	s.SetCookie(rec, r, sess)
	c := rec.Result().Cookies()[0]
	if !c.HttpOnly || c.SameSite != http.SameSiteStrictMode {
		t.Errorf("cookie is not HttpOnly+Strict: %+v", c)
	}
	if c.Secure {
		t.Error("cookie marked Secure on a plain HTTP request; the browser would never send it back")
	}
}

// The first-run path. Upstream ships with no authentication and a README saying
// never to expose it; this is what lets someone answer that instead of editing
// TOML, so it has to be exactly as careful as the command-line path.

func loginRequest() *http.Request {
	r := httptest.NewRequest("POST", "/api/v1/auth/login", nil)
	r.RemoteAddr = "127.0.0.1:5000"
	return r
}

func setupService(t *testing.T) *Service {
	t.Helper()
	f, err := LoadUserFile(filepath.Join(t.TempDir(), "config.toml"))
	if err != nil {
		t.Fatalf("LoadUserFile: %v", err)
	}
	s, err := NewService(Config{UserFile: f})
	if err != nil {
		t.Fatalf("NewService: %v", err)
	}
	return s
}

func TestSetupIsNeededUntilAnAccountExists(t *testing.T) {
	s := setupService(t)
	if !s.NeedsSetup() {
		t.Fatal("a server with no account anywhere needs setting up")
	}
	if s.Enabled() {
		t.Error("authentication cannot be on with no account")
	}

	if err := s.AddUser("pierre", "correcthorsebattery"); err != nil {
		t.Fatalf("AddUser: %v", err)
	}
	if s.NeedsSetup() {
		t.Error("setup should be done once an account exists")
	}
	if !s.Enabled() {
		t.Error("creating an account must turn authentication on, or it protects nothing")
	}
	if _, err := s.SignIn(loginRequest(), "pierre", "correcthorsebattery"); err != nil {
		t.Errorf("the account just created should be able to sign in: %v", err)
	}
}

// The page creates accounts without being signed in, so it has to stop being
// able to the moment there is someone to sign in as.
func TestSetupRefusesOnceAnAccountExists(t *testing.T) {
	s := setupService(t)
	if err := s.AddUser("pierre", "correcthorsebattery"); err != nil {
		t.Fatalf("first AddUser: %v", err)
	}
	if err := s.AddUser("intrus", "alsolongenough"); err == nil {
		t.Fatal("a second account through the setup path must be refused")
	}
	if _, err := s.SignIn(loginRequest(), "intrus", "alsolongenough"); err == nil {
		t.Error("the refused account must not exist")
	}
}

func TestSetupRefusesAShortPassword(t *testing.T) {
	s := setupService(t)
	if err := s.AddUser("pierre", "court"); err == nil {
		t.Fatal("a short password must be refused")
	}
	if !s.NeedsSetup() {
		t.Error("a refused account must leave the server still needing setup")
	}
}

// What is written must be the hash and nothing else. The password exists in
// memory for the moment it takes to hash it, and nowhere afterwards.
func TestTheAccountFileHoldsAHashAndNotThePassword(t *testing.T) {
	dir := t.TempDir()
	f, err := LoadUserFile(filepath.Join(dir, "config.toml"))
	if err != nil {
		t.Fatal(err)
	}
	s, err := NewService(Config{UserFile: f})
	if err != nil {
		t.Fatal(err)
	}
	const password = "correcthorsebattery"
	if err := s.AddUser("pierre", password); err != nil {
		t.Fatal(err)
	}

	raw, err := os.ReadFile(f.Path())
	if err != nil {
		t.Fatalf("the accounts file should exist at %s: %v", f.Path(), err)
	}
	if strings.Contains(string(raw), password) {
		t.Fatal("the password is in the file in clear")
	}
	if !strings.Contains(string(raw), "$argon2id$") {
		t.Error("the file should hold an Argon2id hash")
	}
	if info, err := os.Stat(f.Path()); err == nil && info.Mode().Perm() != 0o600 {
		t.Errorf("the accounts file holds password hashes: mode is %v, want 0600", info.Mode().Perm())
	}
}

// Accounts survive a restart, or the page would silently undo itself.
func TestAccountsAreReadBackAtTheNextStart(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "config.toml")

	f1, _ := LoadUserFile(cfgPath)
	s1, _ := NewService(Config{UserFile: f1})
	if err := s1.AddUser("pierre", "correcthorsebattery"); err != nil {
		t.Fatal(err)
	}

	f2, err := LoadUserFile(cfgPath)
	if err != nil {
		t.Fatalf("reload: %v", err)
	}
	s2, err := NewService(Config{Enabled: true, UserFile: f2})
	if err != nil {
		t.Fatalf("a restart should find the account: %v", err)
	}
	if s2.NeedsSetup() {
		t.Error("the account did not survive the restart")
	}
	if _, err := s2.SignIn(loginRequest(), "pierre", "correcthorsebattery"); err != nil {
		t.Errorf("the account should still sign in after a restart: %v", err)
	}
}

// A missing accounts file is the normal state before anyone sets anything up,
// not an error to fail on.
func TestAMissingAccountFileIsNotAnError(t *testing.T) {
	f, err := LoadUserFile(filepath.Join(t.TempDir(), "config.toml"))
	if err != nil {
		t.Fatalf("a missing accounts file must not be an error: %v", err)
	}
	if len(f.Users) != 0 {
		t.Errorf("expected no accounts, got %d", len(f.Users))
	}
}

// The bug this guards against, found by restarting a server an hour after
// writing the setup page: the account was in the file, the setup page was
// satisfied, and authentication was off -- because auth.enabled lives in a
// config.toml the page never writes. Configured-looking and unprotected is
// worse than either.
func TestAnAccountInTheFileTurnsAuthenticationOn(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "config.toml")

	f1, _ := LoadUserFile(cfgPath)
	s1, _ := NewService(Config{UserFile: f1}) // Enabled false, as a config without [auth]
	if err := s1.AddUser("pierre", "correcthorsebattery"); err != nil {
		t.Fatal(err)
	}

	// The restart: a fresh service from the same files, Enabled still false.
	f2, _ := LoadUserFile(cfgPath)
	s2, err := NewService(Config{Enabled: false, UserFile: f2})
	if err != nil {
		t.Fatalf("NewService: %v", err)
	}
	if !s2.Enabled() {
		t.Fatal("an account exists and authentication is off: the server is open")
	}
	if s2.NeedsSetup() {
		t.Error("setup should stay done")
	}
}

// A hand-written `enabled = false` beside hand-written users is a decision, not
// an oversight, and must be left alone.
func TestConfiguredUsersStillObeyEnabled(t *testing.T) {
	hash, _ := HashPassword("un mot de passe correct")
	s, err := NewService(Config{Enabled: false, Users: []User{{Name: "pierre", PasswordHash: hash}}})
	if err != nil {
		t.Fatalf("NewService: %v", err)
	}
	if s.Enabled() {
		t.Error("auth.enabled = false with users in the config must stay off")
	}
}

// Behind two trusted proxies -- a CDN, then Traefik -- the browser is the
// first address from the right that is not a trusted proxy's: not the CDN's,
// which every visitor would then share, and not whatever the browser wrote
// itself further left. A proxy that adds a header line of its own instead of
// appending is read the same way.
func TestTheBrowserIsTheFirstUntrustedAddressFromTheRight(t *testing.T) {
	p, err := NewTrustedProxies([]string{"192.0.2.1", "198.51.100.0/24"})
	if err != nil {
		t.Fatal(err)
	}
	for _, c := range []struct {
		lines []string
		want  string
	}{
		{[]string{"203.0.113.5"}, "203.0.113.5"},                         // one proxy
		{[]string{"203.0.113.5, 198.51.100.20"}, "203.0.113.5"},          // the CDN, then Traefik
		{[]string{"6.6.6.6, 203.0.113.5, 198.51.100.20"}, "203.0.113.5"}, // the browser's own invention
		{[]string{"6.6.6.6", "203.0.113.5"}, "203.0.113.5"},              // a second line, not appended
		{[]string{"198.51.100.7, 198.51.100.20"}, "198.51.100.7"},        // nothing but proxies: the farthest
		{[]string{"2001:db8::1, 198.51.100.20"}, "2001:db8::1"},          // IPv6
		{[]string{" 203.0.113.5 ,, 198.51.100.20 "}, "203.0.113.5"},      // spaces, an empty element
	} {
		r := httptest.NewRequest("GET", "/", nil)
		r.RemoteAddr = "192.0.2.1:5000"
		for _, l := range c.lines {
			r.Header.Add("X-Forwarded-For", l)
		}
		if got := p.ClientIP(r); got != c.want {
			t.Errorf("X-Forwarded-For %q: %q, want %q", c.lines, got, c.want)
		}
	}
}

// An unknown name costs what a wrong password does because both verify an
// Argon2id hash of the same parameters: the decoy's are HashPassword's.
// Raising the cost without the decoy would make an unknown name answer
// faster, and the form would tell which names exist (measured on 26/10 with
// the decoy as it is: 115 ms median for an unknown name, 116-120 ms for a
// wrong password, 21 tries each).
func TestTheDecoyHashCostsWhatARealOneDoes(t *testing.T) {
	real, err := HashPassword("a password of some length")
	if err != nil {
		t.Fatal(err)
	}
	params := func(h string) string {
		parts := strings.Split(h, "$")
		if len(parts) != 6 {
			t.Fatalf("not a PHC hash: %q", h)
		}
		salt, err1 := base64.RawStdEncoding.DecodeString(parts[4])
		key, err2 := base64.RawStdEncoding.DecodeString(parts[5])
		if err1 != nil || err2 != nil {
			t.Fatalf("undecodable hash %q", h)
		}
		return fmt.Sprintf("%s %s %s salt %d key %d", parts[1], parts[2], parts[3], len(salt), len(key))
	}
	if d, r := params(decoyHash), params(real); d != r {
		t.Errorf("the decoy is %s, a real hash %s", d, r)
	}
}
