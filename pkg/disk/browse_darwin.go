package disk

import "golang.org/x/sys/unix"

// Hidden says whether the volume mounted at path is one the system keeps out
// of sight (nobrowse: Recovery, Preboot, VM...), which the Finder never shows.
func Hidden(path string) bool {
	var st unix.Statfs_t
	if unix.Statfs(path, &st) != nil {
		return false
	}
	return st.Flags&unix.MNT_DONTBROWSE != 0
}
