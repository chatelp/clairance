// Package www is the page, built into the binary: the server serves the page
// of its own commit, wherever it runs and whatever lies beside it
// (docs-fr/35-plan.md, B2). Until 26/10 it was read from a directory, and
// production read a copy of it taken at each delivery, which could differ from
// the binary that served it. server.static_dir still serves a directory
// instead, to work on the page without rebuilding.
package www

import (
	"embed"
	"io/fs"
	"path"
	"strings"
)

// The page, by its directories and the kinds of file at its top. Not "*":
// www/data/ holds what belongs to one station (the airport layout, made per
// station and ignored by git), which must not ride along in a binary built on
// that station's machine; the server serves it from the data directory. A
// directory or a kind of file added at the top of www/ is added here:
// TestEveryFileOfThePageIsBuiltIn names it.
//
//go:embed index.html news.json *.js *.css altitude assets brand map now par phase sounds store story timeline vendor
var files embed.FS

// FS is the page as the server serves it: what is built in, less the tests
// Node runs (*.test.js), which go:embed takes with their directories since it
// cannot leave a file out. They weigh 100 KB in the binary and are never served.
var FS fs.FS = page{files}

// page hides what the browser never loads.
type page struct{ fsys fs.FS }

func (p page) Open(name string) (fs.File, error) {
	if !served(name) {
		return nil, &fs.PathError{Op: "open", Path: name, Err: fs.ErrNotExist}
	}
	return p.fsys.Open(name)
}

func (p page) ReadDir(name string) ([]fs.DirEntry, error) {
	entries, err := fs.ReadDir(p.fsys, name)
	if err != nil {
		return nil, err
	}
	var kept []fs.DirEntry
	for _, e := range entries {
		if served(path.Join(name, e.Name())) {
			kept = append(kept, e)
		}
	}
	return kept, nil
}

func served(name string) bool {
	return !strings.HasSuffix(name, ".test.js")
}
