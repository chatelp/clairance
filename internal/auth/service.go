package auth

import (
	"errors"
	"fmt"
	"net/http"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"
)

// CookieName is the session cookie. Prefixed so it is obvious in a browser's
// storage inspector which application it belongs to.
const CookieName = "coatc_session"

// User is an account that may sign in.
type User struct {
	Name         string
	PasswordHash string
	Role         string // RoleOwner or RoleViewer; empty is owner
}

// Service answers "who is asking", and is the only place that says yes.
type Service struct {
	mu      sync.RWMutex
	enabled bool
	users   map[string]string // name -> password hash
	roles   map[string]string // name -> RoleOwner or RoleViewer, for every name in users
	file    *UserFile
	store   *Store
	proxies *TrustedProxies
	limiter *Limiter
	trace   *Trace

	// What enabled is decided from, kept because the choice can now change
	// while the server runs; see resolve.
	configEnabled bool
	localAllowed  bool
	localRefused  bool
}

// ErrLocalNotAllowed refuses sign-in being turned off on a server that can be
// reached from elsewhere.
var ErrLocalNotAllowed = errors.New("this server can be reached from other machines " +
	"(it is not bound to 127.0.0.1, or a proxy is declared in front of it), so it cannot run without sign-in")

// ErrNoAccount refuses sign-in being required when there is nobody to sign in as.
var ErrNoAccount = errors.New("no account exists yet: create one")

// Why SignIn refuses. The caller tells the client neither: a wrong name and a
// wrong password are one error, and the limit is said only in the trace.
var (
	ErrTooManyAttempts    = errors.New("too many attempts")
	ErrInvalidCredentials = errors.New("invalid credentials")
)

// Config is what the service needs from the configuration file.
type Config struct {
	Enabled        bool
	Users          []User
	SessionTTL     time.Duration
	TrustedProxies []string
	MaxAttempts    int
	AttemptWindow  time.Duration

	// Loopback says the server listens only on this machine's loopback address.
	// Running without sign-in is allowed only then, and only with no proxy
	// declared -- a proxy is exactly what makes a loopback-bound server
	// reachable from elsewhere.
	Loopback bool

	// Accounts created by the first-run page. config.toml is never rewritten by
	// the program -- it carries the measured tables this project reasons from,
	// in comments -- so a page that creates an account writes here instead, and
	// the two sources are merged.
	UserFile *UserFile
}

func NewService(cfg Config) (*Service, error) {
	proxies, err := NewTrustedProxies(cfg.TrustedProxies)
	if err != nil {
		return nil, fmt.Errorf("invalid trusted_proxies: %w", err)
	}
	users, roles := map[string]string{}, map[string]string{}
	for _, u := range cfg.Users {
		name := strings.TrimSpace(u.Name)
		if name == "" || u.PasswordHash == "" {
			continue
		}
		role, err := NormalizeRole(u.Role)
		if err != nil {
			return nil, fmt.Errorf("account %q in the configuration: %w", name, err)
		}
		users[name], roles[name] = u.PasswordHash, role
	}
	// The file comes second so an account created from the page can replace one
	// of the same name in the configuration -- the page is the thing someone
	// just used, and surprising them is worse than surprising the file.
	if cfg.UserFile != nil {
		for _, u := range cfg.UserFile.Users {
			if name := strings.TrimSpace(u.Name); name != "" && u.PasswordHash != "" {
				role, err := NormalizeRole(u.Role)
				if err != nil {
					return nil, fmt.Errorf("account %q in %s: %w", name, cfg.UserFile.Path(), err)
				}
				users[name], roles[name] = u.PasswordHash, role
			}
		}
	}
	if cfg.Enabled && len(users) == 0 {
		return nil, fmt.Errorf("auth.enabled is true but no account exists; create one from the setup page, or run: co-atc -add-user <name>")
	}
	max := cfg.MaxAttempts
	if max <= 0 {
		max = 8
	}
	window := cfg.AttemptWindow
	if window <= 0 {
		window = 15 * time.Minute
	}
	s := &Service{
		users:         users,
		roles:         roles,
		file:          cfg.UserFile,
		store:         NewStore(cfg.SessionTTL),
		proxies:       proxies,
		limiter:       NewLimiter(max, window),
		trace:         NewTrace(tracePath(cfg.UserFile)),
		configEnabled: cfg.Enabled,
		localAllowed:  cfg.Loopback && len(cfg.TrustedProxies) == 0,
	}
	s.resolve()
	return s, nil
}

// resolve decides whether authentication is on. Called with s.mu held -- or
// before the service is shared -- at start and after every change of choice or
// of accounts, so that there is one place where the rule is written.
func (s *Service) resolve() {
	access, fileUsers := "", false
	if s.file != nil {
		access = s.file.Access
		for _, u := range s.file.Users {
			if strings.TrimSpace(u.Name) != "" && u.PasswordHash != "" {
				fileUsers = true
			}
		}
	}
	s.localRefused = access == AccessLocal && !s.localAllowed

	switch {
	case access == AccessLocal && s.localAllowed:
		// Chosen, and this server can only be reached from this machine. The
		// accounts stay in the file for when an account is chosen again.
		s.enabled = false
	case access == AccessAccount:
		s.enabled = len(s.users) > 0
	default:
		// Never chosen -- a file from before the choice was recorded -- or
		// local-only chosen on a server that has since become reachable from
		// elsewhere: the accounts decide, as they always did.
		//
		// An account in the file turns authentication on, whatever the
		// configuration says. Creating one through the setup page *is* the act
		// of turning it on; without this, a restart left the account in place,
		// the setup page satisfied, and the server open (D37). Accounts written
		// by hand into config.toml keep obeying auth.enabled: a deliberate
		// `enabled = false` there is someone's decision, not an oversight.
		s.enabled = s.configEnabled || fileUsers
	}
}

// Enabled reads under the lock: it is asked on every request, and the answer
// can now change from the settings panel while requests are in flight.
func (s *Service) Enabled() bool {
	if s == nil {
		return false
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.enabled
}

func (s *Service) Sessions() *Store         { return s.store }
func (s *Service) Proxies() *TrustedProxies { return s.proxies }
func (s *Service) Trace() *Trace            { return s.trace }

// HasAccount says whether a name is an account's. For the trace, after a
// sign-in has been answered -- never to decide how to answer one.
func (s *Service) HasAccount(name string) bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	_, ok := s.users[strings.TrimSpace(name)]
	return ok
}

// tracePath is where the sign-in trace is kept: beside the accounts file,
// and nowhere -- in memory only -- without one.
func tracePath(f *UserFile) string {
	if f == nil || f.Path() == "" {
		return ""
	}
	return filepath.Join(filepath.Dir(f.Path()), TraceFile)
}

// SignIn checks a password and issues a session.
//
// A wrong user name and a wrong password are answered identically, and both pay the
// same Argon2id cost: telling them apart would turn the login form into a way to
// enumerate accounts.
func (s *Service) SignIn(r *http.Request, name, password string) (*Session, error) {
	key := limitKey(s.proxies.ClientIP(r))
	if !s.limiter.Allow(key) {
		return nil, ErrTooManyAttempts
	}

	s.mu.RLock()
	hash, known := s.users[strings.TrimSpace(name)]
	s.mu.RUnlock()
	if !known {
		// Verify against a throwaway hash so an unknown name costs the same time as
		// a known one.
		VerifyPassword(password, decoyHash)
		return nil, ErrInvalidCredentials
	}
	if !VerifyPassword(password, hash) {
		return nil, ErrInvalidCredentials
	}

	s.limiter.Succeeded(key)
	return s.store.Create(strings.TrimSpace(name))
}

// RetryAfter is how long the address r comes from waits before its next
// sign-in is tried: zero when it may try now. The limit is the address's,
// not an account's, so saying it tells nothing of who has one.
func (s *Service) RetryAfter(r *http.Request) time.Duration {
	return s.limiter.Wait(limitKey(s.proxies.ClientIP(r)))
}

// decoyHash is a real Argon2id hash of a value nobody knows, used only to spend the
// same time on an unknown user as on a known one.
const decoyHash = "$argon2id$v=19$m=65536,t=3,p=2$AAAAAAAAAAAAAAAAAAAAAA$kFYJ0kYOQ0aH7VbTlSJ8YZm1LcAqvJLHhWpFkYhqQ2M"

// SessionFrom returns the session a request carries, if any.
func (s *Service) SessionFrom(r *http.Request) (*Session, bool) {
	c, err := r.Cookie(CookieName)
	if err != nil {
		return nil, false
	}
	return s.store.Lookup(c.Value)
}

// SetCookie installs the session cookie.
//
// HttpOnly so no script can read it, SameSite=Strict so no other site can cause it
// to be sent, and Secure whenever the browser is actually on HTTPS -- which behind
// a proxy is known only from a forwarded header, and only from a trusted one.
func (s *Service) SetCookie(w http.ResponseWriter, r *http.Request, sess *Session) {
	http.SetCookie(w, &http.Cookie{
		Name:     CookieName,
		Value:    sess.Token,
		Path:     "/",
		HttpOnly: true,
		SameSite: http.SameSiteStrictMode,
		Secure:   s.proxies.Scheme(r) == "https",
		Expires:  sess.ExpiresAt,
	})
}

// ClearCookie removes it.
func (s *Service) ClearCookie(w http.ResponseWriter, r *http.Request) {
	http.SetCookie(w, &http.Cookie{
		Name:     CookieName,
		Value:    "",
		Path:     "/",
		HttpOnly: true,
		SameSite: http.SameSiteStrictMode,
		Secure:   s.proxies.Scheme(r) == "https",
		MaxAge:   -1,
	})
}

// Require rejects a request that carries no valid session. It is a no-op when
// authentication is disabled, which is the default of the published configuration:
// someone cloning the repository must not find themselves locked out of their own
// receiver.
func (s *Service) Require(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !s.Enabled() {
			next.ServeHTTP(w, r)
			return
		}
		if _, ok := s.SessionFrom(r); !ok {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusUnauthorized)
			w.Write([]byte(`{"error":"authentication required"}`))
			return
		}
		next.ServeHTTP(w, r)
	})
}

// NeedsSetup reports that the first-run question has no answer: no account
// exists anywhere, and local-only has not been chosen -- or was, but this
// server can now be reached from elsewhere. It stays true until someone
// answers, and an answer is kept across restarts.
func (s *Service) NeedsSetup() bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.needsSetup()
}

func (s *Service) needsSetup() bool {
	local := s.file != nil && s.file.Access == AccessLocal && s.localAllowed
	return len(s.users) == 0 && !local
}

// LocalAllowed says whether this server may run without sign-in.
func (s *Service) LocalAllowed() bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.localAllowed
}

// LocalRefused reports a recorded local-only choice that is not honoured,
// because the server has since become reachable from elsewhere. The caller
// says so in the log; the accounts, if any, are in force meanwhile.
func (s *Service) LocalRefused() bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.localRefused
}

// ChooseLocal turns sign-in off and records it, so the first-run question is
// not asked again. Any accounts stay in the file, unused: choosing an account
// again brings them back as they were.
func (s *Service) ChooseLocal() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if !s.localAllowed {
		return ErrLocalNotAllowed
	}
	if s.file == nil {
		return fmt.Errorf("no accounts file configured")
	}
	if err := s.file.SetAccess(AccessLocal); err != nil {
		return err
	}
	// Sessions from before would otherwise outlive the switch: requiring
	// sign-in again later would find them still valid and ask nobody to sign
	// in -- which is what the owner met on the first try.
	s.store.RevokeAll()
	s.resolve()
	return nil
}

// ChooseAccount turns sign-in back on with the accounts that exist, and records
// it. With none it refuses: an account has to be created first, with AddUser.
func (s *Service) ChooseAccount() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if len(s.users) == 0 {
		return ErrNoAccount
	}
	if s.file == nil {
		return fmt.Errorf("no accounts file configured")
	}
	if err := s.file.SetAccess(AccessAccount); err != nil {
		return err
	}
	s.resolve()
	return nil
}

// AccessState is what the settings panel shows: how the server is reached now,
// whether local-only is possible, and who could sign in. Names only.
type AccessState struct {
	Mode         string   `json:"mode"` // "local", "account", or "none" before the first answer
	LocalAllowed bool     `json:"local_allowed"`
	Accounts     []string `json:"accounts"`
}

func (s *Service) Access() AccessState {
	s.mu.RLock()
	defer s.mu.RUnlock()
	st := AccessState{LocalAllowed: s.localAllowed, Accounts: []string{}}
	for name := range s.users {
		st.Accounts = append(st.Accounts, name)
	}
	sort.Strings(st.Accounts)
	switch {
	case s.enabled:
		st.Mode = AccessAccount
	case s.needsSetup():
		st.Mode = "none"
	default:
		st.Mode = AccessLocal
	}
	return st
}

// AddUser hashes a password, writes the account to the accounts file, and turns
// authentication on -- recording that choice, so a server that was local-only
// becomes one that requires sign-in. The password is never stored, logged or
// returned.
//
// It refuses once an account exists, even unused while local-only: this is the
// path that works without being signed in, and it must stop being able to
// create accounts the moment there is someone to sign in as.
func (s *Service) AddUser(name, password string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if len(s.users) > 0 {
		return fmt.Errorf("an account already exists; sign in, or use co-atc -add-user")
	}
	if s.file == nil {
		return fmt.Errorf("no accounts file configured")
	}
	if len(strings.TrimSpace(name)) == 0 {
		return fmt.Errorf("choose a user name")
	}
	if len([]rune(password)) < 10 {
		// Ten because this may end up reachable from outside a LAN, and the
		// rate limiter buys time rather than safety.
		return fmt.Errorf("the password must be at least 10 characters")
	}

	hash, err := HashPassword(password)
	if err != nil {
		return fmt.Errorf("hash password: %w", err)
	}
	// One write for both: an account on disk with local-only still recorded
	// would come back at the next start as an unprotected server.
	prev := s.file.Access
	s.file.Access = AccessAccount
	if err := s.file.Add(strings.TrimSpace(name), hash); err != nil {
		s.file.Access = prev
		return err
	}
	// The first account owns the server: it is the one that set it up.
	s.users[strings.TrimSpace(name)] = hash
	s.roles[strings.TrimSpace(name)] = RoleOwner
	s.resolve()
	return nil
}

// AccountsFile is where accounts created from the page are kept.
func (s *Service) AccountsFile() string {
	if s.file == nil {
		return ""
	}
	return s.file.Path()
}
