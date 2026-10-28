//go:build windows

package disk

// Writable is not known on Windows without trying a write: every folder is
// offered, and the server's own check refuses one it cannot write.
func Writable(dir string) bool {
	return true
}
