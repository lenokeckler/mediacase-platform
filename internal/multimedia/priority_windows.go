//go:build windows

package multimedia

import (
	"os/exec"
	"syscall"
)

const belowNormalPriorityClass = 0x00004000

func lowerPriority(cmd *exec.Cmd) {
	if cmd.SysProcAttr == nil {
		cmd.SysProcAttr = &syscall.SysProcAttr{}
	}
	cmd.SysProcAttr.CreationFlags |= belowNormalPriorityClass
}

func lowerPriorityStarted(cmd *exec.Cmd) {}
