//go:build windows

package disk

import "golang.org/x/sys/windows"

// Space returns the space left to this process and the size of the volume
// that holds dir. ok is false when the volume cannot be read. The free figure is
// the one available to the caller, quotas included, as Bavail is on Unix.
func Space(dir string) (free, total int64, ok bool) {
	path, err := windows.UTF16PtrFromString(dir)
	if err != nil {
		return 0, 0, false
	}
	var avail, size, totalFree uint64
	if err := windows.GetDiskFreeSpaceEx(path, &avail, &size, &totalFree); err != nil {
		return 0, 0, false
	}
	return int64(avail), int64(size), true
}
