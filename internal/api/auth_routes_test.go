package api

import (
	"net/http"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/yegors/co-atc/internal/auth"
)

// Every route behind the lock, with what each answers once signed in. Most
// answer 200 on this stack; the few that do not say why: the reference data
// is not loaded here, and /ws takes a WebSocket handshake, not a plain GET.
type route struct {
	method, path, body string
	signedIn           int
}

func lockedRoutes(s *stack) []route {
	return []route{
		{"GET", "/api/v1/aircraft", "", 200},
		{"GET", "/api/v1/aircraft/" + hexCruise, "", 200},
		{"GET", "/api/v1/aircraft/" + hexCruise + "/tracks", "", 200},
		{"GET", "/api/v1/frequencies", "", 200},
		{"GET", "/api/v1/frequencies/mix", "", 200},
		{"PUT", "/api/v1/frequencies/mix/label", `{"label":"CDG approaches"}`, 200},
		{"PUT", "/api/v1/sources/added", `{"name":"Added","url":"` + s.gone + `/added.mp3"}`, 200},
		{"DELETE", "/api/v1/sources/added", "", 204},
		{"HEAD", "/api/v1/stream/mix", "", 200},
		{"GET", "/api/v1/ws", "", 400}, // no Upgrade header: the hub refuses the handshake
		{"GET", "/api/v1/transcriptions", "", 200},
		{"GET", "/api/v1/transcriptions/frequency/mix", "", 200},
		{"GET", "/api/v1/transcriptions/time-range?start_time=2026-10-10T00:00:00Z", "", 200},
		{"GET", "/api/v1/transcriptions/speaker/ATC", "", 200},
		{"GET", "/api/v1/transcriptions/callsign/AFR123", "", 200},
		{"GET", "/api/v1/config", "", 200},
		{"GET", "/api/v1/server", "", 200},
		{"PUT", "/api/v1/server/settings", `{}`, 200},
		{"PUT", "/api/v1/access", `{}`, 400}, // a malformed switch, past the lock
		{"GET", "/api/v1/adsb/source", "", 200},
		{"GET", "/api/v1/station", "", 200},
		{"GET", "/api/v1/wx", "", 200},
		{"GET", "/api/v1/airports", "", 200},
		{"GET", "/api/v1/airports/LFPG", "", 404}, // no reference data on this stack
		{"GET", "/api/v1/heliports", "", 200},
		{"GET", "/api/v1/navaids", "", 200},
		{"GET", "/api/v1/navaids/CGN", "", 404},
		{"GET", "/api/v1/runways", "", 200},
	}
}

// Without a session every data route is turned away before its handler runs,
// and only health, the sign-in question and the first-run question answer;
// with a session, each route answers what it is for.
func TestTheLockAndWhatIsReachableWithoutIt(t *testing.T) {
	s := newStack(t, stackOptions{})
	routes := lockedRoutes(s)
	for _, r := range routes {
		rec := s.do(r.method, r.path, r.body)
		if rec.Code != http.StatusUnauthorized {
			t.Errorf("%s %s without a session: got %d, want 401", r.method, r.path, rec.Code)
			continue
		}
		if ct := rec.Header().Get("Content-Type"); ct != "application/json" {
			t.Errorf("%s %s: a 401 is JSON for the page, got %q", r.method, r.path, ct)
		}
		if !strings.Contains(rec.Body.String(), "authentication required") {
			t.Errorf("%s %s: 401 body %q", r.method, r.path, rec.Body.String())
		}
	}
	for _, path := range []string{"/api/v1/health", "/api/v1/auth/status", "/api/v1/setup/status"} {
		if rec := s.do("GET", path, ""); rec.Code != http.StatusOK {
			t.Errorf("GET %s without a session: got %d, want 200", path, rec.Code)
		}
	}
	var health struct {
		Status        bool `json:"status"`
		AircraftCount int  `json:"aircraft_count"`
	}
	s.get("/api/v1/health", &health)
	if !health.Status || health.AircraftCount != aircraftSeen {
		t.Errorf("health = %+v, want the fake receiver's %d aircraft and a good last fetch", health, aircraftSeen)
	}

	s.signIn()
	for _, r := range routes {
		rec := s.do(r.method, r.path, r.body)
		if rec.Code != r.signedIn {
			t.Errorf("%s %s signed in: got %d, want %d (%s)", r.method, r.path, rec.Code, r.signedIn,
				strings.TrimSpace(rec.Body.String()))
		}
	}
}

func TestAuthStatusSaysWhoIsSignedIn(t *testing.T) {
	s := newStack(t, stackOptions{})
	var st struct {
		Enabled       bool      `json:"enabled"`
		Authenticated bool      `json:"authenticated"`
		User          string    `json:"user"`
		ExpiresAt     time.Time `json:"expires_at"`
	}
	s.get("/api/v1/auth/status", &st)
	if !st.Enabled || st.Authenticated || st.User != "" {
		t.Errorf("before sign-in: %+v, want enabled and not authenticated", st)
	}
	s.signIn()
	s.get("/api/v1/auth/status", &st)
	if !st.Enabled || !st.Authenticated || st.User != testUser || st.ExpiresAt.IsZero() {
		t.Errorf("after sign-in: %+v, want authenticated as %s with an expiry", st, testUser)
	}
	// Local-only: nobody needs to sign in, and the page is told so.
	local := newStack(t, stackOptions{noAccount: true})
	local.get("/api/v1/auth/status", &st)
	if st.Enabled || !st.Authenticated {
		t.Errorf("local-only: %+v, want not enabled and authenticated", st)
	}
	if rec := local.do("GET", "/api/v1/aircraft", ""); rec.Code != http.StatusOK {
		t.Errorf("local-only data without a session: got %d, want 200", rec.Code)
	}
	if rec := local.do("POST", "/api/v1/auth/login", `{}`); rec.Code != http.StatusOK ||
		!strings.Contains(rec.Body.String(), `"authenticated":true`) {
		t.Errorf("local-only login: %d %s, want 200 authenticated", rec.Code, rec.Body.String())
	}
}

// One session, from the refusals before it to the sign-out after it.
func TestASessionFromSignInToSignOut(t *testing.T) {
	shared := newStack(t, stackOptions{})

	t.Run("a wrong password or a bad body gives no session", func(t *testing.T) {
		s := shared.with(t)
		rec := s.do("POST", "/api/v1/auth/login", `{"name":"`+testUser+`","password":"wrong-password"}`)
		if rec.Code != http.StatusUnauthorized || len(rec.Result().Cookies()) != 0 {
			t.Errorf("wrong password: %d with %d cookies, want 401 and none", rec.Code, len(rec.Result().Cookies()))
		}
		if !strings.Contains(rec.Body.String(), "invalid credentials") {
			t.Errorf("wrong password body %q", rec.Body.String())
		}
		if rec := s.do("POST", "/api/v1/auth/login", `not json`); rec.Code != http.StatusBadRequest {
			t.Errorf("malformed login: got %d, want 400", rec.Code)
		}
		if rec := s.do("POST", "/api/v1/auth/login", `{"name":"`+testUser+`","password":"`+strings.Repeat("x", 5000)+`"}`); rec.Code != http.StatusBadRequest {
			t.Errorf("a 5 KB login body: got %d, want 400 (the body is capped at 4 KB)", rec.Code)
		}
		if rec := s.do("GET", "/api/v1/aircraft", ""); rec.Code != http.StatusUnauthorized {
			t.Errorf("after the refusals the data is open: %d", rec.Code)
		}
	})

	// The session cookie as the browser must receive it: readable by no
	// script, sent by no other site, and over HTTP not marked Secure -- which
	// would make the browser drop it on the plain-HTTP install this project
	// runs. X-Forwarded-Proto from a client is not believed: this stack
	// declares no proxy.
	t.Run("signing in sets a strict HttpOnly cookie", func(t *testing.T) {
		s := shared.with(t)
		forwarded := func(r *http.Request) { r.Header.Set("X-Forwarded-Proto", "https") }
		rec := s.do("POST", "/api/v1/auth/login", `{"name":"`+testUser+`","password":"`+testPassword+`"}`, forwarded)
		if rec.Code != http.StatusOK {
			t.Fatalf("login: %d %s", rec.Code, rec.Body.String())
		}
		var body struct {
			Authenticated bool      `json:"authenticated"`
			User          string    `json:"user"`
			ExpiresAt     time.Time `json:"expires_at"`
		}
		decode(t, rec, &body)
		if !body.Authenticated || body.User != testUser || body.ExpiresAt.Before(time.Now().Add(50*time.Minute)) {
			t.Errorf("login body = %+v, want authenticated as %s for about an hour", body, testUser)
		}
		cookies := rec.Result().Cookies()
		if len(cookies) != 1 {
			t.Fatalf("got %d cookies, want 1: %v", len(cookies), cookies)
		}
		c := cookies[0]
		switch {
		case c.Name != auth.CookieName:
			t.Errorf("cookie name %q, want %q", c.Name, auth.CookieName)
		case !c.HttpOnly:
			t.Error("the session cookie is readable by scripts")
		case c.SameSite != http.SameSiteStrictMode:
			t.Errorf("SameSite = %v, want Strict", c.SameSite)
		case c.Path != "/":
			t.Errorf("Path = %q, want /", c.Path)
		case c.Secure:
			t.Error("Secure over plain HTTP, on the word of an untrusted client: the browser would never send it back")
		case c.Value == "" || c.Expires.IsZero():
			t.Errorf("cookie %+v, want a token and an expiry", c)
		}
		shared.cookie = c
	})

	t.Run("signing out ends the session", func(t *testing.T) {
		s := shared.with(t)
		if s.cookie == nil {
			t.Skip("no session from the step before")
		}
		if rec := s.do("GET", "/api/v1/aircraft", ""); rec.Code != http.StatusOK {
			t.Fatalf("signed in: %d", rec.Code)
		}
		rec := s.do("POST", "/api/v1/auth/logout", "")
		if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `"authenticated":false`) {
			t.Errorf("logout: %d %s", rec.Code, rec.Body.String())
		}
		cleared := false
		for _, c := range rec.Result().Cookies() {
			if c.Name == auth.CookieName && c.Value == "" && c.MaxAge < 0 {
				cleared = true
			}
		}
		if !cleared {
			t.Errorf("logout did not clear the cookie: %v", rec.Result().Cookies())
		}
		// The browser that kept the old cookie is out all the same: the
		// session is revoked on the server, not only forgotten by the client.
		if rec := s.do("GET", "/api/v1/aircraft", ""); rec.Code != http.StatusUnauthorized {
			t.Errorf("the old cookie after logout: %d, want 401", rec.Code)
		}
		var st struct{ Authenticated bool }
		s.get("/api/v1/auth/status", &st)
		if st.Authenticated {
			t.Error("auth/status still says authenticated after logout")
		}
		// Signing out twice, or without a session, is harmless.
		if rec := s.do("POST", "/api/v1/auth/logout", ""); rec.Code != http.StatusOK {
			t.Errorf("logout without a session: %d", rec.Code)
		}
	})
}

// Behind a trusted proxy that says the browser is on HTTPS, the cookie is
// Secure. (httptest.NewRequest comes from 192.0.2.1.)
func TestTheCookieIsSecureBehindATrustedProxyOnHTTPS(t *testing.T) {
	s := newStack(t, stackOptions{proxies: []string{"192.0.2.1"}})
	forwarded := func(r *http.Request) { r.Header.Set("X-Forwarded-Proto", "https") }
	rec := s.do("POST", "/api/v1/auth/login", `{"name":"`+testUser+`","password":"`+testPassword+`"}`, forwarded)
	if cs := rec.Result().Cookies(); len(cs) != 1 || !cs[0].Secure {
		t.Errorf("behind the trusted proxy on HTTPS: cookies %v, want one marked Secure", cs)
	}
}

// Eight wrong passwords from one address in a quarter of an hour, and the
// ninth attempt is refused even with the right one -- as too many attempts,
// 429, with how long to wait, so that the page says to wait rather than to
// try another password. The eight are sent at once: each wrong answer costs
// a 250 ms pause, which is the point of it.
func TestEightWrongPasswordsLockTheAddress(t *testing.T) {
	s := newStack(t, stackOptions{})
	var wg sync.WaitGroup
	codes := make([]int, 8)
	for i := range codes {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			codes[i] = s.do("POST", "/api/v1/auth/login", `{"name":"`+testUser+`","password":"wrong"}`).Code
		}(i)
	}
	wg.Wait()
	for i, code := range codes {
		if code != http.StatusUnauthorized {
			t.Errorf("wrong password #%d: got %d, want 401", i+1, code)
		}
	}
	rec := s.do("POST", "/api/v1/auth/login", `{"name":"`+testUser+`","password":"`+testPassword+`"}`)
	if rec.Code != http.StatusTooManyRequests || len(rec.Result().Cookies()) != 0 {
		t.Errorf("the right password after eight wrong ones: %d with %d cookies, want 429 and none",
			rec.Code, len(rec.Result().Cookies()))
	}
	// Fifteen minutes from the first wrong one, a moment ago.
	if wait, err := strconv.Atoi(rec.Header().Get("Retry-After")); err != nil || wait < 14*60 || wait > 15*60 {
		t.Errorf("Retry-After %q, want about 900 seconds", rec.Header().Get("Retry-After"))
	}
	// An unknown name from the blocked address is answered the same: the
	// block is the address's, and says nothing of the accounts.
	if other := s.do("POST", "/api/v1/auth/login", `{"name":"nobody","password":"x"}`); other.Code != http.StatusTooManyRequests {
		t.Errorf("an unknown name from the blocked address: %d, want 429", other.Code)
	}
	// Another address is not locked out with it. The limiter is per address,
	// and the address is the client's, not a forwarded header's: this stack
	// declares no proxy. The window itself (fifteen minutes) is not waited for.
	other := func(r *http.Request) { r.RemoteAddr = "192.0.2.77:4444" }
	rec = s.do("POST", "/api/v1/auth/login", `{"name":"`+testUser+`","password":"`+testPassword+`"}`, other)
	if rec.Code != http.StatusOK {
		t.Errorf("another address after the lockout: %d, want 200", rec.Code)
	}
}
