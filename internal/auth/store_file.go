package auth

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// UserFile is the accounts the first-run page creates, kept beside the
// configuration rather than inside it.
//
// config.toml is never rewritten by the program: it carries the measured tables
// this project reasons from, in comments, and a round-trip through a TOML
// encoder would lose every one of them. So a page that creates an account has to
// write somewhere else, and the two sources coexist -- the file you edit by hand
// and the file the page owns.
type UserFile struct {
	path string

	// Access is how the server is to be reached, as last chosen on the first-run
	// page or in the settings panel: AccessLocal or AccessAccount. Empty in a
	// file written before the choice was recorded, where the accounts alone
	// decide, as they always did.
	Access string       `json:"access,omitempty"`
	Users  []UserRecord `json:"users"`
}

// The two answers to "how is this server reached". Local-only keeps any
// accounts in the file, unused, so choosing an account again brings them back.
const (
	AccessLocal   = "local"
	AccessAccount = "account"
)

// UserRecord is one account. The password itself is never here, only its
// Argon2id hash; see password.go.
type UserRecord struct {
	Name         string `json:"name"`
	PasswordHash string `json:"password_hash"`
	CreatedAt    string `json:"created_at"`

	// Role is "owner" or "viewer" (see role.go). Absent in every file written
	// before roles existed, and absent means owner; never written for the
	// account the first-run page creates, which is the owner's.
	Role string `json:"role,omitempty"`
}

// DefaultUserFile is the file the first-run page writes, next to config.toml.
const DefaultUserFile = "users.json"

// LoadUserFile reads the accounts file. A missing file is not an error: it is
// the normal state before anyone has set anything up.
func LoadUserFile(configPath string) (*UserFile, error) {
	path := filepath.Join(filepath.Dir(configPath), DefaultUserFile)
	f := &UserFile{path: path}

	raw, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return f, nil
	}
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", path, err)
	}
	if err := json.Unmarshal(raw, f); err != nil {
		return nil, fmt.Errorf("parse %s: %w", path, err)
	}
	f.path = path
	return f, nil
}

// Path is where the accounts are kept.
func (f *UserFile) Path() string { return f.path }

// Add appends an account and writes the file. It refuses a duplicate name
// rather than shadowing one silently: two accounts with one name is a question
// about which password works, and nobody should have to find out by trying.
func (f *UserFile) Add(name, hash string) error {
	return f.add(name, hash, "")
}

func (f *UserFile) add(name, hash, role string) error {
	name = strings.TrimSpace(name)
	if name == "" {
		return fmt.Errorf("a user needs a name")
	}
	if hash == "" {
		return fmt.Errorf("a user needs a password hash")
	}
	for _, u := range f.Users {
		if strings.EqualFold(strings.TrimSpace(u.Name), name) {
			return fmt.Errorf("an account named %q already exists in %s", name, f.path)
		}
	}
	f.Users = append(f.Users, UserRecord{Name: name, PasswordHash: hash, CreatedAt: nowRFC3339(), Role: role})
	if err := f.save(); err != nil {
		// Not written, so not added: a later save must not persist it by accident.
		f.Users = f.Users[:len(f.Users)-1]
		return err
	}
	return nil
}

// AddWithRole is Add for an account created from the owner's panel, written
// with its role in the same write: an account on disk for a moment without
// it would be an owner.
func (f *UserFile) AddWithRole(name, hash, role string) error {
	if role == RoleOwner {
		role = "" // absent means owner, as in every file written before roles
	}
	return f.add(name, hash, role)
}

// Remove takes an account out and writes the file, or changes nothing.
func (f *UserFile) Remove(name string) error {
	for i, u := range f.Users {
		if strings.TrimSpace(u.Name) == name {
			prev := append([]UserRecord(nil), f.Users...)
			f.Users = append(f.Users[:i:i], f.Users[i+1:]...)
			if err := f.save(); err != nil {
				f.Users = prev
				return err
			}
			return nil
		}
	}
	return fmt.Errorf("no account named %q in %s", name, f.path)
}

// SetHash replaces an account's password hash and writes the file, or
// changes nothing.
func (f *UserFile) SetHash(name, hash string) error {
	for i, u := range f.Users {
		if strings.TrimSpace(u.Name) == name {
			prev := f.Users[i].PasswordHash
			f.Users[i].PasswordHash = hash
			if err := f.save(); err != nil {
				f.Users[i].PasswordHash = prev
				return err
			}
			return nil
		}
	}
	return fmt.Errorf("no account named %q in %s", name, f.path)
}

// SetAccess records how the server is to be reached, or changes nothing if it
// cannot be written.
func (f *UserFile) SetAccess(mode string) error {
	prev := f.Access
	f.Access = mode
	if err := f.save(); err != nil {
		f.Access = prev
		return err
	}
	return nil
}

// save writes through a temporary file. A half-written accounts file would lock
// the owner out of their own server, and the failure would only appear at the
// next start.
func (f *UserFile) save() error {
	if f.Users == nil {
		f.Users = []UserRecord{} // "users": [] rather than null in a local-only file
	}
	b, err := json.MarshalIndent(f, "", "  ")
	if err != nil {
		return err
	}
	tmp := f.path + ".tmp"
	// 0600: this holds password hashes. They are Argon2id and slow to attack,
	// which is a reason not to hand them out, not a reason to.
	if err := os.WriteFile(tmp, append(b, '\n'), 0o600); err != nil {
		return fmt.Errorf("write %s: %w", tmp, err)
	}
	if err := os.Rename(tmp, f.path); err != nil {
		return fmt.Errorf("install %s: %w", f.path, err)
	}
	return nil
}

func nowRFC3339() string { return time.Now().UTC().Format(time.RFC3339) }
