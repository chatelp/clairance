package api

import (
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"

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
