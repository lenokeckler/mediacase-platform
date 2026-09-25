//go:build !windows

package main

// killChildrenWithWorker: en Linux el worker corre bajo systemd, que al reiniciar el servicio
// mata todo su grupo de control (KillMode=control-group), ffmpeg incluido. Ver children_windows.go.
func killChildrenWithWorker() error { return nil }
