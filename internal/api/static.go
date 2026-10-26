package api

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io/fs"
	"mime"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"strings"
	"time"

	"github.com/yegors/co-atc/internal/config"
	"github.com/yegors/co-atc/pkg/logger"
	"github.com/yegors/co-atc/www"
)

// StaticFileHandler serves the page: the one built into the binary (package
// www), or a directory named by server.static_dir. Under /data/ it serves
// first the files that belong to this station (config.StationFilesDir), which
// cannot be built in.
type StaticFileHandler struct {
	page    fs.FS
	station fs.FS // nil: none
	logger  *logger.Logger
}

// NewStaticFileHandler serves page, and under /data/ station's files before
// page's own; station may be nil.
func NewStaticFileHandler(page, station fs.FS, logger *logger.Logger) *StaticFileHandler {
	// The airport layout is GeoJSON; without this it went out as text/plain.
	_ = mime.AddExtensionType(".geojson", "application/geo+json")
	_ = mime.AddExtensionType(".webmanifest", "application/manifest+json")
	return &StaticFileHandler{
		page:    page,
		station: station,
		logger:  logger.Named("static-handler"),
	}
}

// PageSource is where the page comes from: "embedded" in the binary, or
// "directory" with that directory, absolute.
type PageSource struct {
	Source string `json:"source"`
	Dir    string `json:"dir,omitempty"`
	Commit string `json:"commit,omitempty"` // the binary's, when embedded
}

// pageFiles is the page a configuration serves, and where it comes from.
func pageFiles(cfg *config.Config) (fs.FS, PageSource) {
	if cfg == nil || cfg.Server.StaticDir == "" {
		return www.FS, PageSource{Source: "embedded"}
	}
	dir := cfg.Server.StaticDir
	if abs, err := filepath.Abs(dir); err == nil {
		dir = abs
	}
	return os.DirFS(dir), PageSource{Source: "directory", Dir: dir}
}

// PageOf says where a configuration's page comes from, for the startup log.
func PageOf(cfg *config.Config) PageSource {
	_, src := pageFiles(cfg)
	return src
}

// stationFiles is the station's own files, nil without a configuration.
func stationFiles(cfg *config.Config) fs.FS {
	if cfg == nil {
		return nil
	}
	return os.DirFS(cfg.StationFilesDir())
}

// ServeHTTP serves the page's files without caching
func (h *StaticFileHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	// The page is read, never written: every other method used to be answered
	// with the file, TRACE and CONNECT included. OPTIONS does not get here --
	// the CORS middleware answers it.
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		w.Header().Set("Allow", "GET, HEAD")
		http.Error(w, "Method Not Allowed", http.StatusMethodNotAllowed)
		return
	}

	// Cleaned as an absolute path, nothing climbs above the page: "/../x" is "/x".
	name := strings.TrimPrefix(path.Clean("/"+r.URL.Path), "/")
	if name == "" {
		name = "index.html"
	}

	// No hidden file or directory: what the page needs has no leading dot,
	// and what has one is not the page -- a .DS_Store that Finder leaves in a
	// copied directory lists its files, a .git would hand out its history.
	for _, seg := range strings.Split(name, "/") {
		if strings.HasPrefix(seg, ".") {
			http.NotFound(w, r)
			return
		}
	}

	var data []byte
	served, status := "", http.StatusNotFound
	if rest, ok := strings.CutPrefix(name, "data/"); ok && h.station != nil {
		if d, err := fs.ReadFile(h.station, rest); err == nil {
			data, served, status = d, name, http.StatusOK
		}
	}
	if status != http.StatusOK {
		data, served, status = h.readPage(name)
	}
	switch status {
	case http.StatusOK:
	case http.StatusNotFound:
		h.logger.Debug("File not found", logger.String("path", name))
		http.NotFound(w, r)
		return
	case http.StatusForbidden:
		h.logger.Debug("Directory listing not allowed", logger.String("path", name))
		http.Error(w, "Forbidden", http.StatusForbidden)
		return
	default:
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}

	// Always revalidated, never served from the cache without asking: a file
	// changed on disk is seen on the next load, and an unchanged one costs a 304
	// instead of its bytes. Upstream's no-store made every reload fetch the
	// whole page again (audit 4.3).
	//
	// "Unchanged" is judged on the content, not on the file's date: a checkout
	// of an older version gives every file a newer date, and a browser that
	// had cached the newer version was then told "not modified" and kept a mix
	// of the two (13/10/2026, switching the served directory between branches:
	// an old index.html with new scripts, the store never built, the page
	// "Connection lost"). The ETag is the content's hash; no Last-Modified.
	// The built-in page has no date at all.
	w.Header().Set("Cache-Control", "no-cache")
	sum := sha256.Sum256(data)
	w.Header().Set("ETag", `"`+hex.EncodeToString(sum[:16])+`"`)

	h.logger.Debug("Serving static file",
		logger.String("requested_path", r.URL.Path),
		logger.String("file_path", served))

	http.ServeContent(w, r, path.Base(served), time.Time{}, bytes.NewReader(data))
}

// readPage reads name from the page, or the index.html of the directory of
// that name; a directory without one is not listed. It returns the file, the
// name it was read under and the status to answer.
func (h *StaticFileHandler) readPage(name string) ([]byte, string, int) {
	info, err := fs.Stat(h.page, name)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) || errors.Is(err, fs.ErrInvalid) {
			return nil, "", http.StatusNotFound
		}
		h.logger.Error("Failed to stat file", logger.Error(err), logger.String("path", name))
		return nil, "", http.StatusInternalServerError
	}
	if info.IsDir() {
		name = path.Join(name, "index.html")
		if _, err := fs.Stat(h.page, name); err != nil {
			return nil, "", http.StatusForbidden
		}
	}
	data, err := fs.ReadFile(h.page, name)
	if err != nil {
		h.logger.Debug("Static file not readable", logger.String("path", name), logger.Error(err))
		return nil, "", http.StatusNotFound
	}
	return data, name, http.StatusOK
}
