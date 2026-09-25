package main

import (
	"log"
	"os"
	"runtime"
	"strconv"
	"strings"

	"github.com/shirou/gopsutil/v3/mem"
)

// Capacidad del worker = cuántas sub-tareas procesa a la vez. Con WORKER_POOL_SIZE=auto (o
// vacío) se deriva del hardware, para que una máquina más potente reciba más trabajo en vez de
// quedarse en el mismo número fijo que una laptop chica (Unidad 1: heterogeneidad de cómputo).
const (
	threadsPerSlot   = 2       // cada ffmpeg ya usa varios hilos: un cupo por cada 2 hilos lógicos
	memBytesPerSlot  = 2 << 30 // ~2 GB por conversión pesada (1080p/4K con x264) sin paginar
	minAutoPoolSize  = 1
	maxAutoPoolSize  = 8
	defaultPoolValue = "auto"
)

// autoPoolSize decide la capacidad con el recurso que se agote primero: hilos de CPU o RAM.
// Ej.: 12 hilos y 15 GB → min(6, 7) = 6; una VM de 2 hilos y 2 GB → 1; 32 hilos y 64 GB → 8.
func autoPoolSize(threads int, memTotal uint64) int {
	n := threads / threadsPerSlot
	if memTotal > 0 {
		if byMem := int(memTotal / memBytesPerSlot); byMem < n {
			n = byMem
		}
	}
	if n < minAutoPoolSize {
		n = minAutoPoolSize
	}
	if n > maxAutoPoolSize {
		n = maxAutoPoolSize
	}
	return n
}

// poolSizeFromEnv lee WORKER_POOL_SIZE: un número fija la capacidad a mano (node-1 comparte la
// máquina con Postgres, Redis y MinIO y la deja en 4); "auto", vacío o inválido la calcula.
func poolSizeFromEnv() (size int, auto bool) {
	v := strings.TrimSpace(os.Getenv("WORKER_POOL_SIZE"))
	if n, err := strconv.Atoi(v); err == nil && n > 0 {
		return n, false
	}
	if v != "" && !strings.EqualFold(v, defaultPoolValue) {
		log.Printf("[config] WORKER_POOL_SIZE=%q no es un número: se calcula según el hardware", v)
	}
	var total uint64
	if vm, err := mem.VirtualMemory(); err == nil {
		total = vm.Total
	}
	return autoPoolSize(runtime.NumCPU(), total), true
}
