package api

import (
	"encoding/json"
	"errors"
	"net"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/yegors/co-atc/internal/auth"
	"github.com/yegors/co-atc/internal/config"
	"github.com/yegors/co-atc/pkg/logger"
)

type loginRequest struct {
	Name     string `json:"name"`
	Password string `json:"password"`
}

// GetAuthStatus tells the page whether anyone needs to sign in, and who is signed
// in. It is deliberately reachable without a session: the login form has to know
// whether to show itself.
func (h *Handler) GetAuthStatus(w http.ResponseWriter, r *http.Request) {
	out := map[string]interface{}{"enabled": h.auth.Enabled(), "authenticated": !h.auth.Enabled()}
	if h.auth.Enabled() {
		if sess, ok := h.auth.SessionFrom(r); ok {
			out["authenticated"] = true
			out["user"] = sess.User
			out["expires_at"] = sess.ExpiresAt
		}
	}
	// What the page may offer: "owner" or "viewer", and owner when sign-in is
	// off. Absent for a visitor who has not signed in.
	if role := h.auth.Role(r); role != "" {
		out["role"] = role
	}
	WriteJSON(w, http.StatusOK, out)
}

// PostLogin exchanges a password for a session cookie.
func (h *Handler) PostLogin(w http.ResponseWriter, r *http.Request) {
	if !h.auth.Enabled() {
		WriteJSON(w, http.StatusOK, map[string]interface{}{"authenticated": true})
		return
	}
	var req loginRequest
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 4096)).Decode(&req); err != nil {
		http.Error(w, "invalid request", http.StatusBadRequest)
		return
	}

	sess, err := h.auth.SignIn(r, req.Name, req.Password)
	if err != nil {
		// The client is told only that it failed. Which of the name or the password
		// was wrong, and whether the account exists at all, stay unsaid: a login
		// form that distinguishes them is a way to enumerate accounts. The trace
		// says it, to the owner; and names the account only when there is one --
		// a password typed in the name field is not written down.
		ev := h.signInEvent(r, auth.EventRefused)
		if h.auth.HasAccount(req.Name) {
			ev.User, ev.Reason = strings.TrimSpace(req.Name), auth.ReasonWrongPassword
		} else {
			ev.Reason = auth.ReasonUnknownName
		}
		if errors.Is(err, auth.ErrTooManyAttempts) {
			ev.Reason = auth.ReasonTooManyAttempts
		}
		h.trace(ev)
		h.logger.Warn("Failed sign-in",
			logger.String("user", ev.User),
			logger.String("reason", ev.Reason),
			logger.String("from", ev.From),
			logger.String("agent", ev.Agent))
		time.Sleep(250 * time.Millisecond)
		if errors.Is(err, auth.ErrTooManyAttempts) {
			// Said as such, and for how long: the page tells the user to
			// wait rather than to try another password (login.js), which
			// would only be refused again.
			w.Header().Set("Retry-After", strconv.Itoa(retryAfterSeconds(h.auth.RetryAfter(r))))
			WriteJSON(w, http.StatusTooManyRequests, map[string]string{"error": "too many attempts"})
			return
		}
		WriteJSON(w, http.StatusUnauthorized, map[string]string{"error": "invalid credentials"})
		return
	}

	h.auth.SetCookie(w, r, sess)
	ev := h.signInEvent(r, auth.EventSignedIn)
	ev.User = sess.User
	h.trace(ev)
	h.logger.Info("Signed in",
		logger.String("user", sess.User),
		logger.String("from", ev.From),
		logger.String("via", ev.Via),
		logger.String("agent", ev.Agent))
	WriteJSON(w, http.StatusOK, map[string]interface{}{
		"authenticated": true, "user": sess.User, "expires_at": sess.ExpiresAt,
	})
}

// retryAfterSeconds is a wait in whole seconds, rounded up, one at least: a
// Retry-After of 0 would say to try again at once.
func retryAfterSeconds(d time.Duration) int {
	return max(1, int((d+time.Second-1)/time.Second))
}

// SaveSessions writes the sessions' last expiries, for the server's stop:
// sign-ins and sign-outs are already on disk, the sliding expiry is written
// once a minute at most.
func (h *Handler) SaveSessions() error {
	return h.auth.Sessions().Save()
}

// PostLogout ends the session that made the request.
func (h *Handler) PostLogout(w http.ResponseWriter, r *http.Request) {
	if sess, ok := h.auth.SessionFrom(r); ok {
		h.auth.Sessions().Revoke(sess.Token)
		ev := h.signInEvent(r, auth.EventSignedOut)
		ev.User = sess.User
		h.trace(ev)
		h.logger.Info("Signed out", logger.String("user", sess.User), logger.String("from", ev.From))
	}
	h.auth.ClearCookie(w, r)
	WriteJSON(w, http.StatusOK, map[string]interface{}{"authenticated": false})
}

// signInEvent is the part of a trace event the request says: who, from
// where, through which proxy, with which browser.
func (h *Handler) signInEvent(r *http.Request, event string) auth.SignInEvent {
	ev := auth.SignInEvent{Event: event, From: h.auth.Proxies().ClientIP(r), Agent: r.UserAgent()}
	if h.auth.Proxies().Forwarded(r) {
		if host, _, err := net.SplitHostPort(r.RemoteAddr); err == nil {
			ev.Via = host
		}
	}
	return ev
}

// trace records an event; a trace that cannot be written is said in the log,
// and the event is kept in memory all the same.
func (h *Handler) trace(ev auth.SignInEvent) {
	if err := h.auth.Trace().Record(ev); err != nil {
		h.logger.Warn("Could not write the sign-in trace", logger.Error(err))
	}
}

// GetSignIns is the sign-in trace, the newest first: the last 100, or
// ?limit= up to the 500 held. The owner's only, like the settings (see
// ownerRoutes): who signed in from where is not a viewer's to read.
func (h *Handler) GetSignIns(w http.ResponseWriter, r *http.Request) {
	n := 100
	if v, err := strconv.Atoi(r.URL.Query().Get("limit")); err == nil && v > 0 {
		n = v
	}
	w.Header().Set("Cache-Control", "no-store")
	WriteJSON(w, http.StatusOK, map[string]any{"signins": h.auth.Trace().Recent(n)})
}

// ViewerGuard keeps a viewer account to reading, behind the sign-in lock: GET
// and HEAD everywhere except the audio unless server.audio_replay = "all",
// and nothing that writes -- not the
// settings, not the sources, not the access mode. Signing out is outside the
// lock and stays open to everyone. An owner, and everyone when sign-in is
// off, goes through untouched.
func (h *Handler) ViewerGuard(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if h.auth.Role(r) == auth.RoleViewer && !viewerMay(r, h.audioForViewers()) {
			WriteJSON(w, http.StatusForbidden, map[string]string{"error": "this account is read-only"})
			return
		}
		next.ServeHTTP(w, r)
	})
}

// audioForViewers is server.audio_replay = "all". Read from the configuration
// at each request; empty, as in a configuration built by hand, is "owner".
func (h *Handler) audioForViewers() bool {
	return h.config != nil && h.config.Server.AudioReplay == config.AudioReplayAll
}

// audioRoutes are the routes that hand out the receiver's audio: the live
// stream and the archived clip of a transmission. A viewer is not given them
// unless server.audio_replay says so: rebroadcasting what the receiver hears
// is the owner's call.
var audioRoutes = map[string]bool{
	"/api/v1/stream/{id}":               true,
	"/api/v1/transcriptions/{id}/audio": true,
}

// ownerRoutes are routes that only read and are the owner's all the same:
// the sign-in trace says who came in, from where, with what.
var ownerRoutes = map[string]bool{
	"/api/v1/signins":  true,
	"/api/v1/accounts": true,
	"/api/v1/usage":    true, // what each account did (D84)
}

// viewerMay says whether a viewer may make this request. The route is read
// from chi's routing, already done when a group's middleware runs, rather
// than from the path, so that a path spelled another way cannot slip past.
func viewerMay(r *http.Request, audio bool) bool {
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		return false
	}
	pattern := ""
	if rctx := chi.RouteContext(r.Context()); rctx != nil {
		pattern = rctx.RoutePattern()
	}
	if ownerRoutes[pattern] {
		return false
	}
	return audio || !audioRoutes[pattern]
}

// Auth exposes the service so the router can hang its middleware on it. The
// service is never nil: a disabled one is built when no account is configured, and
// it lets every request through.
func (h *Handler) Auth() *auth.Service { return h.auth }
