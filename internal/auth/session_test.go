package auth

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

const (
	ownerRecord = `{"name":"pierre","password_hash":"` + decoyHash + `","created_at":"2026-10-01T00:00:00Z"}`
	guestRecord = `{"name":"guest","password_hash":"` + decoyHash + `","created_at":"2026-10-26T00:00:00Z","role":"viewer"}`
)

// withCookie is a request carrying a session token, as a browser would after
// the restart: the token is all it has.
func withCookie(token string) *http.Request {
	r := httptest.NewRequest("GET", "/api/v1/aircraft", nil)
	r.AddCookie(&http.Cookie{Name: CookieName, Value: token})
	return r
}

func token(t *testing.T, r *http.Request) string {
	t.Helper()
	c, err := r.Cookie(CookieName)
	if err != nil {
		t.Fatal(err)
	}
	return c.Value
}

// The reason for the file: a restart of production no longer signs the guest
// out. The same cookie is accepted, with the same account and role.
func TestASessionOutlivesARestart(t *testing.T) {
	dir := t.TempDir()
	writeUsers(t, dir, ownerRecord+","+guestRecord)
	tok := token(t, requestAs(t, startIn(t, dir, false), "guest"))

	s := startIn(t, dir, false)
	if kept, dropped, err := s.Sessions().Restored(); kept != 1 || dropped != 0 || err != nil {
		t.Fatalf("Restored() = %d, %d, %v; want 1, 0, nil", kept, dropped, err)
	}
	sess, ok := s.SessionFrom(withCookie(tok))
	if !ok {
		t.Fatal("the session of before the restart is refused")
	}
	if sess.User != "guest" || sess.Token != tok {
		t.Errorf("session read back: user %q, token known %v", sess.User, sess.Token == tok)
	}
	if got := s.Role(withCookie(tok)); got != RoleViewer {
		t.Errorf("role after the restart: %q, want viewer", got)
	}
}

// Through the real sign-in: the role is the account's, and a second restart
// still knows the session -- it was rewritten under the same hash.
func TestASignInIsOnDiskAtOnce(t *testing.T) {
	dir := t.TempDir()
	hash, err := HashPassword("correct horse battery")
	if err != nil {
		t.Fatal(err)
	}
	writeUsers(t, dir, `{"name":"guest","password_hash":"`+hash+`","created_at":"2026-10-26T00:00:00Z","role":"viewer"}`)
	sess, err := startIn(t, dir, false).SignIn(httptest.NewRequest("POST", "/api/v1/auth/login", nil), "guest", "correct horse battery")
	if err != nil {
		t.Fatal(err)
	}
	if sess.Role != RoleViewer {
		t.Errorf("session issued with role %q, want viewer", sess.Role)
	}
	for i := 0; i < 2; i++ {
		if _, ok := startIn(t, dir, false).SessionFrom(withCookie(sess.Token)); !ok {
			t.Fatalf("restart %d: the session is refused", i+1)
		}
	}
}

// A sign-out that a restart undid would hand the browser back a session its
// user ended.
func TestASignOutOutlivesARestart(t *testing.T) {
	dir := t.TempDir()
	writeUsers(t, dir, ownerRecord+","+guestRecord)
	s := startIn(t, dir, false)
	gone, kept := token(t, requestAs(t, s, "guest")), token(t, requestAs(t, s, "pierre"))
	s.Sessions().Revoke(gone)

	s = startIn(t, dir, false)
	if _, ok := s.SessionFrom(withCookie(gone)); ok {
		t.Error("a session signed out before the restart is accepted after it")
	}
	if _, ok := s.SessionFrom(withCookie(kept)); !ok {
		t.Error("signing one session out ended another")
	}
}

func TestRevokingAUserOutlivesARestart(t *testing.T) {
	dir := t.TempDir()
	writeUsers(t, dir, ownerRecord+","+guestRecord)
	s := startIn(t, dir, false)
	a, b := token(t, requestAs(t, s, "guest")), token(t, requestAs(t, s, "guest"))
	if n := s.Sessions().RevokeUser("guest"); n != 2 {
		t.Fatalf("RevokeUser = %d, want 2", n)
	}
	s = startIn(t, dir, false)
	for _, tok := range []string{a, b} {
		if _, ok := s.SessionFrom(withCookie(tok)); ok {
			t.Error("a session of a revoked account is accepted after the restart")
		}
	}
}

// Removing an account from users.json and restarting is how the owner shuts
// a guest out; making an owner a viewer must not leave them an owner's
// session. Both are set aside at the start, and the file loses them.
func TestASessionWhoseAccountChangedIsDropped(t *testing.T) {
	dir := t.TempDir()
	writeUsers(t, dir, ownerRecord+","+guestRecord)
	s := startIn(t, dir, false)
	guest, owner := token(t, requestAs(t, s, "guest")), token(t, requestAs(t, s, "pierre"))

	writeUsers(t, dir, `{"name":"pierre","password_hash":"`+decoyHash+`","created_at":"2026-10-01T00:00:00Z","role":"viewer"}`)
	s = startIn(t, dir, false)
	if kept, dropped, err := s.Sessions().Restored(); kept != 0 || dropped != 2 || err != nil {
		t.Errorf("Restored() = %d, %d, %v; want 0, 2, nil", kept, dropped, err)
	}
	if _, ok := s.SessionFrom(withCookie(guest)); ok {
		t.Error("the session of a removed account is accepted")
	}
	if _, ok := s.SessionFrom(withCookie(owner)); ok {
		t.Error("an owner's session is accepted for an account that is now a viewer")
	}
	raw, _ := os.ReadFile(filepath.Join(dir, SessionFile))
	if strings.Contains(string(raw), `"user"`) {
		t.Errorf("the sessions set aside are still on disk:\n%s", raw)
	}
}

// A file that cannot be read costs a sign-in, never the server.
func TestAnUnreadableSessionFileStartsWithNone(t *testing.T) {
	for name, prepare := range map[string]func(path string) error{
		"corrupt":   func(p string) error { return os.WriteFile(p, []byte(`{"sessions":[{"hash":`), 0o600) },
		"directory": func(p string) error { return os.Mkdir(p, 0o700) },
	} {
		t.Run(name, func(t *testing.T) {
			dir := t.TempDir()
			writeUsers(t, dir, ownerRecord)
			if err := prepare(filepath.Join(dir, SessionFile)); err != nil {
				t.Fatal(err)
			}
			s := startIn(t, dir, false) // fails the test if the service does not build
			kept, _, err := s.Sessions().Restored()
			if err == nil || kept != 0 || s.Sessions().Count() != 0 {
				t.Errorf("Restored() = %d, _, %v; want 0 sessions and the error said", kept, err)
			}
		})
	}
	// And the next sign-in replaces the corrupt file with a good one.
	dir := t.TempDir()
	writeUsers(t, dir, ownerRecord)
	if err := os.WriteFile(filepath.Join(dir, SessionFile), []byte("not json"), 0o600); err != nil {
		t.Fatal(err)
	}
	tok := token(t, requestAs(t, startIn(t, dir, false), "pierre"))
	if _, ok := startIn(t, dir, false).SessionFrom(withCookie(tok)); !ok {
		t.Error("the file was not replaced at the next sign-in")
	}
}

// The file holds what a session is, never what opens it: reading it, a backup
// of it, a copy left on a disk, gives nothing a browser could present.
func TestTheSessionFileHoldsNoToken(t *testing.T) {
	dir := t.TempDir()
	writeUsers(t, dir, ownerRecord+","+guestRecord)
	s := startIn(t, dir, false)
	var tokens []string
	for _, name := range []string{"pierre", "guest", "guest"} {
		tokens = append(tokens, token(t, requestAs(t, s, name)))
	}
	path := filepath.Join(dir, SessionFile)
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	for _, tok := range tokens {
		if strings.Contains(string(raw), tok) {
			t.Fatalf("a token is in the file:\n%s", raw)
		}
		if !strings.Contains(string(raw), tokenHash(tok)) {
			t.Errorf("the hash of a session is missing from the file")
		}
	}
	if st, err := os.Stat(path); err != nil || st.Mode().Perm() != 0o600 {
		t.Errorf("file mode %v, %v; want 0600", st.Mode().Perm(), err)
	}
	if _, err := os.Stat(path + ".tmp"); !os.IsNotExist(err) {
		t.Errorf("the temporary file is left behind: %v", err)
	}
}

// The sliding expiry is written once a minute at most, and at the stop:
// every request moving a date by seconds is not worth a disk write.
func TestTheSlidingExpiryIsWrittenOnceAMinute(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, SessionFile)
	now := time.Date(2026, 10, 28, 20, 0, 0, 0, time.UTC)
	st := NewStore(30 * 24 * time.Hour)
	st.now = func() time.Time { return now }
	st.open(path, func(string, string) bool { return true })
	sess, _ := st.Create("guest", RoleViewer)
	expiryOnDisk := func() time.Time {
		t.Helper()
		var f sessionFile
		raw, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		if err := json.Unmarshal(raw, &f); err != nil || len(f.Sessions) != 1 {
			t.Fatalf("file: %v, %d sessions", err, len(f.Sessions))
		}
		return f.Sessions[0].ExpiresAt
	}
	created := expiryOnDisk()

	now = now.Add(30 * time.Second)
	st.Lookup(sess.Token)
	if !expiryOnDisk().Equal(created) {
		t.Error("a use 30 s after the last write was written at once")
	}
	now = now.Add(31 * time.Second)
	st.Lookup(sess.Token)
	if want := now.Add(30 * 24 * time.Hour); !expiryOnDisk().Equal(want) {
		t.Errorf("a use a minute after the last write: on disk %v, want %v", expiryOnDisk(), want)
	}
	now = now.Add(10 * time.Second)
	st.Lookup(sess.Token)
	if err := st.Save(); err != nil {
		t.Fatal(err)
	}
	if want := now.Add(30 * 24 * time.Hour); !expiryOnDisk().Equal(want) {
		t.Errorf("after Save: on disk %v, want %v", expiryOnDisk(), want)
	}
}

// Expired on disk is expired: set aside at the start, whatever the account.
func TestASessionExpiredWhileStoppedIsDropped(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, SessionFile)
	st := NewStore(time.Hour)
	st.open(path, func(string, string) bool { return true })
	sess, _ := st.Create("guest", RoleViewer)

	later := NewStore(time.Hour)
	later.now = func() time.Time { return time.Now().Add(2 * time.Hour) }
	later.open(path, func(string, string) bool { return true })
	if kept, dropped, _ := later.Restored(); kept != 0 || dropped != 1 {
		t.Errorf("Restored() = %d, %d; want 0, 1", kept, dropped)
	}
	if _, ok := later.Lookup(sess.Token); ok {
		t.Error("a session expired while the server was stopped is accepted")
	}
}

// The expiry slides, but not past ninety days from the sign-in (D83): a
// session in daily use ends then, and its expiry never promises more.
func TestASessionEndsNinetyDaysAfterItsSignIn(t *testing.T) {
	st := NewStore(30 * 24 * time.Hour)
	now := time.Date(2026, 10, 27, 12, 0, 0, 0, time.UTC)
	st.now = func() time.Time { return now }
	sess, err := st.Create("guest", RoleViewer)
	if err != nil {
		t.Fatal(err)
	}
	end := now.Add(90 * 24 * time.Hour)
	for day := 1; day < 90; day++ {
		now = now.Add(24 * time.Hour)
		got, ok := st.Lookup(sess.Token)
		if !ok {
			t.Fatalf("day %d: a session in daily use ended", day)
		}
		if got.ExpiresAt.After(end) {
			t.Fatalf("day %d: expiry %v past the cap %v", day, got.ExpiresAt, end)
		}
	}
	now = end
	if _, ok := st.Lookup(sess.Token); ok {
		t.Error("ninety days after its sign-in, the session is still honoured")
	}
}

// The browser is given the cookie again with the slid expiry, once a day: it
// used to keep the sign-in's, and a guest in daily use was signed out on the
// thirtieth day while the server still held the session.
func TestTheCookieFollowsTheSlidingExpiry(t *testing.T) {
	dir := t.TempDir()
	writeUsers(t, dir, `{"name":"guest","password_hash":"`+decoyHash+`","role":"viewer"}`)
	s := startIn(t, dir, false)
	now := time.Now()
	s.store.now = func() time.Time { return now }
	r := requestAs(t, s, "guest")
	locked := s.Require(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
	cookieAfter := func() *http.Cookie {
		w := httptest.NewRecorder()
		locked.ServeHTTP(w, r)
		if w.Code != http.StatusOK {
			t.Fatalf("request: %d", w.Code)
		}
		for _, c := range w.Result().Cookies() {
			if c.Name == CookieName {
				return c
			}
		}
		return nil
	}

	if c := cookieAfter(); c != nil {
		t.Errorf("right after the sign-in, the cookie is set again: %v", c)
	}
	now = now.Add(23 * time.Hour)
	if c := cookieAfter(); c != nil {
		t.Errorf("23 h after the sign-in: cookie set again")
	}
	now = now.Add(2 * time.Hour)
	c := cookieAfter()
	if c == nil {
		t.Fatal("25 h after the sign-in: no cookie with the slid expiry")
	}
	if want := now.Add(30 * 24 * time.Hour); c.Expires.Sub(want).Abs() > time.Second {
		t.Errorf("cookie expires %v, want %v", c.Expires, want)
	}
	if c := cookieAfter(); c != nil {
		t.Errorf("set again on the next request: once a day is enough")
	}
	up := r.Clone(r.Context())
	up.Header.Set("Upgrade", "websocket")
	now = now.Add(25 * time.Hour)
	w := httptest.NewRecorder()
	locked.ServeHTTP(w, up)
	if len(w.Result().Cookies()) != 0 {
		t.Error("a WebSocket upgrade spent the renewal")
	}
	if cookieAfter() == nil {
		t.Error("the next plain request after the upgrade got no cookie")
	}
}
