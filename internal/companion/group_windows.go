//go:build windows

package companion

import "os/exec"

// Windows has no process groups and no SIGTERM: the program is killed once the
// grace period has run out, and a helper it forked is not reached. The station
// link forks none.

func startInOwnGroup(cmd *exec.Cmd) {}

func terminateGroup(cmd *exec.Cmd) {}

func killGroup(cmd *exec.Cmd) {
	_ = cmd.Process.Kill()
}
