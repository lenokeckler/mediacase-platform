package coordinator

import (
	"strings"
	"testing"
)

func TestWorkerEnvFor_DerivaMinIODelHost(t *testing.T) {
	t.Setenv("MINIO_PUBLIC_ENDPOINT", "")
	env := workerEnvFor("172.24.83.164:8080")
	for _, want := range []string{
		"COORDINATOR_URL=http://172.24.83.164:8080",
		"MINIO_ENDPOINT=172.24.83.164:9000",
		"MINIO_PUBLIC_ENDPOINT=172.24.83.164:9000",
		"WORKER_ROLE=all",
	} {
		if !strings.Contains(env, want) {
			t.Errorf("falta %q en:\n%s", want, env)
		}
	}
}

func TestWorkerEnvFor_RespetaMinIOPublicoSiEsRemoto(t *testing.T) {
	t.Setenv("MINIO_PUBLIC_ENDPOINT", "minio.midominio.com:9000")
	env := workerEnvFor("mediacase.midominio.com")
	if !strings.Contains(env, "MINIO_ENDPOINT=minio.midominio.com:9000") {
		t.Errorf("debía respetar el endpoint público configurado:\n%s", env)
	}
	if !strings.Contains(env, "COORDINATOR_URL=http://mediacase.midominio.com\n") {
		t.Errorf("host sin puerto debe quedar tal cual:\n%s", env)
	}
}

func TestWorkerEnvFor_IgnoraLocalhost(t *testing.T) {
	// El coordinador corre con MINIO_PUBLIC_ENDPOINT=localhost:9000 en desarrollo;
	// eso no le sirve a otra máquina: se reemplaza por el host real.
	t.Setenv("MINIO_PUBLIC_ENDPOINT", "localhost:9000")
	env := workerEnvFor("192.168.1.10:8080")
	if strings.Contains(env, "localhost") {
		t.Errorf("no debe filtrar localhost a otra máquina:\n%s", env)
	}
}
