package api

import (
	"crypto/sha256"
	"encoding/hex"
	"io"
	"io/fs"
	"net/http"
	"time"

	"github.com/yegors/co-atc/internal/config"
)

// What the page and the server are (D87, 28/10): a page left open while the
// server was restarted with a new one is told to reload, and the listener is
// told what is new (www/notices.js). The page is compared, not the server: a
// new server with the same page needs no reload.

// PageVersion is a fingerprint of the page the configuration serves: every
// file's path and bytes, in order. It changes when any of them does, whatever
// commit the binary says; read once, at startup.
func PageVersion(cfg *config.Config) string {
	fsys, _ := pageFiles(cfg)
	h := sha256.New()
	_ = fs.WalkDir(fsys, ".", func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return nil
		}
		f, err := fsys.Open(path)
		if err != nil {
			return nil
		}
		defer f.Close()
		io.WriteString(h, path+"\x00")
		io.Copy(h, f)
		return nil
	})
	return hex.EncodeToString(h.Sum(nil))[:12]
}

// SetVersions gives the handler the page's fingerprint and when the server's
// commit was made, for /api/v1/version.
func (h *Handler) SetVersions(page string, built time.Time) {
	h.pageVersion, h.builtAt = page, built
}

// Versions is what the heartbeat and /api/v1/version say: the page's
// fingerprint and the date of the server's version. The commit is the admin's
// only (guest.go).
func (h *Handler) Versions() map[string]interface{} {
	out := map[string]interface{}{"page": h.pageVersion}
	if !h.builtAt.IsZero() {
		out["built"] = h.builtAt.UTC().Format(time.RFC3339)
	}
	return out
}

// GetVersion is what the page reads at load: the page it was given, and the
// server's version; the commit beside it for the owner.
func (h *Handler) GetVersion(w http.ResponseWriter, r *http.Request) {
	out := h.Versions()
	_, commit := h.build()
	out["commit"] = commit
	w.Header().Set("Cache-Control", "no-store")
	WriteJSON(w, http.StatusOK, out)
}
