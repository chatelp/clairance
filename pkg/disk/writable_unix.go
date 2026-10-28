//go:build !windows

package disk

import "golang.org/x/sys/unix"

// Writable says whether this process may create files in dir, without
// creating any: the access the system grants, not a trial write.
func Writable(dir string) bool {
	return unix.Access(dir, unix.W_OK) == nil
}
