//go:build !darwin

package disk

// Hidden is the system's out-of-sight volumes; only macOS marks them.
func Hidden(path string) bool { return false }
