//go:build windows

package multimedia

import (
	"os/exec"
	"syscall"
)

// BELOW_NORMAL_PRIORITY_CLASS: ffmpeg satura todos los núcleos; con prioridad normal deja sin CPU
// al coordinador, a Postgres y a Redis cuando corren en la misma máquina (node-1), y el worker
// termina "muerto" por heartbeat aunque esté trabajando. Con esta clase el SO atiende primero a
// los procesos de control y ffmpeg usa lo que sobra, que bajo carga es casi todo igual.
const belowNormalPriorityClass = 0x00004000

// lowerPriority hace que el proceso hijo nazca con prioridad por debajo de la normal.
func lowerPriority(cmd *exec.Cmd) {
	if cmd.SysProcAttr == nil {
		cmd.SysProcAttr = &syscall.SysProcAttr{}
	}
	cmd.SysProcAttr.CreationFlags |= belowNormalPriorityClass
}

// lowerPriorityStarted: en Windows la prioridad ya viene en la creación del proceso.
func lowerPriorityStarted(cmd *exec.Cmd) {}
