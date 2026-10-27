package auth

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// The last owner is never removed: nobody would be left to manage the others.
// With a second owner, either may go.
func TestTheLastOwnerIsNeverRemoved(t *testing.T) {
	dir := t.TempDir()
	writeUsers(t, dir, `{"name":"pierre","password_hash":"`+decoyHash+`"},
		{"name":"guest","password_hash":"`+decoyHash+`","role":"viewer"}`)
	s := startIn(t, dir, false)
	if err := s.RemoveAccount("pierre", ""); !errors.Is(err, ErrLastOwner) {
		t.Fatalf("removing the only owner: %v, want ErrLastOwner", err)
	}
	if err := s.CreateAccount("second", "long enough here", RoleOwner); err != nil {
		t.Fatal(err)
	}
	if err := s.RemoveAccount("pierre", "second"); err != nil {
		t.Fatalf("removing one of two owners: %v", err)
	}
	if err := s.RemoveAccount("second", ""); !errors.Is(err, ErrLastOwner) {
		t.Errorf("removing the owner left: %v, want ErrLastOwner", err)
	}
}

// What the panel writes is the accounts file, read back by the next start
// with the roles it was given; an owner made from the panel carries no role,
// as in every file written before roles.
func TestAccountsChangedFromThePanelOutliveARestart(t *testing.T) {
	dir := t.TempDir()
	writeUsers(t, dir, `{"name":"pierre","password_hash":"`+decoyHash+`"}`)
	s := startIn(t, dir, false)
	if err := s.CreateAccount("ami", "long enough here", RoleViewer); err != nil {
		t.Fatal(err)
	}
	if err := s.CreateAccount("second", "long enough here", RoleOwner); err != nil {
		t.Fatal(err)
	}
	if err := s.SetPassword("ami", "another long one", ""); err != nil {
		t.Fatal(err)
	}
	if err := s.RemoveAccount("second", "pierre"); err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile(filepath.Join(dir, DefaultUserFile))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(raw), "long enough here") || strings.Contains(string(raw), "another long one") {
		t.Fatal("a password is in the accounts file")
	}
	again := startIn(t, dir, false)
	got := map[string]string{}
	for _, a := range again.Accounts() {
		got[a.Name] = a.Role
	}
	if len(got) != 2 || got["pierre"] != RoleOwner || got["ami"] != RoleViewer {
		t.Errorf("read back: %v, want pierre owner and ami viewer", got)
	}
	again.mu.RLock()
	hash := again.users["ami"]
	again.mu.RUnlock()
	if !VerifyPassword("another long one", hash) {
		t.Error("the new password was not kept")
	}
}
