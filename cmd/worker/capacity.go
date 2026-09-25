package main

import (
	"log"
	"os"
	"runtime"
	"strconv"
	"strings"

	"github.com/shirou/gopsutil/v3/mem"
)

const (
	threadsPerSlot   = 2
	memBytesPerSlot  = 2 << 30
	minAutoPoolSize  = 1
	maxAutoPoolSize  = 8
	defaultPoolValue = "auto"
)

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
