package tool

import (
	"os"
	"path/filepath"
	"testing"
)

// A program that is not on the PATH is still found where it is installed:
// the PATH of a server started by launchd or `open` holds none of Homebrew.
func TestAProgramOffThePathIsFoundWhereItIsInstalled(t *testing.T) {
	dir := t.TempDir()
	prog := filepath.Join(dir, "fakeffmpeg")
	if err := os.WriteFile(prog, []byte("#!/bin/sh\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	notExec := filepath.Join(dir, "notexec")
	if err := os.WriteFile(notExec, []byte("data"), 0o644); err != nil {
		t.Fatal(err)
	}
	saved := Dirs
	t.Cleanup(func() { Dirs = saved })
	Dirs = []string{t.TempDir(), dir}
	t.Setenv("PATH", "/usr/bin:/bin:/usr/sbin:/sbin")

	if p, ok := Find("fakeffmpeg"); !ok || p != prog {
		t.Errorf("off the PATH: %q %v, want %q", p, ok, prog)
	}
	if p, ok := Find("notexec"); ok {
		t.Errorf("a file that cannot run was found: %q", p)
	}
	if p, ok := Find("nosuchprogram"); ok || p != "nosuchprogram" {
		t.Errorf("a missing program: %q %v, want it back as asked", p, ok)
	}
	// On the PATH: the PATH wins.
	t.Setenv("PATH", dir)
	if p, ok := Find("fakeffmpeg"); !ok || p != prog {
		t.Errorf("on the PATH: %q %v", p, ok)
	}
	// A path is taken as written.
	if p, ok := Find(prog); !ok || p != prog {
		t.Errorf("a path: %q %v", p, ok)
	}
	if _, ok := Find(filepath.Join(dir, "gone")); ok {
		t.Error("a path to nothing was found")
	}
}
