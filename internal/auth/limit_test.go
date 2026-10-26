package auth

import (
	"encoding/base64"
	"fmt"
	"net/http/httptest"
	"testing"

	"golang.org/x/crypto/argon2"
)

// cheapHash is an Argon2id hash with tiny parameters, which VerifyPassword
// reads from the hash itself: what these tests count is attempts, not the
// cost of each.
func cheapHash(password string) string {
	salt := []byte("sixteen byte salt")[:16]
	key := argon2.IDKey([]byte(password), salt, 1, 8*1024, 1, 32)
	return fmt.Sprintf("$argon2id$v=%d$m=%d,t=%d,p=%d$%s$%s", argon2.Version, 8*1024, 1, 1,
		base64.RawStdEncoding.EncodeToString(salt), base64.RawStdEncoding.EncodeToString(key))
}

func TestTheLimitCountsAnAddressAndAnIPv6Slash64(t *testing.T) {
	for addr, want := range map[string]string{
		"203.0.113.5":            "203.0.113.5",
		"203.0.113.5:4444":       "203.0.113.5",
		"::ffff:203.0.113.5":     "203.0.113.5",
		"2001:db8:1:2::1":        "2001:db8:1:2::/64",
		"2001:db8:1:2:ffff::9":   "2001:db8:1:2::/64",
		"[2001:db8:1:2::1]:4444": "2001:db8:1:2::/64",
		"2001:db8:1:3::1":        "2001:db8:1:3::/64",
		"not an address":         "not an address",
	} {
		if got := limitKey(addr); got != want {
			t.Errorf("limitKey(%q) = %q, want %q", addr, got, want)
		}
	}
}

// Eight wrong passwords, each from a new address of the same IPv6 /64, and
// the right one from a ninth is refused; another /64 is not caught.
func TestANewIPv6AddressPerGuessDoesNotEscapeTheLimit(t *testing.T) {
	s, err := NewService(Config{Enabled: true, Users: []User{{Name: "pierre", PasswordHash: cheapHash("the right one")}}})
	if err != nil {
		t.Fatal(err)
	}
	try := func(addr, password string) error {
		r := httptest.NewRequest("POST", "/api/v1/auth/login", nil)
		r.RemoteAddr = "[" + addr + "]:5000"
		_, err := s.SignIn(r, "pierre", password)
		return err
	}
	for i := 1; i <= 8; i++ {
		_ = try(fmt.Sprintf("2001:db8:1:2::%x", i), "wrong")
	}
	if err := try("2001:db8:1:2:abcd::1", "the right one"); err == nil {
		t.Error("a ninth address of the same /64 signed in after eight wrong passwords")
	}
	if err := try("2001:db8:1:3::1", "the right one"); err != nil {
		t.Errorf("another /64: %v", err)
	}
}
