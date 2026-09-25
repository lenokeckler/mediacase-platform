package main

import (
	"runtime"
	"testing"
)

func TestAutoPoolSize(t *testing.T) {
	const gb = uint64(1) << 30
	tests := []struct {
		name    string
		threads int
		mem     uint64
		want    int
	}{
		{"laptop de Leno: 12 hilos, 15 GB", 12, 15 * gb, 6},
		{"VM chica: 2 hilos, 2 GB", 2, 2 * gb, 1},
		{"poca RAM manda: 16 hilos, 6 GB", 16, 6 * gb, 3},
		{"estación potente: tope 8", 32, 64 * gb, 8},
		{"un solo hilo: mínimo 1", 1, 8 * gb, 1},
		{"RAM desconocida: solo hilos", 8, 0, 4},
	}
	for _, tc := range tests {
		if got := autoPoolSize(tc.threads, tc.mem); got != tc.want {
			t.Errorf("%s: autoPoolSize(%d, %d) = %d, quería %d", tc.name, tc.threads, tc.mem, got, tc.want)
		}
	}
}

func TestPoolSizeFromEnv(t *testing.T) {
	t.Setenv("WORKER_POOL_SIZE", "3")
	if n, auto := poolSizeFromEnv(); n != 3 || auto {
		t.Errorf("fijo: %d auto=%v", n, auto)
	}
	for _, v := range []string{"", "auto", "AUTO", "muchos"} {
		t.Setenv("WORKER_POOL_SIZE", v)
		n, auto := poolSizeFromEnv()
		if !auto || n < minAutoPoolSize || n > maxAutoPoolSize || n > runtime.NumCPU() {
			t.Errorf("%q: %d auto=%v", v, n, auto)
		}
	}
}
