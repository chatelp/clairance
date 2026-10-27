package auth

import (
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"
	"unicode"
)

// Accounts managed from the page, by the owner (D83, 27/10): listed, created,
// given a new password, cut off, removed -- while the server runs, where they
// used to change only through co-atc -add-user and a restart.
//
// Only the accounts of the accounts file can be changed. Those written by hand
// into config.toml are listed and left alone: the program never rewrites that
// file (see UserFile).

// Why an account change is refused. The handlers say them as they are.
var (
	ErrAccountExists    = errors.New("an account with this name already exists")
	ErrNoSuchAccount    = errors.New("no account with this name")
	ErrConfigAccount    = errors.New("this account is written in config.toml: change it there")
	ErrLastOwner        = errors.New("this is the last owner account: there would be nobody to manage the others")
	ErrYourself         = errors.New("not your own account: sign out instead")
	ErrNoAccountsFile   = errors.New("no accounts file configured")
	ErrBadAccountName   = errors.New("a name is 1 to 32 letters, digits, dots, dashes or underscores")
	ErrPasswordTooShort = errors.New("the password must be at least 10 characters")
)

// minPassword is the shortest password accepted, as on the first-run page.
const minPassword = 10

// Account is one account as the owner's panel shows it.
type Account struct {
	Name      string `json:"name"`
	Role      string `json:"role"`
	CreatedAt string `json:"created_at,omitempty"`
	// Source is "file" for an account of the accounts file, which the page
	// can change, and "config" for one written in config.toml, which it
	// cannot.
	Source string `json:"source"`

	Sessions int        `json:"sessions"`            // in force now
	LastSeen *time.Time `json:"last_seen,omitempty"` // the last request of any of them
	// The last sign-in the trace holds: when, and from where.
	LastSignIn *SignInEvent `json:"last_sign_in,omitempty"`
}

// Accounts lists every account, owners first, then by name.
func (s *Service) Accounts() []Account {
	stats := s.store.byUser()
	last := map[string]SignInEvent{}
	for _, ev := range s.trace.Recent(traceKept) {
		if ev.Event == EventSignedIn && ev.User != "" {
			last[ev.User] = ev // Recent is oldest first: the last one wins
		}
	}

	s.mu.RLock()
	defer s.mu.RUnlock()
	inFile := map[string]UserRecord{}
	if s.file != nil {
		for _, u := range s.file.Users {
			inFile[strings.TrimSpace(u.Name)] = u
		}
	}
	out := make([]Account, 0, len(s.users))
	for name := range s.users {
		a := Account{Name: name, Role: s.roles[name], Source: "config"}
		if u, ok := inFile[name]; ok {
			a.Source, a.CreatedAt = "file", u.CreatedAt
		}
		if st, ok := stats[name]; ok {
			a.Sessions = st.n
			if !st.lastSeen.IsZero() {
				seen := st.lastSeen.UTC()
				a.LastSeen = &seen
			}
		}
		if ev, ok := last[name]; ok {
			a.LastSignIn = &ev
		}
		out = append(out, a)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Role != out[j].Role {
			return out[i].Role == RoleOwner
		}
		return strings.ToLower(out[i].Name) < strings.ToLower(out[j].Name)
	})
	return out
}

// validAccountName keeps names to what reads unambiguously in a log line, a
// URL and the trace.
func validAccountName(name string) bool {
	if name == "" || len(name) > 32 {
		return false
	}
	for _, r := range name {
		if !(r < unicode.MaxASCII && (unicode.IsLetter(r) || unicode.IsDigit(r) || r == '.' || r == '-' || r == '_')) {
			return false
		}
	}
	return true
}

// CreateAccount adds an account to the accounts file and lets it sign in at
// once. The password is hashed here and never kept, logged or returned.
func (s *Service) CreateAccount(name, password, role string) error {
	name = strings.TrimSpace(name)
	if !validAccountName(name) {
		return ErrBadAccountName
	}
	if len([]rune(password)) < minPassword {
		return ErrPasswordTooShort
	}
	role, err := NormalizeRole(role)
	if err != nil {
		return err
	}
	// Hashed before the lock: Argon2id takes a quarter of a second, and every
	// request reads under this lock.
	hash, err := HashPassword(password)
	if err != nil {
		return fmt.Errorf("hash password: %w", err)
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	if s.file == nil {
		return ErrNoAccountsFile
	}
	for existing := range s.users {
		if strings.EqualFold(existing, name) {
			return ErrAccountExists
		}
	}
	if err := s.file.AddWithRole(name, hash, role); err != nil {
		return err
	}
	s.users[name], s.roles[name] = hash, role
	s.resolve()
	return nil
}

// RemoveAccount takes an account out of the accounts file and ends its
// sessions; the caller closes what it still has open. by is the account
// asking, which may not remove itself.
func (s *Service) RemoveAccount(name, by string) error {
	s.mu.Lock()
	if err := s.changeableLocked(name, by); err != nil {
		s.mu.Unlock()
		return err
	}
	if s.roles[name] == RoleOwner && s.ownersLocked() == 1 {
		s.mu.Unlock()
		return ErrLastOwner
	}
	if err := s.file.Remove(name); err != nil {
		s.mu.Unlock()
		return err
	}
	delete(s.users, name)
	delete(s.roles, name)
	s.resolve()
	s.mu.Unlock()
	s.store.RevokeUser(name)
	return nil
}

// SetPassword gives an account a new password and ends its sessions, but the
// one of the browser asking when it changes its own (keep, its token).
func (s *Service) SetPassword(name, password, keep string) error {
	if len([]rune(password)) < minPassword {
		return ErrPasswordTooShort
	}
	hash, err := HashPassword(password)
	if err != nil {
		return fmt.Errorf("hash password: %w", err)
	}
	s.mu.Lock()
	if err := s.changeableLocked(name, ""); err != nil {
		s.mu.Unlock()
		return err
	}
	if err := s.file.SetHash(name, hash); err != nil {
		s.mu.Unlock()
		return err
	}
	s.users[name] = hash
	s.mu.Unlock()
	s.store.RevokeUserExcept(name, keep)
	return nil
}

// CutAccess ends every session of an account, which keeps its password: the
// next sign-in works. by may not cut itself. It returns how many sessions
// ended; the caller closes what they still have open.
func (s *Service) CutAccess(name, by string) (int, error) {
	s.mu.RLock()
	_, known := s.users[name]
	s.mu.RUnlock()
	if !known {
		return 0, ErrNoSuchAccount
	}
	if name == by {
		return 0, ErrYourself
	}
	return s.store.RevokeUser(name), nil
}

// changeableLocked says whether the page may change this account: one of the
// accounts file, and not the asker's own when by is given.
func (s *Service) changeableLocked(name, by string) error {
	if _, ok := s.users[name]; !ok {
		return ErrNoSuchAccount
	}
	if by != "" && name == by {
		return ErrYourself
	}
	if s.file == nil {
		return ErrNoAccountsFile
	}
	for _, u := range s.file.Users {
		if strings.TrimSpace(u.Name) == name {
			return nil
		}
	}
	return ErrConfigAccount
}

func (s *Service) ownersLocked() int {
	n := 0
	for _, r := range s.roles {
		if r == RoleOwner {
			n++
		}
	}
	return n
}
