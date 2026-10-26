package api

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"mime"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/yegors/co-atc/pkg/logger"
)

// StaticFileHandler serves static files dynamically without caching
type StaticFileHandler struct {
	staticDir string
	logger    *logger.Logger
}

// NewStaticFileHandler creates a new static file handler
func NewStaticFileHandler(staticDir string, logger *logger.Logger) *StaticFileHandler {
	// The airport layout is GeoJSON; without this it went out as text/plain.
	_ = mime.AddExtensionType(".geojson", "application/geo+json")
	_ = mime.AddExtensionType(".webmanifest", "application/manifest+json")
	return &StaticFileHandler{
		staticDir: staticDir,
		logger:    logger.Named("static-handler"),
	}
}

// ServeHTTP serves static files dynamically
func (h *StaticFileHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	// The page is read, never written: every other method used to be answered
	// with the file, TRACE and CONNECT included. OPTIONS does not get here --
	// the CORS middleware answers it.
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		w.Header().Set("Allow", "GET, HEAD")
		http.Error(w, "Method Not Allowed", http.StatusMethodNotAllowed)
		return
	}

	// Clean the path to prevent directory traversal attacks
	path := filepath.Clean(r.URL.Path)

	// Remove leading slash
	if strings.HasPrefix(path, "/") {
		path = path[1:]
	}

	// If path is empty, serve index.html
	if path == "" {
		path = "index.html"
	}

	// No hidden file or directory: what the page needs has no leading dot,
	// and what has one is not the page -- a .DS_Store that Finder leaves in a
	// copied directory lists its files, a .git would hand out its history.
	for _, seg := range strings.Split(filepath.ToSlash(path), "/") {
		if strings.HasPrefix(seg, ".") {
			http.NotFound(w, r)
			return
		}
	}

	// Construct full file path
	fullPath := filepath.Join(h.staticDir, path)

	// Ensure the file is within the static directory (security check)
	absStaticDir, err := filepath.Abs(h.staticDir)
	if err != nil {
		h.logger.Error("Failed to get absolute path for static directory", logger.Error(err))
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}

	absFullPath, err := filepath.Abs(fullPath)
	if err != nil {
		h.logger.Error("Failed to get absolute path for requested file", logger.Error(err))
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}

	if !strings.HasPrefix(absFullPath, absStaticDir) {
		h.logger.Warn("Attempted directory traversal attack",
			logger.String("requested_path", path),
			logger.String("full_path", absFullPath),
			logger.String("static_dir", absStaticDir))
		http.Error(w, "Forbidden", http.StatusForbidden)
		return
	}

	// Check if file exists
	fileInfo, err := os.Stat(fullPath)
	if err != nil {
		if os.IsNotExist(err) {
			// If it's a directory request without trailing slash, try index.html
			if !strings.HasSuffix(path, "/") {
				indexPath := filepath.Join(fullPath, "index.html")
				if _, indexErr := os.Stat(indexPath); indexErr == nil {
					fullPath = indexPath
					fileInfo, err = os.Stat(fullPath)
				}
			}

			if err != nil {
				h.logger.Debug("File not found", logger.String("path", fullPath))
				http.NotFound(w, r)
				return
			}
		} else {
			h.logger.Error("Failed to stat file", logger.Error(err), logger.String("path", fullPath))
			http.Error(w, "Internal Server Error", http.StatusInternalServerError)
			return
		}
	}

	// Don't serve directories directly
	if fileInfo.IsDir() {
		// Try to serve index.html from the directory
		indexPath := filepath.Join(fullPath, "index.html")
		if _, err := os.Stat(indexPath); err == nil {
			fullPath = indexPath
		} else {
			h.logger.Debug("Directory listing not allowed", logger.String("path", fullPath))
			http.Error(w, "Forbidden", http.StatusForbidden)
			return
		}
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
	w.Header().Set("Cache-Control", "no-cache")

	data, err := os.ReadFile(fullPath)
	if err != nil {
		h.logger.Debug("Static file not readable", logger.String("path", fullPath), logger.Error(err))
		http.Error(w, "Not Found", http.StatusNotFound)
		return
	}
	sum := sha256.Sum256(data)
	w.Header().Set("ETag", `"`+hex.EncodeToString(sum[:16])+`"`)

	h.logger.Debug("Serving static file",
		logger.String("requested_path", r.URL.Path),
		logger.String("file_path", fullPath))

	http.ServeContent(w, r, filepath.Base(fullPath), time.Time{}, bytes.NewReader(data))
}
