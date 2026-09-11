//go:build !windows

package multimedia

import (
	"os/exec"
	"syscall"
)

// lowerPriority: en Unix no hay flag de creación; se ajusta después de Start (ver abajo).
func lowerPriority(cmd *exec.Cmd) {}

// lowerPriorityStarted baja el nice del proceso hijo recién lanzado (10 = claramente por debajo
// de los procesos de control, sin llegar a "solo cuando nadie más quiere CPU"). Ver priority_windows.go.
func lowerPriorityStarted(cmd *exec.Cmd) {
	if cmd.Process != nil {
		_ = syscall.Setpriority(syscall.PRIO_PROCESS, cmd.Process.Pid, 10)
	}
}
