//go:build windows

package main

import (
	"fmt"
	"unsafe"

	"golang.org/x/sys/windows"
)

// killChildrenWithWorker mete al propio worker en un Job Object con KILL_ON_JOB_CLOSE. Los
// ffmpeg/ffprobe que lance heredan el job, y si el worker muere de golpe (ventana cerrada,
// taskkill /F, corte de luz del proceso) Windows cierra el handle y los mata a todos. Sin esto
// un ffmpeg huérfano seguía convirtiendo y retenía el archivo de entrada, y el reintento de la
// sub-tarea en el worker nuevo fallaba con "Access is denied" al bajar la entrada otra vez.
//
// El handle no se cierra nunca a propósito: vive lo que vive el proceso.
func killChildrenWithWorker() error {
	job, err := windows.CreateJobObject(nil, nil)
	if err != nil {
		return fmt.Errorf("CreateJobObject: %w", err)
	}
	info := windows.JOBOBJECT_EXTENDED_LIMIT_INFORMATION{
		BasicLimitInformation: windows.JOBOBJECT_BASIC_LIMIT_INFORMATION{
			LimitFlags: windows.JOB_OBJECT_LIMIT_KILL_ON_JOB_CLOSE,
		},
	}
	if _, err := windows.SetInformationJobObject(job, windows.JobObjectExtendedLimitInformation,
		uintptr(unsafe.Pointer(&info)), uint32(unsafe.Sizeof(info))); err != nil {
		windows.CloseHandle(job)
		return fmt.Errorf("SetInformationJobObject: %w", err)
	}
	if err := windows.AssignProcessToJobObject(job, windows.CurrentProcess()); err != nil {
		windows.CloseHandle(job)
		return fmt.Errorf("AssignProcessToJobObject: %w", err)
	}
	return nil
}
