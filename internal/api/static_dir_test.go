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

// The page is served from server.static_dir when it is set: production pointed
// it at a snapshot of www/ taken at start (12/10), and an instance that tries
// the page points it at the working tree.
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

	// Unset, the page is the one built into the binary.
	cfg.Server.StaticDir = ""
	router = NewRouter(nil, nil, nil, nil, cfg, log, nil, sqlite.NewTranscriptionStorage(nil, log), nil)
	rec = httptest.NewRecorder()
	router.Routes().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/probe.txt", nil))
	if rec.Code == http.StatusOK && rec.Body.String() == "<title>snapshot</title>" {
		t.Fatal("static_dir unset still served the snapshot: the default is not the built-in page")
	}
	rec = httptest.NewRecorder()
	router.Routes().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/", nil))
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), "app.js") {
		t.Fatalf("static_dir unset: GET / %d, want 200 and the built-in index.html", rec.Code)
	}
}

// With no directory beside it, the binary serves its own page, with the
// headers the directory had: the index, a script, the stylesheet and a font,
// each with its type, an ETag on the content and no date, 304 when unchanged,
// GET and HEAD only. What the browser never loads is not there.
func TestTheBinaryServesItsOwnPage(t *testing.T) {
	s := newStack(t, stackOptions{})
	cases := []struct{ path, typ string }{
		{"/", "text/html"},
		{"/index.html", "text/html"},
		{"/app.js", "javascript"},
		{"/par/par-geometry.js", "javascript"},
		{"/vendor/tailwind.css", "text/css"},
		{"/vendor/fontawesome/webfonts/fa-solid-900.woff2", "font/woff2"},
		{"/brand/manifest.webmanifest", "application/manifest+json"},
	}
	for _, c := range cases {
		rec := s.do("GET", c.path, "")
		etag := rec.Header().Get("ETag")
		if rec.Code != http.StatusOK || rec.Body.Len() == 0 || !strings.Contains(rec.Header().Get("Content-Type"), c.typ) {
			t.Errorf("GET %s: %d, %d bytes, type %q; want 200 and %s", c.path, rec.Code, rec.Body.Len(), rec.Header().Get("Content-Type"), c.typ)
			continue
		}
		if etag == "" || rec.Header().Get("Cache-Control") != "no-cache" || rec.Header().Get("Last-Modified") != "" {
			t.Errorf("GET %s: ETag %q, Cache-Control %q, Last-Modified %q; want an ETag, no-cache and no date",
				c.path, etag, rec.Header().Get("Cache-Control"), rec.Header().Get("Last-Modified"))
		}
		if again := s.do("GET", c.path, "", func(r *http.Request) { r.Header.Set("If-None-Match", etag) }); again.Code != http.StatusNotModified {
			t.Errorf("GET %s with its ETag: %d, want 304", c.path, again.Code)
		}
		if head := s.do("HEAD", c.path, ""); head.Code != http.StatusOK {
			t.Errorf("HEAD %s: %d, want 200", c.path, head.Code)
		}
	}
	if rec := s.do("POST", "/app.js", "x"); rec.Code != http.StatusMethodNotAllowed || rec.Header().Get("Allow") != "GET, HEAD" {
		t.Errorf("POST /app.js: %d, Allow %q; want 405, GET and HEAD", rec.Code, rec.Header().Get("Allow"))
	}
	for _, target := range []string{"/par/par-geometry.test.js", "/aircraft-animation.test.js", "/page.go", "/page_test.go",
		"/data/airport-layout.geojson", "/.DS_Store", "/../go.mod", "/%2e%2e/go.mod", "/nothing.js", "/vendor/"} {
		if rec := s.do("GET", target, ""); rec.Code == http.StatusOK {
			t.Errorf("GET %s: 200, want it refused", target)
		}
	}
}

// The station's own files (the airport layout) are not built in: they are
// served under /data/ from their directory beside the databases, then from
// static_dir's data/ as before; nothing else of the data directory is served.
func TestTheStationsOwnFilesAreServedFromTheDataDirectory(t *testing.T) {
	s := newStack(t, stackOptions{})
	layout := filepath.Join(s.cfg.StationFilesDir(), "airport-layout.geojson")
	if rec := s.do("GET", "/data/airport-layout.geojson", ""); rec.Code != http.StatusNotFound {
		t.Fatalf("no layout made: %d, want 404 (the map then draws none)", rec.Code)
	}
	if err := os.MkdirAll(filepath.Dir(layout), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(layout, []byte(`{"type":"FeatureCollection","features":[]}`), 0o644); err != nil {
		t.Fatal(err)
	}
	rec := s.do("GET", "/data/airport-layout.geojson", "")
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), "FeatureCollection") ||
		rec.Header().Get("Content-Type") != "application/geo+json" || rec.Header().Get("ETag") == "" {
		t.Fatalf("GET /data/airport-layout.geojson: %d %q, type %q", rec.Code, rec.Body.String(), rec.Header().Get("Content-Type"))
	}
	// The databases sit one directory up: never served, however asked.
	for _, target := range []string{"/data/" + filepath.Base(s.aircraft.GetDB().Path()), "/data/../" + filepath.Base(s.aircraft.GetDB().Path()),
		"/data/%2e%2e/" + filepath.Base(s.aircraft.GetDB().Path()), "/data/", "/data"} {
		if rec := s.do("GET", target, ""); rec.Code == http.StatusOK {
			t.Errorf("GET %s: 200, want it refused", target)
		}
	}

	// A page directory with its own data/: the station's directory first, the
	// page's when the station's has nothing.
	pages := t.TempDir()
	for name, content := range map[string]string{"index.html": "<title>page</title>", "data/airport-layout.geojson": "page's layout",
		"data/other.geojson": "page's other"} {
		if err := os.MkdirAll(filepath.Join(pages, filepath.Dir(name)), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(pages, filepath.FromSlash(name)), []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	s = newStack(t, stackOptions{static: pages})
	if rec := s.do("GET", "/data/other.geojson", ""); rec.Code != http.StatusOK || rec.Body.String() != "page's other" {
		t.Errorf("only in static_dir/data: %d %q, want the page's", rec.Code, rec.Body.String())
	}
	if err := os.MkdirAll(s.cfg.StationFilesDir(), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(s.cfg.StationFilesDir(), "airport-layout.geojson"), []byte("station's layout"), 0o644); err != nil {
		t.Fatal(err)
	}
	if rec := s.do("GET", "/data/airport-layout.geojson", ""); rec.Code != http.StatusOK || rec.Body.String() != "station's layout" {
		t.Errorf("in both: %d %q, want the station's", rec.Code, rec.Body.String())
	}
}

// The launcher and the sign-in page say where the page comes from: built in
// with the binary's commit, or a directory -- which the public route never names.
func TestTheHealthSaysWhereThePageComesFrom(t *testing.T) {
	s := newStack(t, stackOptions{})
	s.handler.SetBuild("1.0", "abc1234")
	var d HealthDetail
	decode(t, s.do("GET", "/api/v1/health/detail", "", fromLoopback("127.0.0.1:50123", "127.0.0.1:8000")), &d)
	if d.Page != (PageSource{Source: "embedded", Commit: "abc1234"}) {
		t.Errorf("built in: page %+v, want embedded at abc1234", d.Page)
	}
	var v Vitals
	decode(t, s.do("GET", "/api/v1/vitals", ""), &v)
	if v.Page != "embedded" {
		t.Errorf("vitals: page %q, want embedded", v.Page)
	}

	pages := t.TempDir()
	s = newStack(t, stackOptions{static: pages})
	s.handler.SetBuild("1.0", "abc1234")
	var dd HealthDetail
	decode(t, s.do("GET", "/api/v1/health/detail", "", fromLoopback("127.0.0.1:50123", "127.0.0.1:8000")), &dd)
	if dd.Page != (PageSource{Source: "directory", Dir: pages}) {
		t.Errorf("from a directory: page %+v, want directory %s", dd.Page, pages)
	}
	rec := s.do("GET", "/api/v1/vitals", "")
	var vv Vitals
	decode(t, rec, &vv)
	if vv.Page != "directory" || strings.Contains(rec.Body.String(), pages) {
		t.Errorf("vitals: %s, want directory and no path", rec.Body.String())
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
