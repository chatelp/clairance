package api

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/yegors/co-atc/internal/config"
)

// The page's fingerprint follows its files (D87): the same files, the same
// version; one byte changed, another.
func TestThePagesVersionFollowsItsFiles(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "index.html"), []byte("<p>one</p>"), 0o644)
	os.MkdirAll(filepath.Join(dir, "store"), 0o755)
	os.WriteFile(filepath.Join(dir, "store", "a.js"), []byte("let a = 1"), 0o644)
	cfg := &config.Config{}
	cfg.Server.StaticDir = dir
	first := PageVersion(cfg)
	if len(first) != 12 || PageVersion(cfg) != first {
		t.Fatalf("version %q, then %q", first, PageVersion(cfg))
	}
	os.WriteFile(filepath.Join(dir, "store", "a.js"), []byte("let a = 2"), 0o644)
	if PageVersion(cfg) == first {
		t.Error("a file changed, the version did not")
	}
	if PageVersion(&config.Config{}) == first {
		t.Error("the embedded page has the directory's version")
	}
}

// /api/v1/version gives the page's version and the server's date to everyone
// signed in, and the commit to the owner only.
func TestTheVersionAndItsCommit(t *testing.T) {
	s := newStack(t, stackOptions{viewer: true})
	s.handler.SetBuild("dev", "abc1234")
	s.handler.SetVersions("0123456789ab", time.Date(2026, 10, 28, 11, 0, 0, 0, time.UTC))
	s.signIn()
	var owner map[string]any
	s.get("/api/v1/version", &owner)
	if owner["page"] != "0123456789ab" || owner["built"] != "2026-10-28T11:00:00Z" || owner["commit"] != "abc1234" {
		t.Errorf("the owner reads %v", owner)
	}
	s.signInAs(viewerUser)
	var viewer map[string]any
	s.get("/api/v1/version", &viewer)
	if viewer["page"] != "0123456789ab" || viewer["commit"] != "" {
		t.Errorf("a viewer reads %v", viewer)
	}
}
