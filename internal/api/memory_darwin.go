package api

import (
	"context"
	"os/exec"
	"strconv"
	"strings"
	"time"

	"golang.org/x/sys/unix"
)

// processMemory is the resident memory of each process, in bytes, as ps reads
// it -- one call for all of them. Resident memory, not the "physical
// footprint" the launcher read by itself (proc_pid_rusage), which only cgo
// would reach: close, and the same for both readers from now on. A process
// that is gone is left out.
func processMemory(pids []int) map[int]int64 {
	out := map[int]int64{}
	if len(pids) == 0 {
		return out
	}
	list := make([]string, len(pids))
	for i, p := range pids {
		list[i] = strconv.Itoa(p)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	b, err := exec.CommandContext(ctx, "/bin/ps", "-o", "pid=,rss=", "-p", strings.Join(list, ",")).Output()
	if err != nil && len(b) == 0 {
		return out
	}
	for _, line := range strings.Split(string(b), "\n") {
		f := strings.Fields(line)
		if len(f) != 2 {
			continue
		}
		pid, err1 := strconv.Atoi(f[0])
		kb, err2 := strconv.ParseInt(f[1], 10, 64)
		if err1 == nil && err2 == nil {
			out[pid] = kb * 1024
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
