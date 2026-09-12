//go:build !windows && !linux

package monitoring

import "os/exec"

func hideWindow(*exec.Cmd) {}

func openProbes() []gpuProbe { return nil }
