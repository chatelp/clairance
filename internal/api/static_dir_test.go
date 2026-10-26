package api

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/yegors/co-atc/internal/config"
	"github.com/yegors/co-atc/internal/storage/sqlite"
	"github.com/yegors/co-atc/pkg/logger"
)

// The page is served from server.static_dir when it is set. Production points
// it at a snapshot of www/ taken at start (12/10): until then, production and
// the working tree were the same directory, and an edit was live on reload.
func TestThePageIsServedFromTheConfiguredDirectory(t *testing.T) {
	log, err := logger.New(logger.Config{Level: "error", Format: "console"})
	if err != nil {
		t.Fatal(err)
	}
	pages := t.TempDir()
	if err := os.WriteFile(filepath.Join(pages, "probe.txt"), []byte("<title>snapshot</title>"), 0o644); err != nil {
		t.Fatal(err)
	}
	cfg := &config.Config{ConfigPath: filepath.Join(t.TempDir(), "config.toml")}
	cfg.Server.Host = "127.0.0.1"
	cfg.Server.StaticDir = pages
	router := NewRouter(nil, nil, nil, nil, cfg, log, nil, sqlite.NewTranscriptionStorage(nil, log), nil)

	rec := httptest.NewRecorder()
	router.Routes().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/probe.txt", nil))
	if rec.Code != http.StatusOK || rec.Body.String() != "<title>snapshot</title>" {
		t.Fatalf("GET /probe.txt from static_dir: %d %q, want 200 and the snapshot's page", rec.Code, rec.Body.String())
	}

	// Unset, the directory is www next to the working directory, as it always was.
	cfg.Server.StaticDir = ""
	router = NewRouter(nil, nil, nil, nil, cfg, log, nil, sqlite.NewTranscriptionStorage(nil, log), nil)
	rec = httptest.NewRecorder()
	router.Routes().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/probe.txt", nil))
	if rec.Code == http.StatusOK && rec.Body.String() == "<title>snapshot</title>" {
		t.Fatal("static_dir unset still served the snapshot: the default is not www")
	}
}

// A browser revalidates every file (no-cache) and is told "not modified" on
// the content, not on the file's date: a file replaced by an older version
// with a newer date must still come back in full (13/10/2026).
func TestAnUnchangedFileIs304AndAChangedOneIs200WhateverItsDate(t *testing.T) {
	log, err := logger.New(logger.Config{Level: "error", Format: "console"})
	if err != nil {
		t.Fatal(err)
	}
	pages := t.TempDir()
	page := filepath.Join(pages, "app.js")
	if err := os.WriteFile(page, []byte("new()"), 0o644); err != nil {
		t.Fatal(err)
	}
	cfg := &config.Config{ConfigPath: filepath.Join(t.TempDir(), "config.toml")}
	cfg.Server.Host = "127.0.0.1"
	cfg.Server.StaticDir = pages
	routes := NewRouter(nil, nil, nil, nil, cfg, log, nil, sqlite.NewTranscriptionStorage(nil, log), nil).Routes()

	rec := httptest.NewRecorder()
	routes.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/app.js", nil))
	etag := rec.Header().Get("ETag")
	if rec.Code != http.StatusOK || etag == "" || rec.Header().Get("Last-Modified") != "" {
		t.Fatalf("first GET: %d, ETag %q, Last-Modified %q; want 200, an ETag and no date", rec.Code, etag, rec.Header().Get("Last-Modified"))
	}
	if cc := rec.Header().Get("Cache-Control"); cc != "no-cache" {
		t.Fatalf("Cache-Control %q, want no-cache", cc)
	}

	again := httptest.NewRequest(http.MethodGet, "/app.js", nil)
	again.Header.Set("If-None-Match", etag)
	rec = httptest.NewRecorder()
	routes.ServeHTTP(rec, again)
	if rec.Code != http.StatusNotModified {
		t.Fatalf("same content, same ETag: %d, want 304", rec.Code)
	}

	// The older version comes back, with a newer date than the one the browser holds.
	if err := os.WriteFile(page, []byte("old()"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Chtimes(page, time.Now(), time.Now().Add(time.Hour)); err != nil {
		t.Fatal(err)
	}
	rec = httptest.NewRecorder()
	routes.ServeHTTP(rec, again)
	if rec.Code != http.StatusOK || rec.Body.String() != "old()" {
		t.Fatalf("changed content, stale ETag: %d %q, want 200 and the file as it is now", rec.Code, rec.Body.String())
	}
	if rec.Header().Get("ETag") == etag {
		t.Fatal("the ETag did not change with the content")
	}
}

// The page is only read: GET and HEAD serve it, every other method is
// refused, and none of them returns the file.
func TestThePageIsReadAndNeverWritten(t *testing.T) {
	log, err := logger.New(logger.Config{Level: "error", Format: "console"})
	if err != nil {
		t.Fatal(err)
	}
	pages := t.TempDir()
	if err := os.WriteFile(filepath.Join(pages, "index.html"), []byte("<title>page</title>"), 0o644); err != nil {
		t.Fatal(err)
	}
	cfg := &config.Config{ConfigPath: filepath.Join(t.TempDir(), "config.toml")}
	cfg.Server.Host = "127.0.0.1"
	cfg.Server.StaticDir = pages
	routes := NewRouter(nil, nil, nil, nil, cfg, log, nil, sqlite.NewTranscriptionStorage(nil, log), nil).Routes()

	for _, method := range []string{"GET", "HEAD"} {
		rec := httptest.NewRecorder()
		routes.ServeHTTP(rec, httptest.NewRequest(method, "/index.html", nil))
		if rec.Code != http.StatusOK {
			t.Errorf("%s /index.html: %d, want 200", method, rec.Code)
		}
	}
	for _, method := range []string{"POST", "PUT", "DELETE", "PATCH", "TRACE", "CONNECT"} {
		rec := httptest.NewRecorder()
		routes.ServeHTTP(rec, httptest.NewRequest(method, "/index.html", strings.NewReader("x")))
		if rec.Code != http.StatusMethodNotAllowed || rec.Header().Get("Allow") != "GET, HEAD" ||
			strings.Contains(rec.Body.String(), "<title>") {
			t.Errorf("%s /index.html: %d, Allow %q, body %q; want 405, GET and HEAD, not the page",
				method, rec.Code, rec.Header().Get("Allow"), rec.Body.String())
		}
	}
}

// Nothing outside the page's directory, and nothing hidden in it: a path
// that climbs out, spelled plainly or encoded, and a file or directory whose
// name starts with a dot are not served.
func TestThePageServesNeitherWhatIsAboveItNorWhatIsHidden(t *testing.T) {
	log, err := logger.New(logger.Config{Level: "error", Format: "console"})
	if err != nil {
		t.Fatal(err)
	}
	root := t.TempDir()
	pages := filepath.Join(root, "www")
	for name, content := range map[string]string{
		"outside.txt":            "secret above the page",
		"www/index.html":         "<title>page</title>",
		"www/.DS_Store":          "secret finder listing",
		"www/.git/config":        "secret repository",
		"www/map/.hidden.js":     "secret hidden script",
		"www/map/layer.js":       "visible()",
		"www/.well-known/x.json": "secret dot directory",
	} {
		path := filepath.Join(root, filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	cfg := &config.Config{ConfigPath: filepath.Join(t.TempDir(), "config.toml")}
	cfg.Server.Host = "127.0.0.1"
	cfg.Server.StaticDir = pages
	routes := NewRouter(nil, nil, nil, nil, cfg, log, nil, sqlite.NewTranscriptionStorage(nil, log), nil).Routes()

	get := func(target string) *httptest.ResponseRecorder {
		rec := httptest.NewRecorder()
		routes.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, target, nil))
		return rec
	}
	for _, target := range []string{"/", "/index.html", "/map/layer.js"} {
		if rec := get(target); rec.Code != http.StatusOK {
			t.Errorf("GET %s: %d, want 200", target, rec.Code)
		}
	}
	for _, target := range []string{"/../outside.txt", "/%2e%2e/outside.txt", "/..%2foutside.txt", "/map/../../outside.txt",
		"/.DS_Store", "/.git/config", "/.git/", "/map/.hidden.js", "/.well-known/x.json", "/map/%2ehidden.js"} {
		if rec := get(target); rec.Code == http.StatusOK || strings.Contains(rec.Body.String(), "secret") {
			t.Errorf("GET %s: %d %q, want it refused", target, rec.Code, rec.Body.String())
		}
	}
}
