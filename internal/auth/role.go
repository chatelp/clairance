package auth

import (
	"fmt"
	"net/http"
	"strings"
)

// The two roles an account can have. An account that says none is an owner:
// every account written before roles existed -- the owner's own users.json --
// keeps doing everything it did.
const (
	// RoleOwner reads, listens, and changes the settings.
	RoleOwner = "owner"
	// RoleViewer reads: the map, the aircraft, the transcriptions. It changes
	// nothing, and is not given the audio unless server.audio_replay says so.
	RoleViewer = "viewer"
)

// NormalizeRole reads a role as written in users.json or config.toml: empty
// is owner, case and spaces do not count, and anything else is an error
// rather than a guess -- "veiwer" taken for an owner would hand a guest the
// settings, and taken for a viewer would lock the owner out of them.
func NormalizeRole(role string) (string, error) {
	switch r := strings.ToLower(strings.TrimSpace(role)); r {
	case "", RoleOwner:
		return RoleOwner, nil
	case RoleViewer:
		return RoleViewer, nil
	default:
		return "", fmt.Errorf("unknown role %q: owner or viewer", role)
	}
}

// Role is the role of whoever makes the request: the role of the signed-in
// account, owner when sign-in is off (local-only: this machine's user owns the
// server), and "" for a request with no session, which Require turns away
// before anything asks.
func (s *Service) Role(r *http.Request) string {
	if !s.Enabled() {
		return RoleOwner
	}
	// SessionFrom has checked the account still holds the session's role, so
	// the session's role is the account's. An unknown account used to fall
	// through to owner here.
	sess, ok := s.SessionFrom(r)
	if !ok {
		return ""
	}
	return sess.Role
}
