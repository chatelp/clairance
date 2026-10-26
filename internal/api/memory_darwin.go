//go:build darwin && cgo

package api

/*
#include <libproc.h>
#include <sys/resource.h>

// The physical footprint macOS charges a process -- what Activity Monitor
// calls its memory -- or -1 when it cannot be read (gone, or another user's).
static long long phys_footprint(int pid) {
	struct rusage_info_v2 ri;
	if (proc_pid_rusage(pid, RUSAGE_INFO_V2, (rusage_info_t *)&ri) != 0) {
		return -1;
	}
	return (long long)ri.ri_phys_footprint;
}
*/
import "C"

import "golang.org/x/sys/unix"

// processFootprint is the physical footprint of each process, in bytes. Not
// the resident memory ps reads: the sidecar's models live in Metal buffers
// that resident memory leaves out (26/10, the production's sidecar: 93 MB
// resident for 8,879 MB of footprint), where the footprint counts them, as
// the launcher always read it. A process that is gone is left out.
func processFootprint(pids []int) map[int]int64 {
	out := map[int]int64{}
	for _, pid := range pids {
		if b := int64(C.phys_footprint(C.int(pid))); b >= 0 {
			out[pid] = b
		}
	}
	return out
}

// memoryPressure is macOS's own judgement, the one the launcher read:
// kern.memorystatus_vm_pressure_level, 1 normal, 2 warn, 4 critical.
func memoryPressure() string {
	v, err := unix.SysctlUint32("kern.memorystatus_vm_pressure_level")
	if err != nil {
		return ""
	}
	switch v {
	case 1:
		return "normal"
	case 2:
		return "warn"
	case 4:
		return "critical"
	}
	return ""
}
