//go:build !windows

package companion

import (
	"os/exec"
	"syscall"
)

// The same three gestures as the sidecar's (internal/transcription,
// sidecar_unix.go), kept apart so that file stays offerable upstream alone.

// startInOwnGroup puts the program in a process group of its own, so that
// what it forks is stopped with it.
func startInOwnGroup(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
}

// terminateGroup asks the whole group to stop.
func terminateGroup(cmd *exec.Cmd) {
	if err := syscall.Kill(-cmd.Process.Pid, syscall.SIGTERM); err != nil {
		_ = cmd.Process.Signal(syscall.SIGTERM)
	}
}

// killGroup kills the whole group.
func killGroup(cmd *exec.Cmd) {
	if err := syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL); err != nil {
		_ = cmd.Process.Kill()
	}
}
