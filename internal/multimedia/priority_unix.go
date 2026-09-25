//go:build !windows

package multimedia

import (
	"os/exec"
	"syscall"
)

func lowerPriority(cmd *exec.Cmd) {}

func lowerPriorityStarted(cmd *exec.Cmd) {
	if cmd.Process != nil {
		_ = syscall.Setpriority(syscall.PRIO_PROCESS, cmd.Process.Pid, 10)
	}
}
