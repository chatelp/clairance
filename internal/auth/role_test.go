package auth

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestARoleIsOwnerOrViewerAndNothingElse(t *testing.T) {
	for in, want := range map[string]string{"": RoleOwner, "owner": RoleOwner, " Viewer ": RoleViewer, "OWNER": RoleOwner} {
		if got, err := NormalizeRole(in); err != nil || got != want {
			t.Errorf("NormalizeRole(%q) = %q, %v; want %q", in, got, err, want)
		}
	}
	for _, bad := range []string{"veiwer", "admin", "read-only"} {
		if _, err := NormalizeRole(bad); err == nil {
			t.Errorf("NormalizeRole(%q) accepted", bad)
		}
	}
}

// requestAs is a request carrying a fresh session of the named account.
func requestAs(t *testing.T, s *Service, name string) *http.Request {
	t.Helper()
	sess, err := s.Sessions().Create(name, s.roles[name])
	if err != nil {
		t.Fatal(err)
	}
	r := httptest.NewRequest("GET", "/api/v1/aircraft", nil)
	r.AddCookie(&http.Cookie{Name: CookieName, Value: sess.Token})
	return r
}

func writeUsers(t *testing.T, dir, users string) {
	t.Helper()
	raw := `{"access":"account","users":[` + users + `]}`
	if err := os.WriteFile(filepath.Join(dir, DefaultUserFile), []byte(raw), 0o600); err != nil {
		t.Fatal(err)
	}
}

// The owner's users.json, written before roles existed, has no "role": its
// account is an owner. A viewer is what the file says.
func TestAnAccountWithoutRoleIsAnOwner(t *testing.T) {
	dir := t.TempDir()
	writeUsers(t, dir, `{"name":"pierre","password_hash":"`+decoyHash+`","created_at":"2026-10-01T00:00:00Z"},
		{"name":"guest","password_hash":"`+decoyHash+`","created_at":"2026-10-13T00:00:00Z","role":"viewer"}`)
	s := startIn(t, dir, false)
	if !s.Enabled() {
		t.Fatal("accounts in the file turn sign-in on")
	}
	if got := s.Role(requestAs(t, s, "pierre")); got != RoleOwner {
		t.Errorf("an account without role: %q, want owner", got)
	}
	if got := s.Role(requestAs(t, s, "guest")); got != RoleViewer {
		t.Errorf("an account with role viewer: %q, want viewer", got)
	}
	if got := s.Role(httptest.NewRequest("GET", "/", nil)); got != "" {
		t.Errorf("no session: %q, want none", got)
	}
}

// A role nobody knows stops the server rather than being guessed, with the
// account named.
func TestAnUnknownRoleIsRefusedByName(t *testing.T) {
	dir := t.TempDir()
	writeUsers(t, dir, `{"name":"guest","password_hash":"`+decoyHash+`","role":"veiwer"}`)
	f, err := LoadUserFile(filepath.Join(dir, "config.toml"))
	if err != nil {
		t.Fatal(err)
	}
	_, err = NewService(Config{UserFile: f})
	if err == nil || !strings.Contains(err.Error(), `"guest"`) || !strings.Contains(err.Error(), "veiwer") {
		t.Errorf("NewService: %v, want a refusal naming the account and the role", err)
	}
	_, err = NewService(Config{Enabled: true, Users: []User{{Name: "pierre", PasswordHash: decoyHash, Role: "admin"}}})
	if err == nil || !strings.Contains(err.Error(), `"pierre"`) {
		t.Errorf("NewService with a configured account: %v, want a refusal naming it", err)
	}
}

// Sign-in off is this machine's user, who owns the server; the account the
// first-run page creates is the owner and is written without a role, like
// every file before.
func TestTheFirstAccountAndLocalOnlyAreTheOwner(t *testing.T) {
	dir := t.TempDir()
	s := startIn(t, dir, true)
	if got := s.Role(httptest.NewRequest("GET", "/", nil)); got != RoleOwner {
		t.Errorf("sign-in off: %q, want owner", got)
	}
	if err := s.AddUser("pierre", "correcthorsebattery"); err != nil {
		t.Fatal(err)
	}
	if got := s.Role(requestAs(t, s, "pierre")); got != RoleOwner {
		t.Errorf("the first account: %q, want owner", got)
	}
	raw, err := os.ReadFile(filepath.Join(dir, DefaultUserFile))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(raw), "role") {
		t.Errorf("the first account was written with a role:\n%s", raw)
	}
}
