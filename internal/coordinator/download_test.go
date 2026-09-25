package coordinator

import (
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestWorkerEnvFor_DerivaMinIODelHost(t *testing.T) {
	t.Setenv("MINIO_PUBLIC_ENDPOINT", "")
	env := workerEnvFor("172.24.83.164:8080", "http", "", "all")
	for _, want := range []string{
		"COORDINATOR_URL=http://172.24.83.164:8080",
		"MINIO_ENDPOINT=172.24.83.164:9000",
		"MINIO_PUBLIC_ENDPOINT=172.24.83.164:9000",
		"MINIO_USE_SSL=false",
		"WORKER_ROLE=all",
	} {
		if !strings.Contains(env, want) {
			t.Errorf("falta %q en:\n%s", want, env)
		}
	}
}

func TestWorkerEnvFor_RespetaMinIOPublicoSiEsRemoto(t *testing.T) {
	t.Setenv("MINIO_PUBLIC_ENDPOINT", "minio.midominio.com:9000")
	env := workerEnvFor("mediacase.midominio.com", "http", "", "all")
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
	env := workerEnvFor("192.168.1.10:8080", "http", "", "all")
	if strings.Contains(env, "localhost") {
		t.Errorf("no debe filtrar localhost a otra máquina:\n%s", env)
	}
}

func TestWorkerEnvFor_TunelHTTPSConMinIOPorTunel(t *testing.T) {
	// El navegador llegó por un quick tunnel (cloudflared pone X-Forwarded-Proto: https) y
	// scripts/tunnel.ps1 dejó el host del túnel de MinIO: el worker remoto usa https/wss y S3 por TLS.
	t.Setenv("MINIO_PUBLIC_ENDPOINT", "172.24.87.192:9000")
	env := workerEnvFor("abc-def.trycloudflare.com", "https", "minio-xyz.trycloudflare.com", "all")
	for _, want := range []string{
		"COORDINATOR_URL=https://abc-def.trycloudflare.com\n",
		"MINIO_ENDPOINT=minio-xyz.trycloudflare.com\n",
		"MINIO_PUBLIC_ENDPOINT=minio-xyz.trycloudflare.com\n",
		"MINIO_USE_SSL=true",
	} {
		if !strings.Contains(env, want) {
			t.Errorf("falta %q en:\n%s", want, env)
		}
	}
	if strings.Contains(env, "172.24.87.192") {
		t.Errorf("no debe filtrar la IP de LAN a un worker que llega por el túnel:\n%s", env)
	}
}

func TestWorkerEnvFor_TunelSinMinIOAvisa(t *testing.T) {
	// Túnel abierto solo para el 8080: el ZIP sigue sirviendo (wss), pero MinIO queda en la LAN
	// y el archivo lo dice, para que quien lo abra entienda por qué fallan las descargas.
	t.Setenv("MINIO_PUBLIC_ENDPOINT", "172.24.87.192:9000")
	env := workerEnvFor("abc-def.trycloudflare.com", "https", "", "all")
	if !strings.Contains(env, "COORDINATOR_URL=https://abc-def.trycloudflare.com\n") {
		t.Errorf("debía anunciar https:\n%s", env)
	}
	if !strings.Contains(env, "MINIO_ENDPOINT=172.24.87.192:9000") || !strings.Contains(env, "MINIO_USE_SSL=false") {
		t.Errorf("sin túnel de MinIO se queda con el endpoint de LAN sin TLS:\n%s", env)
	}
	if !strings.Contains(env, "# AVISO") {
		t.Errorf("debía avisar que MinIO no es alcanzable por el túnel:\n%s", env)
	}
}

func TestRequestScheme(t *testing.T) {
	cases := []struct{ proto, want string }{{"", "http"}, {"http", "http"}, {"https", "https"}, {"HTTPS", "https"}}
	for _, c := range cases {
		r := httptest.NewRequest("GET", "/connect", nil)
		if c.proto != "" {
			r.Header.Set("X-Forwarded-Proto", c.proto)
		}
		if got := requestScheme(r); got != c.want {
			t.Errorf("X-Forwarded-Proto=%q: got %q, want %q", c.proto, got, c.want)
		}
	}
}

func TestMinIOTunnelEndpoint_LeeElArchivoDelTunel(t *testing.T) {
	dir := t.TempDir()
	f := filepath.Join(dir, "tunnel.env")
	t.Setenv("TUNNEL_ENV_FILE", f)
	if got := minioTunnelEndpoint(); got != "" {
		t.Errorf("sin archivo debe ser vacío, got %q", got)
	}
	content := "# escrito por scripts/tunnel.ps1\r\nCOORDINATOR_TUNNEL_URL=https://a.trycloudflare.com\r\nMINIO_TUNNEL_HOST=minio-xyz.trycloudflare.com\r\n"
	if err := os.WriteFile(f, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	if got := minioTunnelEndpoint(); got != "minio-xyz.trycloudflare.com" {
		t.Errorf("got %q", got)
	}
}

func TestWorkerEnvFor_RolElegidoYCapacidadAutomatica(t *testing.T) {
	env := workerEnvFor("192.168.1.10:8080", "http", "", workerRoleParam("video"))
	for _, want := range []string{"WORKER_ROLE=video\n", "WORKER_POOL_SIZE=auto\n"} {
		if !strings.Contains(env, want) {
			t.Errorf("falta %q en:\n%s", want, env)
		}
	}
	for _, bad := range []string{"", "gpu", "VIDEO; rm -rf /"} {
		if got := workerRoleParam(bad); got != "all" {
			t.Errorf("workerRoleParam(%q) = %q, quería all", bad, got)
		}
	}
}
