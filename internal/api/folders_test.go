package api

import (
	"fmt"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"testing"
)

// The server's folders for choosing the archive (D85): folders only, never a
// file nor a hidden one; the databases' own folder shown but not choosable;
// the owner's alone.
func TestTheServersFoldersForTheArchive(t *testing.T) {
	s := newStack(t, stackOptions{viewer: true})
	dir := t.TempDir()
	os.Mkdir(filepath.Join(dir, "Archives"), 0o755)
	os.Mkdir(filepath.Join(dir, ".hidden"), 0o755)
	os.WriteFile(filepath.Join(dir, "a-file.txt"), nil, 0o644)
	data, _ := filepath.Abs(s.cfg.Storage.SQLiteBasePath)
	s.signIn()

	var out struct {
		Path    string        `json:"path"`
		Parent  string        `json:"parent"`
		Here    folderEntry   `json:"here"`
		Folders []folderEntry `json:"folders"`
		Roots   []folderRoot  `json:"roots"`
	}
	s.get("/api/v1/server/folders?path="+url.QueryEscape(dir), &out)
	if out.Path != dir || out.Parent != filepath.Dir(dir) || len(out.Folders) != 1 || out.Folders[0].Name != "Archives" ||
		!out.Folders[0].Writable || !out.Folders[0].Choosable || !out.Here.Choosable {
		t.Errorf("listing %s: %+v", dir, out)
	}
	if len(out.Roots) == 0 {
		t.Error("no place to start from")
	}

	s.get("/api/v1/server/folders?path="+url.QueryEscape(filepath.Dir(data)), &out)
	for _, f := range out.Folders {
		if f.Path == data && (f.Choosable || f.Why != "databases") {
			t.Errorf("the databases' folder offered as the archive: %+v", f)
		}
	}

	for path, want := range map[string]int{"relative/dir": http.StatusBadRequest, filepath.Join(dir, "gone"): http.StatusNotFound} {
		if rec := s.do("GET", "/api/v1/server/folders?path="+url.QueryEscape(path), ""); rec.Code != want {
			t.Errorf("%s: %d, want %d", path, rec.Code, want)
		}
	}

	// An unplugged disk: the nearest folder above that can be read.
	s.get("/api/v1/server/folders?nearest=1&path="+url.QueryEscape(filepath.Join(dir, "gone", "deeper")), &out)
	if out.Path != dir {
		t.Errorf("nearest to a folder gone: %s, want %s", out.Path, dir)
	}

	s.signInAs(viewerUser)
	if rec := s.do("GET", "/api/v1/server/folders?path="+url.QueryEscape(dir), ""); rec.Code != http.StatusForbidden {
		t.Errorf("a viewer: %d, want 403", rec.Code)
	}
}

// More folders than are shown: the first ones by name, and it says so.
func TestManyFoldersAreSortedThenCut(t *testing.T) {
	s := newStack(t, stackOptions{})
	dir := t.TempDir()
	for i := 0; i < foldersShown+20; i++ {
		os.Mkdir(filepath.Join(dir, fmt.Sprintf("d%04d", i)), 0o755)
	}
	s.signIn()
	var out struct {
		Folders   []folderEntry `json:"folders"`
		Truncated bool          `json:"truncated"`
	}
	s.get("/api/v1/server/folders?path="+url.QueryEscape(dir), &out)
	if !out.Truncated || len(out.Folders) != foldersShown || out.Folders[0].Name != "d0000" || out.Folders[foldersShown-1].Name != fmt.Sprintf("d%04d", foldersShown-1) {
		t.Errorf("truncated %v, %d folders, first %q, last %q", out.Truncated, len(out.Folders), out.Folders[0].Name, out.Folders[len(out.Folders)-1].Name)
	}
}
