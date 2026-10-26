package api

import (
	"fmt"
	"os"
	"strconv"
	"strings"
)

// processMemory is the resident memory of each process, in bytes, from
// /proc. A process that is gone is left out.
func processMemory(pids []int) map[int]int64 {
	out := map[int]int64{}
	page := int64(os.Getpagesize())
	for _, pid := range pids {
		b, err := os.ReadFile(fmt.Sprintf("/proc/%d/statm", pid))
		if err != nil {
			continue
		}
		f := strings.Fields(string(b))
		if len(f) < 2 {
			continue
		}
		if pages, err := strconv.ParseInt(f[1], 10, 64); err == nil {
			out[pid] = pages * page
		}
	}
	return out
}

// memoryPressure is said on macOS alone.
func memoryPressure() string { return "" }
