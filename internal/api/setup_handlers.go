package api

import (
	"encoding/json"
	"errors"
	"fmt"

	"github.com/go-chi/chi/v5"
	"io"
	"net/http"

	"github.com/yegors/co-atc/internal/auth"
	"github.com/yegors/co-atc/pkg/logger"
)

// The first-run page. Upstream ships with no authentication at all and a README
// saying never to expose it; this is the other half of changing that -- a way to
// answer the question rather than a paragraph telling you to edit TOML.
//
// No setup token, deliberately. Bound to 127.0.0.1, reaching this page already
// means being on the machine, which is the same trust boundary as the terminal
// where you would have run -add-user. A token would protect nothing it does not
// already protect. Bound anywhere else, the server refuses to start without an
// account at all -- see cmd/server/main.go -- so this page is never served to a
// network.

type SetupState struct {
	Needed       bool   `json:"needed"`        // the first-run question has no answer yet
	LocalOnly    bool   `json:"local_only"`    // the server is bound to loopback
	LocalAllowed bool   `json:"local_allowed"` // ...and no proxy is declared in front of it
	Host         string `json:"host,omitempty"`
	File         string `json:"file,omitempty"` // where an account would be written
}

// GetSetupStatus says whether anything needs setting up. Public: the page has to
// be able to ask before anyone can sign in.
//
// The bind address and the accounts file's path are said only while the
// question is open, which is when the page shows them: once set up, this
// route answers anyone who reaches the server, and the path names the
// owner's home directory.
func (h *Handler) GetSetupStatus(w http.ResponseWriter, r *http.Request) {
	st := SetupState{
		Needed:       h.auth.NeedsSetup(),
		LocalOnly:    isLoopback(h.config.Server.Host),
		LocalAllowed: h.auth.LocalAllowed(),
	}
	if st.Needed {
		st.Host, st.File = h.config.Server.Host, h.auth.AccountsFile()
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(st)
}

// PostSetup answers the first-run question: local only, or reachable with an
// account. It works exactly once, and only while no account exists.
func (h *Handler) PostSetup(w http.ResponseWriter, r *http.Request) {
	if !h.auth.NeedsSetup() {
		http.Error(w, "already set up", http.StatusConflict)
		return
	}
	// Reaching this page means being on the machine only when nothing stands
	// between the two. A server bound to 127.0.0.1 behind a proxy on the same
	// machine is reached from wherever the proxy serves: the first visitor
	// would have made the owner's account, or -- the proxy undeclared --
	// chosen to run without sign-in for the whole Internet.
	if forwardedByAProxy(r) {
		h.logger.Warn("Refused the first-run page through a proxy",
			logger.String("from", h.auth.Proxies().ClientIP(r)))
		http.Error(w, fmt.Sprintf("Set this server up from the machine it runs on (http://127.0.0.1:%d/), "+
			"or create an account there with: co-atc -add-user <name>", h.config.Server.Port), http.StatusForbidden)
		return
	}

	var body struct {
		Mode     string `json:"mode"` // "local" or "account"
		Username string `json:"username"`
		Password string `json:"password"`
	}
	if err := json.NewDecoder(io.LimitReader(r.Body, 4096)).Decode(&body); err != nil {
		http.Error(w, "invalid request: "+err.Error(), http.StatusBadRequest)
		return
	}

	switch body.Mode {
	case "local":
		// Staying without authentication is a legitimate answer on a server only
		// this machine can reach, and recording the choice is what makes it a
		// choice rather than an oversight. Until it was recorded, the page asked
		// again after every reload -- and there was no way past it but an account.
		if err := h.auth.ChooseLocal(); err != nil {
			http.Error(w, err.Error(), accessErrorStatus(err))
			return
		}
		h.logger.Info("Setup: staying local-only, no authentication",
			logger.String("host", h.config.Server.Host),
			logger.String("file", h.auth.AccountsFile()))
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"mode": "local"})

	case "account":
		if err := h.auth.AddUser(body.Username, body.Password); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		h.logger.Info("Setup: account created, authentication is on",
			logger.String("user", body.Username),
			logger.String("file", h.auth.AccountsFile()))
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"mode": "account", "user": body.Username})

	default:
		http.Error(w, `mode must be "local" or "account"`, http.StatusBadRequest)
	}
}

// PutAccess switches between local-only and sign-in from the settings panel,
// either way, as often as wanted:
//
//	{"mode": "local"}                                sign-in off; accounts kept, unused
//	{"mode": "account"}                              sign-in back on, with those accounts
//	{"mode": "account", "username": …, "password": …} sign-in on, creating the first account
//
// It sits in the authenticated group, so turning sign-in off takes a session
// whenever sign-in is on. Turning it on from local-only takes none: whoever
// reaches a local-only server is on the machine already.
func (h *Handler) PutAccess(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Mode     string `json:"mode"`
		Username string `json:"username"`
		Password string `json:"password"`
	}
	if err := json.NewDecoder(io.LimitReader(r.Body, 4096)).Decode(&body); err != nil {
		http.Error(w, "invalid request: "+err.Error(), http.StatusBadRequest)
		return
	}

	var err error
	switch body.Mode {
	case auth.AccessLocal:
		err = h.auth.ChooseLocal()
	case auth.AccessAccount:
		if body.Username != "" || body.Password != "" {
			err = h.auth.AddUser(body.Username, body.Password)
		} else {
			err = h.auth.ChooseAccount()
		}
	default:
		http.Error(w, `mode must be "local" or "account"`, http.StatusBadRequest)
		return
	}
	if err != nil {
		http.Error(w, err.Error(), accessErrorStatus(err))
		return
	}

	st := h.auth.Access()
	h.logger.Info("Access changed from the settings panel",
		logger.String("mode", st.Mode), logger.Int("accounts", len(st.Accounts)))
	WriteJSON(w, http.StatusOK, st)
}

// accessErrorStatus tells a refusal -- this server cannot run open, or there is
// nobody to sign in as -- from a malformed request.
func accessErrorStatus(err error) int {
	if errors.Is(err, auth.ErrLocalNotAllowed) || errors.Is(err, auth.ErrNoAccount) {
		return http.StatusConflict
	}
	return http.StatusBadRequest
}

// isLoopback reports whether a bind address can only be reached from this
// machine. An empty host means every interface, which is the dangerous default
// to get wrong.
func isLoopback(host string) bool {
	switch host {
	case "127.0.0.1", "::1", "localhost":
		return true
	}
	return false
}

// PutFrequencyLabel records what a frequency is currently carrying.
//
// Upstream models a frequency as one channel with one name and one number in
// MHz, which is true of a channel and false of a mix -- and what a mix carries
// can change while the server runs. co-atc cannot know when that happens, and
// should not learn one station's way of switching groups. So the mechanism is
// here and the policy stays outside: whoever changes what is broadcast says so,
// with whatever they already use.
//
//	curl -X PUT -d '{"label":"CDG approaches"}' .../api/v1/frequencies/aero-melange/label
//
// Behind authentication, like the other writes: this changes what every viewer
// is told they are listening to.
func (h *Handler) PutFrequencyLabel(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")

	var body struct {
		Label string `json:"label"` // empty restores the configured name
	}
	if err := json.NewDecoder(io.LimitReader(r.Body, 1024)).Decode(&body); err != nil {
		http.Error(w, "invalid request: "+err.Error(), http.StatusBadRequest)
		return
	}
	if err := h.frequenciesService.SetLabel(id, body.Label); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	h.logger.Info("Frequency label set from outside",
		logger.String("id", id), logger.String("label", body.Label))

	freq, _ := h.frequenciesService.GetFrequencyByID(id)
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(freq)
}
