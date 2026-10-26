package api

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	gorilla "github.com/gorilla/websocket"

	"github.com/yegors/co-atc/internal/auth"
	"github.com/yegors/co-atc/internal/config"
)

// The server once it is reached from the Internet (Q62): a reverse proxy on
// another machine of the owner's network terminates HTTPS and forwards to
// co-atc, bound to every interface, in plain HTTP, with X-Forwarded-For and
// X-Forwarded-Proto. These tests make every request look as that proxy
// forwards it.
const (
	proxyAddr    = "192.0.2.1" // the proxy's machine; httptest.NewRequest's own address
	publicHost   = "clairance.example.org"
	publicOrigin = "https://" + publicHost
)

// throughTheProxy makes a request arrive as the proxy forwards it: from the
// proxy's machine, the browser's address and scheme in the forwarded headers,
// the Host the browser asked for kept.
func throughTheProxy(client string) func(*http.Request) {
	return func(r *http.Request) {
		r.RemoteAddr = proxyAddr + ":40000"
		r.Host = publicHost
		r.Header.Set("X-Forwarded-For", client)
		r.Header.Set("X-Forwarded-Proto", "https")
		r.Header.Set("X-Forwarded-Host", publicHost)
	}
}

// publicStack is the owner's production as the friend reaches it: bound to
// every interface, the proxy declared, the owner and one viewer, and a page
// to serve.
func publicStack(t *testing.T, o stackOptions) *stack {
	t.Helper()
	pages := t.TempDir()
	if err := os.WriteFile(filepath.Join(pages, "index.html"), []byte("<title>Clairance</title>"), 0o644); err != nil {
		t.Fatal(err)
	}
	o.host, o.proxies, o.viewer, o.static = "0.0.0.0", []string{proxyAddr}, true, pages
	return newStack(t, o)
}

// mountedRoutes is every method and route the router mounts, sorted, as in
// testdata/routes.txt.
func mountedRoutes(t *testing.T, s *stack) [][2]string {
	t.Helper()
	var out [][2]string
	walk := func(method, route string, _ http.Handler, _ ...func(http.Handler) http.Handler) error {
		out = append(out, [2]string{method, route})
		return nil
	}
	if err := chi.Walk(s.routes.(chi.Routes), walk); err != nil {
		t.Fatal(err)
	}
	sort.Slice(out, func(i, j int) bool { return out[i][0]+" "+out[i][1] < out[j][0]+" "+out[j][1] })
	return out
}

// example is one request for a route: a path where its parameters are
// filled, and a body that the route accepts from someone allowed to make it.
// A route with parameters and no example fails the test, so that a route
// added later is not left out of the inventory.
func example(t *testing.T, s *stack, method, route string, clip int64) (path, body string) {
	t.Helper()
	paths := map[string]string{
		"/*":                                         "/",
		"/api/v1/aircraft/{id}":                      "/api/v1/aircraft/" + hexCruise,
		"/api/v1/aircraft/{id}/tracks":               "/api/v1/aircraft/" + hexCruise + "/tracks",
		"/api/v1/airports/{ident}":                   "/api/v1/airports/LFPG",
		"/api/v1/frequencies/{id}":                   "/api/v1/frequencies/mix",
		"/api/v1/frequencies/{id}/label":             "/api/v1/frequencies/mix/label",
		"/api/v1/navaids/{ident}":                    "/api/v1/navaids/CGN",
		"/api/v1/sources/{id}":                       "/api/v1/sources/added",
		"/api/v1/stream/{id}":                        "/api/v1/stream/mix",
		"/api/v1/transcriptions/callsign/{callsign}": "/api/v1/transcriptions/callsign/AFR123",
		"/api/v1/transcriptions/frequency/{id}":      "/api/v1/transcriptions/frequency/mix",
		"/api/v1/transcriptions/speaker/{type}":      "/api/v1/transcriptions/speaker/ATC",
		"/api/v1/transcriptions/{id}/audio":          "/api/v1/transcriptions/" + itoa(clip) + "/audio",
		"/api/v1/transcriptions/time-range":          "/api/v1/transcriptions/time-range?start_time=2026-10-10T00:00:00Z",
	}
	// GET on a stream that does not exist: what is measured is who gets past
	// the guards, not the audio, which HEAD on a real one already shows.
	if method == http.MethodGet && route == "/api/v1/stream/{id}" {
		paths[route] = "/api/v1/stream/nope?id=inventory"
	}
	bodies := map[string]string{
		"PUT /api/v1/frequencies/{id}/label": `{"label":"CDG approaches"}`,
		"PUT /api/v1/sources/{id}":           `{"name":"Added","url":"` + s.gone + `/added.mp3"}`,
		"PUT /api/v1/server/settings":        `{}`,
		"PUT /api/v1/access":                 `{}`,
		"PUT /api/v1/receiver":               `{"state":"on_air"}`,
		"POST /api/v1/auth/login":            `{"name":"` + testUser + `","password":"not the password"}`,
		"POST /api/v1/setup":                 `{"mode":"account","username":"intruder","password":"0123456789abc"}`,
	}
	path, ok := paths[route]
	if !ok {
		if strings.Contains(route, "{") {
			t.Fatalf("no example request for %s %s: add one", method, route)
		}
		path = route
	}
	return path, bodies[method+" "+route]
}

// Who may do what through the proxy, route by route: an anonymous visitor, a
// viewer, a viewer when server.audio_replay = "all", and the owner. Frozen in
// testdata/access.txt, like the routes themselves: a change in what anyone
// may do shows in the diff, and ATC_WRITE_ACCESS=1 rewrites the file.
func TestWhoMayDoWhatThroughTheProxy(t *testing.T) {
	s := publicStack(t, stackOptions{})
	clip := s.transcribe("mix", "AFR123 descend FL120", "AFR123", time.Now().Add(-time.Minute))
	viewer, owner := s.signInAs(viewerUser), s.signInAs(testUser)

	type column struct {
		name, account string
		cookie        *http.Cookie
		audio         string
	}
	columns := []column{
		{"anyone", "", nil, config.AudioReplayOwner},
		{"viewer", viewerUser, viewer, config.AudioReplayOwner},
		{"viewer+audio", viewerUser, viewer, config.AudioReplayAll},
		{"owner", testUser, owner, config.AudioReplayOwner},
	}

	lines := []string{"# Through the proxy: anyone (no session), viewer, viewer with audio_replay = \"all\", owner."}
	for _, mr := range mountedRoutes(t, s) {
		method, route := mr[0], mr[1]
		path, body := example(t, s, method, route, clip)
		codes := make([]string, len(columns))
		for i, c := range columns {
			s.cookie = c.cookie
			if c.cookie != nil && route == "/api/v1/auth/logout" {
				// Signing out ends the session it is made with: a session of
				// its own, so that the column's goes on.
				s.signInAs(c.account)
			}
			s.cfg.Server.AudioReplay = c.audio
			client := fmt.Sprintf("203.0.113.%d", 10+i)
			codes[i] = fmt.Sprint(s.do(method, path, body, throughTheProxy(client)).Code)
		}
		lines = append(lines, fmt.Sprintf("%-7s %-44s %s", method, route, strings.Join(codes, " ")))
	}
	got := strings.Join(lines, "\n") + "\n"

	const frozen = "testdata/access.txt"
	if os.Getenv("ATC_WRITE_ACCESS") == "1" {
		if err := os.WriteFile(frozen, []byte(got), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	want, err := os.ReadFile(frozen)
	if err != nil {
		t.Fatal(err)
	}
	if got != string(want) {
		t.Errorf("who may do what through the proxy differs from %s\n--- now\n%s--- frozen\n%s", frozen, got, want)
	}
}

// lockedRoutesOf is every route behind the sign-in lock, from the router
// itself: those with more middleware than the root, as in routes_test.go.
func lockedRoutesOf(t *testing.T, s *stack) [][2]string {
	t.Helper()
	routes := s.routes.(chi.Routes)
	root := len(routes.Middlewares())
	var out [][2]string
	walk := func(method, route string, _ http.Handler, mws ...func(http.Handler) http.Handler) error {
		if len(mws) > root {
			out = append(out, [2]string{method, route})
		}
		return nil
	}
	if err := chi.Walk(routes, walk); err != nil {
		t.Fatal(err)
	}
	return out
}

// Through the proxy, no data route answers anyone without a session, and
// the refusal is the JSON the page reads.
func TestThroughTheProxyEveryLockedRouteNeedsASession(t *testing.T) {
	s := publicStack(t, stackOptions{})
	locked := lockedRoutesOf(t, s)
	if len(locked) < 30 {
		t.Fatalf("%d locked routes found, the router has more", len(locked))
	}
	for _, lr := range locked {
		path, body := example(t, s, lr[0], lr[1], 1)
		rec := s.do(lr[0], path, body, throughTheProxy("203.0.113.7"))
		if rec.Code != http.StatusUnauthorized || !strings.Contains(rec.Body.String(), "authentication required") {
			t.Errorf("%s %s without a session: %d %q, want 401", lr[0], path, rec.Code, rec.Body.String())
		}
	}
}

// Through the proxy, a viewer is refused every route that writes, with the
// audio given or not, and nothing it asked for has happened.
func TestThroughTheProxyAViewerChangesNothing(t *testing.T) {
	s := publicStack(t, stackOptions{})
	owner := s.signInAs(testUser)
	snapshot := func() string {
		s.cookie = owner
		var server ServerState
		var freqs struct {
			Frequencies []struct{ ID, Name string } `json:"frequencies"`
		}
		var status map[string]any
		s.get("/api/v1/server", &server)
		s.get("/api/v1/frequencies", &freqs)
		s.get("/api/v1/status", &status)
		out, err := json.Marshal([]any{server.Settings, server.Access, freqs.Frequencies, status["receiver"]})
		if err != nil {
			t.Fatal(err)
		}
		return string(out)
	}
	before := snapshot()

	viewer := s.signInAs(viewerUser)
	writes := 0
	for _, audio := range []string{config.AudioReplayOwner, config.AudioReplayAll} {
		s.cfg.Server.AudioReplay = audio
		for _, lr := range lockedRoutesOf(t, s) {
			if lr[0] == http.MethodGet || lr[0] == http.MethodHead {
				continue
			}
			writes++
			s.cookie = viewer
			path, body := example(t, s, lr[0], lr[1], 1)
			if rec := s.do(lr[0], path, body, throughTheProxy("203.0.113.8")); rec.Code != http.StatusForbidden {
				t.Errorf("viewer %s %s with audio_replay = %q: %d, want 403", lr[0], path, audio, rec.Code)
			}
		}
	}
	if writes < 2*7 {
		t.Errorf("%d writes tried, the router has more", writes)
	}
	if after := snapshot(); after != before {
		t.Errorf("a viewer changed the server\n before %s\n after  %s", before, after)
	}
}

// The audio -- the live streams and the archived clips -- is the owner's
// unless server.audio_replay = "all"; with it a viewer hears the stream
// through the proxy's address, which the frequencies point to (stream_port
// 0, 43a5eed).
func TestThroughTheProxyTheAudioFollowsAudioReplay(t *testing.T) {
	s := publicStack(t, stackOptions{})
	clip := itoa(s.transcribe("mix", "AFR123 descend FL120", "AFR123", time.Now().Add(-time.Minute)))
	s.signInAs(viewerUser)
	audio := [][2]string{{"HEAD", "/api/v1/stream/mix"}, {"GET", "/api/v1/stream/mix?id=friend-1"},
		{"GET", "/api/v1/transcriptions/" + clip + "/audio"}}

	for _, req := range audio {
		if rec := s.do(req[0], req[1], "", throughTheProxy("203.0.113.9")); rec.Code != http.StatusForbidden {
			t.Errorf("viewer %s %s, audio_replay = \"owner\": %d, want 403", req[0], req[1], rec.Code)
		}
	}
	s.cfg.Server.AudioReplay = config.AudioReplayAll
	if rec := s.do("HEAD", "/api/v1/stream/mix", "", throughTheProxy("203.0.113.9")); rec.Code != http.StatusOK {
		t.Errorf("viewer HEAD stream, audio_replay = \"all\": %d, want 200", rec.Code)
	}
	// No archive on this stack: past the guard, the clip is not found.
	if rec := s.do("GET", audio[2][1], "", throughTheProxy("203.0.113.9")); rec.Code != http.StatusNotFound {
		t.Errorf("viewer clip, audio_replay = \"all\": %d, want 404 from the handler", rec.Code)
	}
}

// The same, on a real port and a real tone: the viewer asks the frequencies
// through the proxy, is told to fetch the stream where the page is, and audio
// arrives there.
func TestThroughTheProxyAViewerHearsTheStreamWhereThePageIs(t *testing.T) {
	if _, err := exec.LookPath("ffmpeg"); err != nil {
		t.Skip("ffmpeg not installed")
	}
	// The test's connections come from the loopback: the proxy is on it.
	s := newStack(t, stackOptions{host: "0.0.0.0", proxies: []string{"127.0.0.1"}, viewer: true, tone: true})
	s.cfg.Server.AudioReplay = config.AudioReplayAll
	s.signInAs(viewerUser)

	var freqs struct {
		Frequencies []struct {
			ID         string `json:"id"`
			StreamURL  string `json:"stream_url"`
			StreamPort int    `json:"stream_port"`
		} `json:"frequencies"`
	}
	rec := s.do("GET", "/api/v1/frequencies", "", throughTheProxy("203.0.113.9"),
		func(r *http.Request) { r.RemoteAddr = "127.0.0.1:40000" })
	decode(t, rec, &freqs)
	streamURL := ""
	for _, f := range freqs.Frequencies {
		if f.StreamPort != 0 {
			t.Errorf("%s: stream_port %d through the proxy, want 0 (the page's own address)", f.ID, f.StreamPort)
		}
		if f.ID == "tone" {
			streamURL = f.StreamURL
		}
	}
	if streamURL == "" {
		t.Fatalf("no stream for the tone: %+v", freqs)
	}

	req, _ := http.NewRequest("GET", s.srv.URL+streamURL+"?id=friend-1", nil)
	req.Host = publicHost
	req.Header.Set("X-Forwarded-For", "203.0.113.9")
	req.Header.Set("X-Forwarded-Proto", "https")
	req.AddCookie(s.cookie)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	resp, err := http.DefaultClient.Do(req.WithContext(ctx))
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	n, _ := io.ReadAtLeast(resp.Body, make([]byte, 16000), 16000)
	if resp.StatusCode != http.StatusOK || n < 16000 {
		t.Errorf("the stream through the proxy: %d, %d bytes, want 200 and audio", resp.StatusCode, n)
	}
}

// The WebSocket through the proxy: the public page, whose origin is the Host
// the proxy keeps, opens it with its session; a foreign page does not, nor
// the public page without a session. A proxy that rewrites the Host needs
// the public origin in server.allowed_origins.
func TestThroughTheProxyTheWebSocketTakesThePublicPageOnly(t *testing.T) {
	dial := func(s *stack, host, origin string, withCookie bool) int {
		h := http.Header{}
		h.Set("Host", host)
		h.Set("X-Forwarded-For", "203.0.113.9")
		h.Set("X-Forwarded-Proto", "https")
		if withCookie {
			h.Set("Cookie", auth.CookieName+"="+s.cookie.Value)
		}
		if origin != "" {
			h.Set("Origin", origin)
		}
		conn, resp, err := gorilla.DefaultDialer.Dial(s.wsURL(), h)
		if conn != nil {
			conn.Close()
		}
		if resp == nil {
			t.Fatalf("dialing with Origin %q: %v", origin, err)
		}
		return resp.StatusCode
	}

	// The test dials from the loopback: the proxy is on it.
	s := newStack(t, stackOptions{host: "0.0.0.0", proxies: []string{"127.0.0.1"}, viewer: true})
	s.signInAs(viewerUser)
	for _, c := range []struct {
		host, origin string
		cookie       bool
		want         int
	}{
		{publicHost, publicOrigin, true, http.StatusSwitchingProtocols},
		{publicHost, "https://evil.example", true, http.StatusForbidden},
		{publicHost, "https://" + publicHost + ".evil.example", true, http.StatusForbidden},
		{publicHost, "null", true, http.StatusForbidden},
		{publicHost, publicOrigin, false, http.StatusUnauthorized},
		{"10.0.0.5:8000", publicOrigin, true, http.StatusForbidden}, // the Host rewritten, origin not declared
	} {
		if got := dial(s, c.host, c.origin, c.cookie); got != c.want {
			t.Errorf("Host %s, Origin %q, session %v: %d, want %d", c.host, c.origin, c.cookie, got, c.want)
		}
	}

	declared := newStack(t, stackOptions{host: "0.0.0.0", proxies: []string{"127.0.0.1"}, viewer: true,
		origins: []string{publicOrigin}})
	declared.signInAs(viewerUser)
	if got := dial(declared, "10.0.0.5:8000", publicOrigin, true); got != http.StatusSwitchingProtocols {
		t.Errorf("Host rewritten, public origin declared: %d, want 101", got)
	}
	if got := dial(declared, "10.0.0.5:8000", "https://evil.example", true); got != http.StatusForbidden {
		t.Errorf("Host rewritten, foreign origin: %d, want 403", got)
	}
}

// Once an account exists the first-run page is closed, to anyone through the
// proxy and to a signed-in viewer alike, and what it was sent creates nobody.
func TestThroughTheProxyTheFirstRunPageIsClosed(t *testing.T) {
	s := publicStack(t, stackOptions{})
	for _, cookie := range []*http.Cookie{nil, s.signInAs(viewerUser)} {
		s.cookie = cookie
		for _, body := range []string{`{"mode":"local"}`,
			`{"mode":"account","username":"intruder","password":"0123456789abc"}`} {
			if rec := s.do("POST", "/api/v1/setup", body, throughTheProxy("203.0.113.9")); rec.Code != http.StatusConflict {
				t.Errorf("POST /setup %s: %d, want 409", body, rec.Code)
			}
		}
	}
	var st SetupState
	decode(t, s.do("GET", "/api/v1/setup/status", "", throughTheProxy("203.0.113.9")), &st)
	if st.Needed {
		t.Error("setup still needed with two accounts")
	}
	s.cookie = nil
	rec := s.do("POST", "/api/v1/auth/login", `{"name":"intruder","password":"0123456789abc"}`, throughTheProxy("203.0.113.9"))
	if rec.Code != http.StatusUnauthorized {
		t.Errorf("the intruder signs in: %d", rec.Code)
	}
}

// The launcher's health is not served through the proxy: not to anyone, not
// to a viewer, not when the proxy is on this machine and connects from its
// loopback. The owner's session reads it from anywhere, by design.
func TestThroughTheProxyTheHealthDetailIsRefused(t *testing.T) {
	s := publicStack(t, stackOptions{})
	if rec := s.do("GET", "/api/v1/health/detail", "", throughTheProxy("203.0.113.9")); rec.Code != http.StatusForbidden {
		t.Errorf("anyone: %d, want 403", rec.Code)
	}
	s.signInAs(viewerUser)
	if rec := s.do("GET", "/api/v1/health/detail", "", throughTheProxy("203.0.113.9")); rec.Code != http.StatusForbidden {
		t.Errorf("a viewer: %d, want 403", rec.Code)
	}
	local := newStack(t, stackOptions{host: "0.0.0.0", proxies: []string{"127.0.0.1"}})
	onThisMachine := func(r *http.Request) { r.RemoteAddr, r.Host = "127.0.0.1:40000", "127.0.0.1:8000" }
	if rec := local.do("GET", "/api/v1/health/detail", "", throughTheProxy("203.0.113.9"), onThisMachine); rec.Code != http.StatusForbidden {
		t.Errorf("a proxy on this machine: %d, want 403", rec.Code)
	}
	s.signInAs(testUser)
	if rec := s.do("GET", "/api/v1/health/detail", "", throughTheProxy("203.0.113.9")); rec.Code != http.StatusOK {
		t.Errorf("the owner: %d, want 200", rec.Code)
	}
}

// The session cookie through the proxy on HTTPS: Secure, HttpOnly, SameSite
// Strict, for the whole site; and signing out clears it with the same flags.
func TestThroughTheProxyTheCookieIsSecureHttpOnlyAndStrict(t *testing.T) {
	s := publicStack(t, stackOptions{})
	check := func(what string, rec *httptest.ResponseRecorder) {
		t.Helper()
		cs := rec.Result().Cookies()
		if len(cs) != 1 {
			t.Fatalf("%s: cookies %v, want one", what, cs)
		}
		c := cs[0]
		if c.Name != auth.CookieName || !c.Secure || !c.HttpOnly || c.SameSite != http.SameSiteStrictMode ||
			c.Path != "/" || c.Domain != "" {
			t.Errorf("%s: %+v, want Secure, HttpOnly, SameSite=Strict, Path=/, no Domain", what, c)
		}
	}
	rec := s.do("POST", "/api/v1/auth/login", `{"name":"`+viewerUser+`","password":"`+testPassword+`"}`,
		throughTheProxy("203.0.113.9"))
	check("sign-in", rec)
	s.cookie = rec.Result().Cookies()[0]
	check("sign-out", s.do("POST", "/api/v1/auth/logout", "", throughTheProxy("203.0.113.9")))
}

// Through the proxy, the address held responsible for a sign-in is the
// browser's as the proxy appends it, last in X-Forwarded-For: a client that
// varies what it sends before it is still the same address, and is locked
// out after eight wrong passwords; another browser behind the same proxy is
// not locked out with it.
func TestThroughTheProxyTheLimitIsTheBrowsersAddress(t *testing.T) {
	s := publicStack(t, stackOptions{})
	wrong := `{"name":"` + testUser + `","password":"wrong"}`
	right := `{"name":"` + testUser + `","password":"` + testPassword + `"}`
	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			// What a client can put in the header itself; the proxy appends
			// the address it sees.
			spoofed := fmt.Sprintf("198.51.100.%d, 203.0.113.66", i)
			s.do("POST", "/api/v1/auth/login", wrong, throughTheProxy(spoofed))
		}(i)
	}
	wg.Wait()
	if rec := s.do("POST", "/api/v1/auth/login", right, throughTheProxy("10.9.8.7, 203.0.113.66")); rec.Code != http.StatusUnauthorized {
		t.Errorf("the right password after eight wrong ones, another spoofed prefix: %d, want 401", rec.Code)
	}
	if rec := s.do("POST", "/api/v1/auth/login", right, throughTheProxy("203.0.113.67")); rec.Code != http.StatusOK {
		t.Errorf("another browser behind the same proxy: %d, want 200", rec.Code)
	}
}

// A name that does not exist and a wrong password are answered alike --
// status, body, headers, no cookie -- so the form says nothing of which
// accounts exist. The time they take is measured in fiche O's report.
func TestThroughTheProxyAWrongNameAndAWrongPasswordLookAlike(t *testing.T) {
	s := publicStack(t, stackOptions{})
	answer := func(name string) string {
		rec := s.do("POST", "/api/v1/auth/login", `{"name":"`+name+`","password":"wrong password"}`,
			throughTheProxy("203.0.113.9"))
		h := rec.Header().Clone()
		return fmt.Sprintf("%d %q %v %d", rec.Code, rec.Body.String(), h, len(rec.Result().Cookies()))
	}
	known, unknown := answer(testUser), answer("nobody-by-that-name")
	if known != unknown {
		t.Errorf("a wrong password and an unknown name differ:\n %s\n %s", known, unknown)
	}
}
