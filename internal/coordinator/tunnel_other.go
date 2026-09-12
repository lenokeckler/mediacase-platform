//go:build !windows

package coordinator

import "os/exec"

func hideWindow(*exec.Cmd) {}
