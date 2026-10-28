package api

import (
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"

	"github.com/yegors/co-atc/pkg/disk"
)

// The folders of the server's machine, for the owner choosing where the
// databases are archived (D85, 28/10): the page cannot open the system's own
// folder picker for the server -- the browser gives no path, and it often runs
// on another machine. Folders only, never files, never hidden ones; the
// owner's alone (ownerRoutes). Reading only: nothing is made or written.

const foldersShown = 500

type folderEntry struct {
	Name      string `json:"name"`
	Path      string `json:"path"`
	Writable  bool   `json:"writable"`
	Choosable bool   `json:"choosable"`
	Why       string `json:"why,omitempty"` // why it cannot be chosen: "databases" or "readonly"
}

type folderRoot struct {
	Name      string `json:"name"`
	Path      string `json:"path"`
	FreeBytes int64  `json:"free_bytes,omitempty"`
}

// GetServerFolders lists a folder of the server's machine: ?path=, absolute;
// without one, the archive in force, else the disks. With ?nearest=1, a folder
// that cannot be read -- its disk unplugged -- gives way to the nearest one
// above it that can.
func (h *Handler) GetServerFolders(w http.ResponseWriter, r *http.Request) {
	dataDir := ""
	if h.config != nil {
		dataDir, _ = filepath.Abs(h.config.Storage.SQLiteBasePath)
	}
	path := r.URL.Query().Get("path")
	if path == "" && h.runtime != nil {
		path = h.runtime.DBArchiveDir()
	}
	if path == "" {
		path = defaultFolder()
	}
	if !filepath.IsAbs(path) {
		WriteJSON(w, http.StatusBadRequest, map[string]string{"error": "the path must be absolute"})
		return
	}
	path = filepath.Clean(path)
	entries, err := os.ReadDir(path)
	for err != nil && r.URL.Query().Get("nearest") == "1" && filepath.Dir(path) != path {
		path = filepath.Dir(path)
		entries, err = os.ReadDir(path)
	}
	if err != nil {
		WriteJSON(w, http.StatusNotFound, map[string]string{"error": "this folder cannot be read (is its disk plugged in?)"})
		return
	}
	folders := []folderEntry{}
	for _, e := range entries {
		name := e.Name()
		if strings.HasPrefix(name, ".") {
			continue
		}
		full := filepath.Join(path, name)
		info, err := os.Stat(full) // follows a link to a folder
		if err != nil || !info.IsDir() {
			continue
		}
		folders = append(folders, folderOf(name, full, dataDir))
	}
	// Sorted, then cut: the first ones by name, never any taken at random.
	sort.Slice(folders, func(i, j int) bool { return strings.ToLower(folders[i].Name) < strings.ToLower(folders[j].Name) })
	truncated := len(folders) > foldersShown
	if truncated {
		folders = folders[:foldersShown]
	}
	parent := filepath.Dir(path)
	if parent == path {
		parent = ""
	}
	here := folderOf(filepath.Base(path), path, dataDir)
	w.Header().Set("Cache-Control", "no-store")
	WriteJSON(w, http.StatusOK, map[string]any{
		"path": path, "parent": parent, "here": here, "folders": folders, "truncated": truncated, "roots": folderRoots(dataDir),
	})
}

// folderOf is one folder: writable, and choosable as the archive -- never the
// databases' own folder nor one inside it.
func folderOf(name, full, dataDir string) folderEntry {
	f := folderEntry{Name: name, Path: full, Writable: disk.Writable(full)}
	switch {
	case dataDir != "" && (full == dataDir || strings.HasPrefix(full, dataDir+string(filepath.Separator))):
		f.Why = "databases" // the databases' own folder
	case !f.Writable:
		f.Why = "readonly" // the server cannot write here
	default:
		f.Choosable = true
	}
	return f
}

// defaultFolder is where browsing starts without an archive: the disks.
func defaultFolder() string {
	if runtime.GOOS == "darwin" {
		return "/Volumes"
	}
	if home, err := os.UserHomeDir(); err == nil {
		return home
	}
	return "/"
}

// folderRoots are the places to start from: each disk mounted, the home folder,
// the databases' folder's disk.
func folderRoots(dataDir string) []folderRoot {
	roots := []folderRoot{}
	add := func(name, path string) {
		for _, r := range roots {
			if r.Path == path {
				return
			}
		}
		root := folderRoot{Name: name, Path: path}
		if free, _, ok := disk.Space(path); ok {
			root.FreeBytes = free
		}
		roots = append(roots, root)
	}
	if runtime.GOOS == "darwin" {
		if entries, err := os.ReadDir("/Volumes"); err == nil {
			for _, e := range entries {
				if !strings.HasPrefix(e.Name(), ".") {
					add(e.Name(), filepath.Join("/Volumes", e.Name()))
				}
			}
		}
	}
	if home, err := os.UserHomeDir(); err == nil {
		add("Home", home)
	}
	return roots
}
