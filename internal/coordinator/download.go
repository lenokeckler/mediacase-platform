package coordinator

import (
	"archive/zip"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// Página "Conectar esta PC" y descarga del worker empaquetado.
//
// El ZIP trae el binario, un worker.env ya escrito con la dirección con la que el navegador
// llegó hasta aquí (cabecera Host), un lanzador, y ffmpeg si está empaquetado en dist/.
// Quien lo baja no escribe IPs, no abre puertos y no instala nada: descomprime y ejecuta.

const connectHTML = `<!doctype html>
<html lang="es"><head><meta charset="utf-8"><meta name="viewport" content="width=device-width,initial-scale=1">
<title>MediaCase — conectar esta PC</title>
<link rel="icon" type="image/png" href="/favicon.png">
<style>
  body{font-family:system-ui,sans-serif;max-width:640px;margin:3rem auto;padding:0 1rem;line-height:1.55;color:#1b1b1b;background:#fafafa}
  h1{font-size:1.6rem}
  .b{display:inline-block;padding:.75rem 1.4rem;background:#1DB954;color:#fff;border-radius:8px;text-decoration:none;font-weight:600;margin:.4rem .6rem .4rem 0}
  .b.alt{background:#2b6cb0}
  code{background:#eee;padding:.1rem .35rem;border-radius:4px}
  ol li{margin:.35rem 0}
  .note{font-size:.92rem;color:#555;margin-top:2rem}
  fieldset{border:1px solid #ddd;border-radius:8px;padding:.6rem 1rem 1rem;margin:1rem 0}
  legend{font-weight:600;padding:0 .3rem}
  label{display:block;margin:.35rem 0;cursor:pointer}
  label small{color:#666}
  button.b{border:0;font:inherit;cursor:pointer}
</style></head><body>
<p style="margin:0 0 .5rem"><img src="/logo.png" alt="MediaCase" width="72" height="72"></p>
<h1>Conectar esta PC como worker</h1>
<p>Descargue el worker para su sistema, descomprímalo y ejecútelo. En unos segundos esta máquina aparece en el dashboard y empieza a recibir sub-tareas.</p>
<form method="get" action="/download/worker">
<fieldset><legend>¿Qué va a procesar esta PC?</legend>
  <label><input type="radio" name="role" value="all" checked> <b>Todo</b> <small>— recomendado: toma lo que haga falta</small></label>
  <label><input type="radio" name="role" value="video"> <b>Video</b> <small>— para la máquina más potente (conversiones pesadas, 4K)</small></label>
  <label><input type="radio" name="role" value="audio"> <b>Audio</b> <small>— conversión y extracción de audio</small></label>
  <label><input type="radio" name="role" value="metadata"> <b>Imágenes y metadatos</b> <small>— miniaturas, metadatos, enriquecer</small></label>
  <p style="margin:.6rem 0 0"><small>Aunque elija un tipo, si esta PC está libre también ayuda con los demás. Cuántas sub-tareas procesa a la vez lo calcula sola según sus núcleos y su RAM.</small></p>
</fieldset>
<p><button class="b" name="os" value="windows">Descargar para Windows</button><button class="b alt" name="os" value="linux">Descargar para Linux</button></p>
</form>
<ol>
  <li>Descomprimir el ZIP en cualquier carpeta.</li>
  <li><b>Windows:</b> doble clic en <code>start-worker.bat</code>. &nbsp; <b>Linux:</b> <code>bash start-worker.sh</code></li>
  <li>Deje la ventana abierta mientras quiera que esta PC procese. Cerrarla la desconecta.</li>
</ol>
<p class="note">No hay que instalar nada ni abrir puertos: el worker se conecta <em>hacia</em> el coordinador en <code>%s</code>. Si esta PC pierde la red, reintenta sola.</p>
</body></html>`

func (a *API) connectPage(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	fmt.Fprintf(w, connectHTML, requestScheme(r)+"://"+r.Host)
}

// requestScheme dice cómo llegó el navegador: directo (http) o por un túnel/proxy con TLS
// (cloudflared y cualquier reverse proxy ponen X-Forwarded-Proto). Con https el worker abre wss.
func requestScheme(r *http.Request) string {
	if r.TLS != nil || strings.EqualFold(r.Header.Get("X-Forwarded-Proto"), "https") {
		return "https"
	}
	return "http"
}

// minioTunnelEndpoint devuelve el host del túnel de MinIO que dejó scripts/tunnel.ps1 en
// TUNNEL_ENV_FILE (por defecto infra/env/tunnel.env), o "" si no hay túnel abierto. Se lee en
// cada descarga porque el túnel nace y muere sin reiniciar el coordinador.
func minioTunnelEndpoint() string {
	path := os.Getenv("TUNNEL_ENV_FILE")
	if path == "" {
		path = filepath.Join("infra", "env", "tunnel.env")
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	for _, line := range strings.Split(string(data), "\n") {
		if k, v, ok := strings.Cut(strings.TrimSpace(line), "="); ok && k == "MINIO_TUNNEL_HOST" {
			return strings.TrimSpace(v)
		}
	}
	return ""
}

// downloadWorker arma el ZIP al vuelo para el SO pedido.
func (a *API) downloadWorker(w http.ResponseWriter, r *http.Request) {
	osName := r.URL.Query().Get("os")
	if osName != "windows" && osName != "linux" {
		http.Error(w, "os debe ser windows o linux", http.StatusBadRequest)
		return
	}

	// Rutas de los binarios: si faltan, mejor avisar antes de empezar a escribir el ZIP.
	binPath := filepath.Join("bin", "worker-linux-amd64")
	if osName == "windows" {
		binPath = filepath.Join("bin", "worker-windows-amd64.exe")
	}
	if _, err := os.Stat(binPath); err != nil {
		http.Error(w, "el binario del worker no está compilado ("+binPath+"); correr scripts/build-workers.ps1",
			http.StatusServiceUnavailable)
		return
	}

	minioTunnel := ""
	if a.tunnel != nil {
		minioTunnel = a.tunnel.MinIOHost()
	}
	if minioTunnel == "" {
		minioTunnel = minioTunnelEndpoint() // scripts/tunnel.ps1, el modo manual
	}
	env := workerEnvFor(r.Host, requestScheme(r), minioTunnel, workerRoleParam(r.URL.Query().Get("role")))

	// El servidor corta cualquier respuesta a los 10 s (WriteTimeout). Un ZIP de ~85 MB por WiFi
	// tarda más: esta respuesta recibe su propio plazo sin relajar el del resto de la API.
	if err := http.NewResponseController(w).SetWriteDeadline(time.Now().Add(15 * time.Minute)); err != nil {
		log.Printf("[download] no se pudo extender el plazo de escritura: %v", err)
	}

	w.Header().Set("Content-Type", "application/zip")
	w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="mediacase-worker-%s.zip"`, osName))
	zw := zip.NewWriter(w)
	defer zw.Close()

	addText(zw, "worker.env", env, 0o644)
	if osName == "windows" {
		if err := addFile(zw, "worker.exe", binPath, 0o755); err != nil {
			log.Printf("[download] worker.exe: %v", err)
			return
		}
		// ffmpeg empaquetado (opcional): dist/ffmpeg/windows/ffmpeg.exe + ffprobe.exe
		for _, exe := range []string{"ffmpeg.exe", "ffprobe.exe"} {
			if err := addFile(zw, exe, filepath.Join("dist", "ffmpeg", "windows", exe), 0o755); err != nil {
				log.Printf("[download] sin %s empaquetado (%v); el usuario necesitará ffmpeg en el PATH", exe, err)
			}
		}
		addText(zw, "start-worker.ps1", startWorkerPS1, 0o644)
		addText(zw, "start-worker.bat", startWorkerBAT, 0o644)
	} else {
		if err := addFile(zw, "worker", binPath, 0o755); err != nil {
			log.Printf("[download] worker: %v", err)
			return
		}
		addText(zw, "start-worker.sh", startWorkerSH, 0o755)
	}
	log.Printf("[download] worker para %s entregado a %s (coordinador anunciado: %s://%s)", osName, r.RemoteAddr, requestScheme(r), r.Host)
}

// workerEnvFor genera el worker.env: el coordinador es la dirección (y esquema) con la que
// llegó el navegador; MinIO vive en el mismo host, puerto 9000, salvo que MINIO_PUBLIC_ENDPOINT
// diga otra cosa. Si el navegador llegó por el túnel (https) y hay túnel de MinIO, el worker
// remoto habla S3 por TLS contra ese túnel; si no lo hay, se avisa en el archivo: el 9000 de
// la LAN no es alcanzable desde otra red.
// workerRoleParam valida el rol elegido en /connect; cualquier otra cosa es "all" (genérico).
func workerRoleParam(role string) string {
	switch role {
	case "video", "audio", "metadata":
		return role
	default:
		return "all"
	}
}

func workerEnvFor(host, scheme, minioTunnel, role string) string {
	coordURL := scheme + "://" + host
	minioPub := os.Getenv("MINIO_PUBLIC_ENDPOINT")
	if minioPub == "" || strings.HasPrefix(minioPub, "localhost") || strings.HasPrefix(minioPub, "127.") {
		h := host
		if hh, _, err := net.SplitHostPort(host); err == nil {
			h = hh
		}
		minioPub = net.JoinHostPort(h, "9000")
	}
	useSSL, aviso := "false", ""
	if scheme == "https" {
		if minioTunnel != "" {
			minioPub, useSSL = minioTunnel, "true"
		} else {
			aviso = "# AVISO: llegaste por un tunel pero MinIO no tiene tunel abierto (scripts/tunnel.ps1 abre los dos);\n" +
				"# desde otra red este worker no va a poder bajar entradas ni subir resultados.\n"
		}
	}
	return fmt.Sprintf(`# Generado por el coordinador. Solo hace falta saber donde esta el coordinador.
COORDINATOR_URL=%s
%sMINIO_ENDPOINT=%s
MINIO_PUBLIC_ENDPOINT=%s
MINIO_USE_SSL=%s
MINIO_ACCESS_KEY=%s
MINIO_SECRET_KEY=%s
MINIO_BUCKET=%s
# video | audio | metadata | all: el pool preferido; si esta PC esta libre ayuda con los demas
WORKER_ROLE=%s
# auto = segun nucleos y RAM (1 cada 2 hilos, ~2 GB cada una, tope 8); un numero la fija a mano
WORKER_POOL_SIZE=auto
# WORKER_ID vacio = el lanzador usa el nombre de esta maquina
WORKER_ID=
`, coordURL, aviso, minioPub, minioPub, useSSL,
		envOr("MINIO_ACCESS_KEY", "minioadmin"), envOr("MINIO_SECRET_KEY", "minioadmin"), envOr("MINIO_BUCKET", "results"), role)
}

// Lanzadores. Sin tildes: PowerShell 5.1 lee .ps1 sin BOM como ANSI.
const startWorkerPS1 = "$env:PATH = \"$PSScriptRoot;$env:PATH\"\r\n" +
	"Get-Content \"$PSScriptRoot\\worker.env\" | Where-Object { $_ -match '^\\s*[^#].*=' } | ForEach-Object {\r\n" +
	"    $k, $v = $_ -split '=', 2\r\n" +
	"    if ($v.Trim() -ne '') { [Environment]::SetEnvironmentVariable($k.Trim(), $v.Trim(), 'Process') }\r\n" +
	"}\r\n" +
	"if (-not $env:WORKER_ID) { $env:WORKER_ID = $env:COMPUTERNAME.ToLower() }\r\n" +
	"if (-not (Get-Command ffmpeg -ErrorAction SilentlyContinue)) { Write-Host 'AVISO: no se encontro ffmpeg; las sub-tareas van a fallar' -ForegroundColor Yellow }\r\n" +
	"Write-Host \"MediaCase worker '$env:WORKER_ID' -> $env:COORDINATOR_URL  (Ctrl+C para desconectar)\" -ForegroundColor Green\r\n" +
	"& \"$PSScriptRoot\\worker.exe\"\r\n"

const startWorkerBAT = "@echo off\r\n" +
	"powershell -NoProfile -ExecutionPolicy Bypass -File \"%~dp0start-worker.ps1\"\r\n" +
	"pause\r\n"

const startWorkerSH = `#!/usr/bin/env bash
cd "$(dirname "$0")"
command -v ffmpeg >/dev/null || { echo "Falta ffmpeg: sudo apt install ffmpeg  (Arch: sudo pacman -S ffmpeg)"; exit 1; }
set -a; source ./worker.env; set +a
# uname -n y no hostname: Arch minimo no trae el paquete inetutils.
export WORKER_ID="${WORKER_ID:-$(uname -n)}"
echo "MediaCase worker '$WORKER_ID' -> $COORDINATOR_URL  (Ctrl+C para desconectar)"
exec ./worker
`

func addFile(zw *zip.Writer, name, src string, mode os.FileMode) error {
	f, err := os.Open(src)
	if err != nil {
		return err
	}
	defer f.Close()
	hdr := &zip.FileHeader{Name: name, Method: zip.Deflate}
	hdr.SetMode(mode)
	zf, err := zw.CreateHeader(hdr)
	if err != nil {
		return err
	}
	_, err = io.Copy(zf, f)
	return err
}

func addText(zw *zip.Writer, name, body string, mode os.FileMode) {
	hdr := &zip.FileHeader{Name: name, Method: zip.Deflate}
	hdr.SetMode(mode)
	zf, err := zw.CreateHeader(hdr)
	if err != nil {
		return
	}
	io.WriteString(zf, body)
}

func envOr(k, def string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return def
}
