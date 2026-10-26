// Package tool finds the external programs the server runs -- ffmpeg,
// pdftotext -- wherever they are installed, whatever started the server.
//
// A program started by a terminal inherits the shell's PATH; one started by
// launchd, by `open` or by an application inherits /usr/bin:/bin:/usr/sbin:/sbin,
// which holds none of what Homebrew installs. On 26/10 production started from
// the menu-bar launcher had no audio at all: "ffmpeg": executable file not found
// in $PATH. What the server needs must not depend on who started it.
package tool

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// Dirs are looked in after the PATH, in order: Homebrew on Apple Silicon, then
// Homebrew on Intel and most hand installs, then MacPorts.
var Dirs = []string{"/opt/homebrew/bin", "/usr/local/bin", "/opt/local/bin"}

// Find returns the program to run for name, and whether it was found. A name
// with a path in it is taken as written, found if it is there. A bare name is
// looked up on the PATH, then in Dirs. Not found, it comes back as it was, so
// the error that follows names what was asked for.
func Find(name string) (string, bool) {
	name = strings.TrimSpace(name)
	if name == "" {
		return name, false
	}
	if strings.ContainsRune(name, filepath.Separator) {
		_, err := os.Stat(name)
		return name, err == nil
	}
	if p, err := exec.LookPath(name); err == nil {
		return p, true
	}
	for _, d := range Dirs {
		p := filepath.Join(d, name)
		if st, err := os.Stat(p); err == nil && !st.IsDir() && st.Mode()&0o111 != 0 {
			return p, true
		}
	}
	return name, false
}
