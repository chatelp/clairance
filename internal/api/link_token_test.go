package api

import (
	"net/http"
	"testing"
)

// The station link's key opens its five routes, from this machine, and
// nothing else: not another route, not from elsewhere, not through a proxy on
// this machine, not when it is wrong, not when no link was started.
func TestTheLinkKeyOpensTheLinksRoutesFromThisMachineOnly(t *testing.T) {
	s := newStack(t, stackOptions{})
	s.handler.SetLinkToken("k3y-made-at-start")
	key := func(v string) func(*http.Request) {
		return func(r *http.Request) { r.Header.Set("Authorization", "Bearer "+v) }
	}
	here := fromLoopback("127.0.0.1:50123", "127.0.0.1:8000")

	// What radio-ctl-sync does, in its order.
	for _, c := range []struct {
		method, path, body string
		want               int
	}{
		{"PUT", "/api/v1/receiver", `{"state":"on_air","title":"Selection libre"}`, http.StatusNoContent},
		{"GET", "/api/v1/frequencies", "", http.StatusOK},
		{"PUT", "/api/v1/frequencies/mix/label", `{"label":"En route et descente CDG"}`, http.StatusOK},
		{"PUT", "/api/v1/sources/124350", sourceBody(s.gone, "124350"), http.StatusOK},
		{"DELETE", "/api/v1/sources/124350", "", http.StatusNoContent},
	} {
		if rec := s.do(c.method, c.path, c.body, here, key("k3y-made-at-start")); rec.Code != c.want {
			t.Errorf("the link's %s %s: %d, want %d (%s)", c.method, c.path, rec.Code, c.want, rec.Body.String())
		}
	}

	refused := []struct {
		name         string
		method, path string
		set          []func(*http.Request)
	}{
		{"another route that reads", "GET", "/api/v1/aircraft", []func(*http.Request){here, key("k3y-made-at-start")}},
		{"another route that writes", "PUT", "/api/v1/server/settings", []func(*http.Request){here, key("k3y-made-at-start")}},
		{"the audio", "GET", "/api/v1/stream/mix", []func(*http.Request){here, key("k3y-made-at-start")}},
		{"the sign-in trace", "GET", "/api/v1/signins", []func(*http.Request){here, key("k3y-made-at-start")}},
		{"from elsewhere", "GET", "/api/v1/frequencies", []func(*http.Request){key("k3y-made-at-start")}},
		{"through a proxy on this machine", "GET", "/api/v1/frequencies", []func(*http.Request){here, key("k3y-made-at-start"),
			func(r *http.Request) { r.Header.Set("X-Forwarded-For", "203.0.113.9") }}},
		{"a rebound name", "GET", "/api/v1/frequencies", []func(*http.Request){fromLoopback("127.0.0.1:50123", "rebind.example:8000"), key("k3y-made-at-start")}},
		{"a wrong key", "GET", "/api/v1/frequencies", []func(*http.Request){here, key("k3y-made-at-staRT")}},
		{"the key without Bearer", "GET", "/api/v1/frequencies", []func(*http.Request){here,
			func(r *http.Request) { r.Header.Set("Authorization", "k3y-made-at-start") }}},
	}
	for _, c := range refused {
		if rec := s.do(c.method, c.path, "", c.set...); rec.Code != http.StatusUnauthorized {
			t.Errorf("%s: %d, want 401 (%s)", c.name, rec.Code, rec.Body.String())
		}
	}

	// No link started: no key opens anything, the empty one least of all.
	s.handler.SetLinkToken("")
	for _, v := range []string{"k3y-made-at-start", ""} {
		if rec := s.do("GET", "/api/v1/frequencies", "", here, key(v)); rec.Code != http.StatusUnauthorized {
			t.Errorf("no link, key %q: %d, want 401", v, rec.Code)
		}
	}
}
