package api

import (
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
		if f.Path == data && (f.Choosable || f.Why == "") {
			t.Errorf("the databases' folder offered as the archive: %+v", f)
		}
	}

	for path, want := range map[string]int{"relative/dir": http.StatusBadRequest, filepath.Join(dir, "gone"): http.StatusNotFound} {
		if rec := s.do("GET", "/api/v1/server/folders?path="+url.QueryEscape(path), ""); rec.Code != want {
			t.Errorf("%s: %d, want %d", path, rec.Code, want)
		}
	}

	s.signInAs(viewerUser)
	if rec := s.do("GET", "/api/v1/server/folders?path="+url.QueryEscape(dir), ""); rec.Code != http.StatusForbidden {
		t.Errorf("a viewer: %d, want 403", rec.Code)
	}
}
