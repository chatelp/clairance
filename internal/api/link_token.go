package api

import (
	"crypto/subtle"
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"
)

// The station link's key (docs-fr/36-contrat-lanceur.md, plan 35 B3). co-atc
// runs radio-ctl-sync itself and hands it a key made at each start, in its
// environment and written nowhere. The key opens the routes the link calls and
// no other, from this machine only: the link needs no account, and the
// owner's password no longer has to sit in a file for a script to read
// (docs-fr/05-decisions.md, D70).

// linkRoutes are what cmd/radio-ctl-sync calls, by method and route.
var linkRoutes = map[string]bool{
	"GET /api/v1/frequencies":            true,
	"PUT /api/v1/frequencies/{id}/label": true,
	"PUT /api/v1/sources/{id}":           true,
	"DELETE /api/v1/sources/{id}":        true,
	"PUT /api/v1/receiver":               true,
}

// SetLinkToken gives the handler the key of the station link co-atc started.
// Empty, the default, opens nothing. Set before the server listens.
func (h *Handler) SetLinkToken(token string) { h.linkToken = token }

// RequireSignIn is the lock in front of the data: a session, as Auth().Require
// asks for, or the station link's key, on one of its routes, from this
// machine. A request with a wrong key, or the right one anywhere else, meets
// the lock as if it carried none.
func (h *Handler) RequireSignIn(next http.Handler) http.Handler {
	locked := h.auth.Require(next)
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if h.linkMay(r) {
			next.ServeHTTP(w, r)
			return
		}
		locked.ServeHTTP(w, r)
	})
}

// linkMay says the request is the station link's. The route is chi's, already
// matched when a group's middleware runs, as for the viewer's guard: a path
// spelled another way cannot slip past.
func (h *Handler) linkMay(r *http.Request) bool {
	if h.linkToken == "" {
		return false
	}
	got, ok := strings.CutPrefix(r.Header.Get("Authorization"), "Bearer ")
	if !ok || subtle.ConstantTimeCompare([]byte(got), []byte(h.linkToken)) != 1 {
		return false
	}
	pattern := ""
	if rctx := chi.RouteContext(r.Context()); rctx != nil {
		pattern = rctx.RoutePattern()
	}
	return linkRoutes[r.Method+" "+pattern] && fromThisMachine(r)
}
