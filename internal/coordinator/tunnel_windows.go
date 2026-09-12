//go:build windows

package coordinator

import (
	"os/exec"
	"syscall"
)

// hideWindow evita que cada cloudflared abra su propia consola en Windows.
func hideWindow(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true, CreationFlags: 0x08000000} // CREATE_NO_WINDOW
}
