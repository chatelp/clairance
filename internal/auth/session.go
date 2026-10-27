package auth

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"sync"
	"time"
)

// Session is one signed-in browser.
type Session struct {
	// Token is what the cookie carries. Known to a session read back from disk
	// only once its browser has shown it again: the file holds its hash.
	Token     string
	User      string
	Role      string // the account's role when the session was issued
	CreatedAt time.Time
	ExpiresAt time.Time
}

// SessionFile is where the sessions are kept, beside the accounts file.
const SessionFile = "sessions.json"

// sessionSaveEvery bounds how often the sliding expiry alone writes the file.
// Signing in and out write at once; a session merely in use moves its expiry
// on every request, and writing that each time would be a disk write per
// request for a date that changes by seconds. At most a minute of sliding is
// lost to a crash, on an expiry of thirty days. The stop writes the rest.
const sessionSaveEvery = time.Minute

// Store keeps the sessions in force.
//
// Kept on disk since 26/10 (fiche S): production restarts often, and each
// restart used to sign everyone out -- a guest had to type the password again
// for something the owner did. The file holds no token, only its SHA-256, so
// reading it gives nothing a browser could present. To sign everyone out, which
// a restart no longer does: stop the server and delete the file, or remove the
// account (its sessions are dropped at the next start).
type Store struct {
	mu       sync.Mutex
	sessions map[string]*Session // by the token's hash
	ttl      time.Duration
	now      func() time.Time

	path     string // "" keeps them in memory only
	dirty    bool   // an expiry slid since the last write
	savedAt  time.Time
	restored int   // read back at start
	dropped  int   // read back and set aside: expired, or their account changed
	loadErr  error // the file was there and could not be read
}

func NewStore(ttl time.Duration) *Store {
	if ttl <= 0 {
		ttl = 30 * 24 * time.Hour
	}
	return &Store{sessions: map[string]*Session{}, ttl: ttl, now: time.Now}
}

// tokenHash is the key a session is kept under, in memory and on disk. Plain
// SHA-256 and no salt: the token is 256 random bits, so there is no dictionary
// to try, and a slow hash would only slow every request.
func tokenHash(token string) string {
	sum := sha256.Sum256([]byte(token))
	return hex.EncodeToString(sum[:])
}

// Create issues a session for a user. The token is 256 bits of randomness and
// nothing else: it carries no claims, so there is nothing in it to forge and
// revoking it is a map delete rather than a blocklist.
func (s *Store) Create(user, role string) (*Session, error) {
	raw := make([]byte, 32)
	if _, err := rand.Read(raw); err != nil {
		return nil, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	now := s.now()
	sess := &Session{
		Token:     base64.RawURLEncoding.EncodeToString(raw),
		User:      user,
		Role:      role,
		CreatedAt: now,
		ExpiresAt: now.Add(s.ttl),
	}
	s.sessions[tokenHash(sess.Token)] = sess
	s.pruneLocked(now)
	// Written at once: a sign-in lost to a restart is the very thing this
	// file is for. A write that fails leaves the session in memory, good
	// until the next restart, rather than refusing someone who typed the
	// right password.
	s.saveLocked(now)
	cp := *sess
	return &cp, nil
}

// Lookup returns the session for a token and slides its expiry forward. A session
// in daily use should not expire under its user; one abandoned for a month should.
func (s *Store) Lookup(token string) (*Session, bool) {
	if token == "" {
		return nil, false
	}
	key := tokenHash(token)

	s.mu.Lock()
	defer s.mu.Unlock()
	now := s.now()
	sess, ok := s.sessions[key]
	if !ok {
		return nil, false
	}
	if now.After(sess.ExpiresAt) {
		delete(s.sessions, key)
		s.dirty = true
		return nil, false
	}
	sess.Token = token // a session read back from disk learns it here
	sess.ExpiresAt = now.Add(s.ttl)
	s.dirty = true
	if now.Sub(s.savedAt) >= sessionSaveEvery {
		s.saveLocked(now)
	}
	cp := *sess
	return &cp, true
}

// Revoke ends one session. Written at once: a sign-out that a restart undid
// would hand the browser back a session its user ended.
func (s *Store) Revoke(token string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.sessions[tokenHash(token)]; ok {
		delete(s.sessions, tokenHash(token))
		s.saveLocked(s.now())
	}
}

// RevokeUser ends every session of one user -- what you reach for when a device is
// lost rather than merely closed.
func (s *Store) RevokeUser(user string) int {
	s.mu.Lock()
	defer s.mu.Unlock()
	n := 0
	for key, sess := range s.sessions {
		if sess.User == user {
			delete(s.sessions, key)
			n++
		}
	}
	if n > 0 {
		s.saveLocked(s.now())
	}
	return n
}

// RevokeAll ends every session, for when sign-in stops meaning anything.
func (s *Store) RevokeAll() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	n := len(s.sessions)
	s.sessions = make(map[string]*Session)
	s.saveLocked(s.now())
	return n
}

// Count reports how many sessions are in force.
func (s *Store) Count() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.pruneLocked(s.now())
	return len(s.sessions)
}

func (s *Store) pruneLocked(now time.Time) {
	for key, sess := range s.sessions {
		if now.After(sess.ExpiresAt) {
			delete(s.sessions, key)
			s.dirty = true
		}
	}
}

// sessionRecord is one session as the file holds it. Hash, never the token.
type sessionRecord struct {
	Hash      string    `json:"hash"`
	User      string    `json:"user"`
	Role      string    `json:"role"`
	CreatedAt time.Time `json:"created_at"`
	ExpiresAt time.Time `json:"expires_at"`
}

type sessionFile struct {
	Sessions []sessionRecord `json:"sessions"`
}

// open reads back the sessions kept at path, and keeps them there from then
// on. A session is set aside when it has expired, or when keep says its
// account no longer exists or no longer has the role it was issued with. A
// file that cannot be read is not an error for the server: it starts with no
// session, LoadError says why, and the next write replaces the file.
func (s *Store) open(path string, keep func(user, role string) bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.path = path
	if path == "" {
		return
	}
	raw, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return
	}
	if err != nil {
		s.loadErr = fmt.Errorf("read %s: %w", path, err)
		return
	}
	var f sessionFile
	if err := json.Unmarshal(raw, &f); err != nil {
		s.loadErr = fmt.Errorf("parse %s: %w", path, err)
		return
	}
	now := s.now()
	for _, r := range f.Sessions {
		if len(r.Hash) != sha256.Size*2 || r.User == "" || now.After(r.ExpiresAt) || !keep(r.User, r.Role) {
			s.dropped++
			continue
		}
		s.sessions[r.Hash] = &Session{User: r.User, Role: r.Role, CreatedAt: r.CreatedAt, ExpiresAt: r.ExpiresAt}
		s.restored++
	}
	if s.dropped > 0 {
		// The file must not keep a session the start set aside: one of an
		// account removed by hand would otherwise stay on disk until the
		// next sign-in rewrites it.
		s.saveLocked(now)
	} else {
		s.savedAt = now
	}
}

// Save writes the sessions if their expiries moved since the last write; the
// server calls it as it stops, once nothing serves requests any more.
func (s *Store) Save() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if !s.dirty {
		return nil
	}
	return s.saveLocked(s.now())
}

// saveLocked writes the whole table through a temporary file: a half-written
// file would read back as no session at all. A write that fails leaves the
// table dirty, so it is tried again within the minute and at the stop, which
// says the error -- the callers in between have nobody to tell.
func (s *Store) saveLocked(now time.Time) error {
	s.dirty = true
	if s.path == "" {
		s.dirty = false
		return nil
	}
	f := sessionFile{Sessions: make([]sessionRecord, 0, len(s.sessions))}
	for key, sess := range s.sessions {
		f.Sessions = append(f.Sessions, sessionRecord{Hash: key, User: sess.User, Role: sess.Role,
			CreatedAt: sess.CreatedAt.UTC(), ExpiresAt: sess.ExpiresAt.UTC()})
	}
	s.savedAt = now // tried: a disk that refuses is tried again in a minute, not on every request
	b, err := json.MarshalIndent(f, "", "  ")
	if err != nil {
		return err
	}
	tmp := s.path + ".tmp"
	// 0600: a hash gives no way in, but who signed in and until when is
	// nobody else's business on this machine.
	if err := os.WriteFile(tmp, append(b, '\n'), 0o600); err != nil {
		return fmt.Errorf("write %s: %w", tmp, err)
	}
	if err := os.Rename(tmp, s.path); err != nil {
		return fmt.Errorf("install %s: %w", s.path, err)
	}
	s.dirty = false
	return nil
}

// Path is the sessions file, empty when they are kept in memory only.
func (s *Store) Path() string { return s.path }

// Restored says what the start read back: the sessions kept, the ones set
// aside, and why the file could not be read when it could not.
func (s *Store) Restored() (kept, dropped int, err error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.restored, s.dropped, s.loadErr
}
