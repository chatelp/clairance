package api

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"sync"

	"github.com/go-chi/chi/v5"
	"github.com/yegors/co-atc/internal/auth"
	"github.com/yegors/co-atc/pkg/logger"
)

// The owner's panel of accounts (D83, 27/10): who may sign in, what they have
// open, and the means to add one, give one a new password, cut one off or
// remove one, while the server runs. Reading is in ownerRoutes; writing is the
// owner's anyway, a viewer writing nothing (ViewerGuard).

// liveStreams holds, per account, a way to end each audio stream it has open:
// cutting an account off ends its sessions, and a stream already playing would
// otherwise play on until the page closed it.
type liveStreams struct {
	mu     sync.Mutex
	next   int
	byUser map[string]map[int]context.CancelFunc
}

func (l *liveStreams) add(user string, cancel context.CancelFunc) (remove func()) {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.byUser == nil {
		l.byUser = map[string]map[int]context.CancelFunc{}
	}
	if l.byUser[user] == nil {
		l.byUser[user] = map[int]context.CancelFunc{}
	}
	l.next++
	id := l.next
	l.byUser[user][id] = cancel
	return func() {
		l.mu.Lock()
		defer l.mu.Unlock()
		delete(l.byUser[user], id)
		if len(l.byUser[user]) == 0 {
			delete(l.byUser, user)
		}
	}
}

func (l *liveStreams) cut(user string) int {
	l.mu.Lock()
	defer l.mu.Unlock()
	n := len(l.byUser[user])
	for _, cancel := range l.byUser[user] {
		cancel()
	}
	return n
}

// signedIn is the account a request is made by, "" when sign-in is off.
func (h *Handler) signedIn(r *http.Request) (*auth.Session, bool) {
	if h.auth == nil || !h.auth.Enabled() {
		return nil, false
	}
	return h.auth.SessionFrom(r)
}

// cuttable gives a long request -- an audio stream -- a context the owner can
// end by cutting its account off.
func (h *Handler) cuttable(r *http.Request) (*http.Request, func()) {
	sess, ok := h.signedIn(r)
	if !ok {
		return r, func() {}
	}
	ctx, cancel := context.WithCancel(r.Context())
	remove := h.live.add(sess.User, cancel)
	return r.WithContext(ctx), func() { remove(); cancel() }
}

// closeLive ends what an account still has open: its WebSocket and its streams.
func (h *Handler) closeLive(user string) (sockets, streams int) {
	h.usage.Cut(user) // what closes now ends by the cut, in the journal
	if h.wsServer != nil {
		sockets = h.wsServer.CloseTagged(user)
	}
	return sockets, h.live.cut(user)
}

// GetAccounts lists the accounts, for the owner.
func (h *Handler) GetAccounts(w http.ResponseWriter, r *http.Request) {
	you := ""
	if sess, ok := h.signedIn(r); ok {
		you = sess.User
	}
	w.Header().Set("Cache-Control", "no-store")
	WriteJSON(w, http.StatusOK, map[string]any{
		"accounts":     h.auth.Accounts(),
		"you":          you,
		"sign_in":      h.auth.Enabled(),
		"min_password": 10,
	})
}

type accountRequest struct {
	Name     string `json:"name"`
	Password string `json:"password"`
	Role     string `json:"role"`
}

func readAccountRequest(w http.ResponseWriter, r *http.Request) (accountRequest, bool) {
	var req accountRequest
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 4096)).Decode(&req); err != nil {
		WriteJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request"})
		return req, false
	}
	return req, true
}

// PostAccount creates an account. The password goes to the hash and nowhere
// else: not the log, not the trace, not the answer.
func (h *Handler) PostAccount(w http.ResponseWriter, r *http.Request) {
	req, ok := readAccountRequest(w, r)
	if !ok {
		return
	}
	name := strings.TrimSpace(req.Name)
	if err := h.auth.CreateAccount(name, req.Password, req.Role); err != nil {
		h.accountRefused(w, err)
		return
	}
	h.accountChanged(r, auth.EventAccountCreated, name)
	WriteJSON(w, http.StatusCreated, map[string]any{"accounts": h.auth.Accounts()})
}

// DeleteAccount removes an account, ends its sessions and closes what it has
// open.
func (h *Handler) DeleteAccount(w http.ResponseWriter, r *http.Request) {
	name := chi.URLParam(r, "name")
	if err := h.auth.RemoveAccount(name, h.you(r)); err != nil {
		h.accountRefused(w, err)
		return
	}
	h.closeLive(name)
	h.accountChanged(r, auth.EventAccountRemoved, name)
	WriteJSON(w, http.StatusOK, map[string]any{"accounts": h.auth.Accounts()})
}

// PutAccountPassword gives an account a new password. Its other browsers are
// signed out; the one asking stays signed in when the account is its own.
func (h *Handler) PutAccountPassword(w http.ResponseWriter, r *http.Request) {
	name := chi.URLParam(r, "name")
	req, ok := readAccountRequest(w, r)
	if !ok {
		return
	}
	keep := ""
	if sess, ok := h.signedIn(r); ok && sess.User == name {
		keep = sess.Token
	}
	if err := h.auth.SetPassword(name, req.Password, keep); err != nil {
		h.accountRefused(w, err)
		return
	}
	if keep == "" {
		h.closeLive(name)
	}
	h.accountChanged(r, auth.EventPasswordChanged, name)
	WriteJSON(w, http.StatusOK, map[string]any{"accounts": h.auth.Accounts()})
}

// PostAccountCut ends an account's sessions and closes what it has open. Its
// password is kept: signing in again works, which a new password stops.
func (h *Handler) PostAccountCut(w http.ResponseWriter, r *http.Request) {
	name := chi.URLParam(r, "name")
	sessions, err := h.auth.CutAccess(name, h.you(r))
	if err != nil {
		h.accountRefused(w, err)
		return
	}
	sockets, streams := h.closeLive(name)
	h.accountChanged(r, auth.EventAccessCut, name)
	WriteJSON(w, http.StatusOK, map[string]any{
		"accounts": h.auth.Accounts(),
		"ended":    map[string]int{"sessions": sessions, "sockets": sockets, "streams": streams},
	})
}

func (h *Handler) you(r *http.Request) string {
	if sess, ok := h.signedIn(r); ok {
		return sess.User
	}
	return ""
}

// accountChanged writes the owner's change to the trace and the log.
func (h *Handler) accountChanged(r *http.Request, event, name string) {
	ev := h.signInEvent(r, event)
	ev.User, ev.By = name, h.you(r)
	h.trace(ev)
	h.logger.Info("Account changed from the owner's panel",
		logger.String("event", event),
		logger.String("user", name),
		logger.String("by", ev.By),
		logger.String("from", ev.From))
}

// accountRefused says why, in the words of internal/auth.
func (h *Handler) accountRefused(w http.ResponseWriter, err error) {
	status := http.StatusInternalServerError
	switch {
	case errors.Is(err, auth.ErrNoSuchAccount):
		status = http.StatusNotFound
	case errors.Is(err, auth.ErrAccountExists), errors.Is(err, auth.ErrConfigAccount),
		errors.Is(err, auth.ErrLastOwner), errors.Is(err, auth.ErrYourself), errors.Is(err, auth.ErrNoAccountsFile):
		status = http.StatusConflict
	case errors.Is(err, auth.ErrBadAccountName), errors.Is(err, auth.ErrPasswordTooShort),
		strings.HasPrefix(err.Error(), "unknown role"):
		status = http.StatusBadRequest
	}
	if status == http.StatusInternalServerError {
		h.logger.Error("Account change failed", logger.Error(err))
		WriteJSON(w, status, map[string]string{"error": "the accounts file could not be written"})
		return
	}
	WriteJSON(w, status, map[string]string{"error": err.Error()})
}
