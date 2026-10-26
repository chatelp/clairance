package www

import (
	"io/fs"
	"os"
	"path"
	"strings"
	"testing"
)

// Every file of www/ the page may load is built in, and nothing that is not
// the page: a script added in a new directory, or a new kind of file at the
// top, would otherwise be in the tree and missing from the binary -- a 404 in
// production and nowhere else.
func TestEveryFileOfThePageIsBuiltIn(t *testing.T) {
	onDisk := os.DirFS(".")
	checked := 0
	err := fs.WalkDir(onDisk, ".", func(name string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		base := path.Base(name)
		if name != "." && (strings.HasPrefix(base, ".") || strings.HasPrefix(base, "_")) {
			if d.IsDir() {
				return fs.SkipDir
			}
			return nil // hidden: neither built in nor served
		}
		if d.IsDir() {
			if name == "data" {
				return fs.SkipDir // the station's own files, served from the data directory
			}
			return nil
		}
		if strings.HasSuffix(name, ".go") || strings.HasSuffix(name, ".test.js") {
			return nil
		}
		checked++
		if _, err := fs.Stat(FS, name); err != nil {
			t.Errorf("www/%s is not built into the binary: add its directory or its kind to the go:embed line of www/page.go", name)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if checked < 100 {
		t.Fatalf("only %d files of the page found on disk: the test does not run in www/", checked)
	}
}

// What the browser never loads is not served: the tests Node runs, this
// package's source, the station's own files.
func TestThePageHoldsNoTestNoSourceAndNoStationFile(t *testing.T) {
	err := fs.WalkDir(FS, ".", func(name string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if strings.HasSuffix(name, ".test.js") || strings.HasSuffix(name, ".go") || name == "data" || strings.HasPrefix(name, "data/") {
			t.Errorf("served with the page: %s", name)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"par/par-geometry.test.js", "page.go", "page_test.go", "data/airport-layout.geojson"} {
		if _, err := FS.Open(name); err == nil {
			t.Errorf("%s opens in the page", name)
		}
	}
	for _, name := range []string{"index.html", "app.js", "vendor/tailwind.css", "par/par-geometry.js"} {
		if _, err := fs.Stat(FS, name); err != nil {
			t.Errorf("%s is not in the page: %v", name, err)
		}
	}
}
