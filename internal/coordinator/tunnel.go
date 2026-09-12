package coordinator

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"log"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"sort"
	"strings"
	"sync"
	"time"
)

// Túnel hacia internet manejado por el coordinador, para que node-1 se publique con un botón
// del dashboard en vez de un script. Abre DOS "quick tunnels" de Cloudflare (coordinador y
// MinIO): el worker remoto necesita ambos. cloudflared es un proceso hijo; se cierra con el
// túnel o con el coordinador.
//
// Las URLs cambian en cada arranque: el ZIP de /connect hay que bajarlo con el túnel abierto.

// TunnelState es lo que ve el dashboard.
type TunnelState struct {
	Status         string `json:"status"` // off | starting | on | error
	CoordinatorURL string `json:"coordinator_url,omitempty"`
	MinIOURL       string `json:"minio_url,omitempty"`
	Error          string `json:"error,omitempty"`
	Hint           string `json:"hint,omitempty"`
	StartedAt      string `json:"started_at,omitempty"`
}

// Tunnel controla los dos procesos cloudflared.
type Tunnel struct {
	coordPort, minioPort string

	mu     sync.Mutex
	state  TunnelState
	cancel context.CancelFunc
	done   chan struct{}
}

func NewTunnel(coordPort, minioPort string) *Tunnel {
	return &Tunnel{coordPort: coordPort, minioPort: minioPort, state: TunnelState{Status: "off"}}
}

func (t *Tunnel) State() TunnelState {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.state
}

// MinIOHost es el host del túnel de MinIO cuando está abierto, "" si no. Lo usa el ZIP.
func (t *Tunnel) MinIOHost() string {
	st := t.State()
	if st.Status != "on" || st.MinIOURL == "" {
		return ""
	}
	return strings.TrimPrefix(st.MinIOURL, "https://")
}

// registerTimeout es cuánto se espera a que cloudflared consiga URL y registre la conexión.
// En una red que bloquea el 7844 reintenta para siempre: hay que cortar y explicar.
const registerTimeout = 45 * time.Second

// Start abre los dos túneles. Vuelve enseguida; el estado pasa a "on" o "error" solo.
func (t *Tunnel) Start() error {
	t.mu.Lock()
	if t.state.Status == "starting" || t.state.Status == "on" {
		t.mu.Unlock()
		return nil
	}
	bin, err := findCloudflared()
	if err != nil {
		t.state = TunnelState{Status: "error", Error: err.Error(),
			Hint: "Instalar con: winget install --id Cloudflare.cloudflared  (y reiniciar el coordinador)"}
		t.mu.Unlock()
		return err
	}
	ctx, cancel := context.WithCancel(context.Background())
	t.cancel = cancel
	t.done = make(chan struct{})
	t.state = TunnelState{Status: "starting", StartedAt: time.Now().Format(time.RFC3339)}
	done := t.done
	t.mu.Unlock()

	go t.run(ctx, bin, done)
	return nil
}

// Stop cierra los dos procesos. No es error cerrar un túnel que no está abierto.
func (t *Tunnel) Stop() error {
	t.mu.Lock()
	cancel, done := t.cancel, t.done
	t.cancel, t.done = nil, nil
	t.state = TunnelState{Status: "off"}
	t.mu.Unlock()
	if cancel != nil {
		cancel()
		<-done
	}
	return nil
}

// run lanza los dos cloudflared, lee sus logs y decide el estado. Termina cuando ctx se cancela
// o cuando alguno de los dos procesos muere.
func (t *Tunnel) run(ctx context.Context, bin string, done chan struct{}) {
	defer close(done)
	type proc struct {
		name string
		port string
		cmd  *exec.Cmd
		url  string
		logs strings.Builder
		reg  bool
	}
	procs := []*proc{{name: "coordinator", port: t.coordPort}, {name: "minio", port: t.minioPort}}

	exited := make(chan error, len(procs))
	var logMu sync.Mutex
	for _, p := range procs {
		p.cmd = exec.CommandContext(ctx, bin, "tunnel", "--protocol", "http2", "--url", "http://localhost:"+p.port)
		hideWindow(p.cmd)
		stderr, err := p.cmd.StderrPipe()
		if err == nil {
			err = p.cmd.Start()
		}
		if err != nil {
			t.fail(fmt.Sprintf("no se pudo lanzar cloudflared: %v", err), "")
			for _, q := range procs {
				if q.cmd != nil && q.cmd.Process != nil {
					_ = q.cmd.Process.Kill()
				}
			}
			return
		}
		go func(p *proc, r io.Reader) {
			sc := bufio.NewScanner(r)
			sc.Buffer(make([]byte, 64*1024), 1024*1024)
			for sc.Scan() {
				line := sc.Text()
				logMu.Lock()
				if p.logs.Len() < 64*1024 {
					p.logs.WriteString(line + "\n")
				}
				if u, ok := parseTunnelURL(line); ok && p.url == "" {
					p.url = u
				}
				if strings.Contains(line, "Registered tunnel connection") {
					p.reg = true
				}
				logMu.Unlock()
			}
			exited <- p.cmd.Wait()
		}(p, stderr)
	}

	deadline := time.NewTimer(registerTimeout)
	defer deadline.Stop()
	tick := time.NewTicker(500 * time.Millisecond)
	defer tick.Stop()
	ready := false
	for {
		select {
		case <-ctx.Done():
			return
		case err := <-exited:
			logMu.Lock()
			all := procs[0].logs.String() + procs[1].logs.String()
			logMu.Unlock()
			msg, hint := classifyTunnelError(all)
			if err != nil && !ready {
				msg = fmt.Sprintf("%s (%v)", msg, err)
			}
			if ready {
				msg = "el túnel se cerró solo; abrirlo de nuevo"
			}
			t.fail(msg, hint)
			for _, q := range procs {
				_ = q.cmd.Process.Kill()
			}
			return
		case <-deadline.C:
			if ready {
				continue
			}
			logMu.Lock()
			all := procs[0].logs.String() + procs[1].logs.String()
			logMu.Unlock()
			msg, hint := classifyTunnelError(all)
			t.fail(msg, hint)
			for _, q := range procs {
				_ = q.cmd.Process.Kill()
			}
			return
		case <-tick.C:
			if ready {
				continue
			}
			logMu.Lock()
			ok := procs[0].url != "" && procs[1].url != "" && procs[0].reg && procs[1].reg
			cu, mu := procs[0].url, procs[1].url
			logMu.Unlock()
			if ok {
				ready = true
				t.mu.Lock()
				if t.state.Status == "starting" {
					t.state.Status, t.state.CoordinatorURL, t.state.MinIOURL = "on", cu, mu
				}
				t.mu.Unlock()
				log.Printf("[tunnel] abierto: %s (coordinador) y %s (MinIO)", cu, mu)
			}
		}
	}
}

func (t *Tunnel) fail(msg, hint string) {
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.state.Status == "off" { // lo cerraron a propósito mientras arrancaba
		return
	}
	t.state = TunnelState{Status: "error", Error: msg, Hint: hint}
	log.Printf("[tunnel] error: %s — %s", msg, hint)
}

var tunnelURLRe = regexp.MustCompile(`https://[a-z0-9-]+\.trycloudflare\.com`)

// parseTunnelURL saca la URL pública de una línea de log de cloudflared.
func parseTunnelURL(line string) (string, bool) {
	m := tunnelURLRe.FindString(line)
	return m, m != ""
}

// classifyTunnelError traduce los logs de cloudflared a un mensaje para el dashboard. El caso
// que importa: una red que corta el 7844 (el WiFi del TEC) → la salida es encender WARP.
func classifyTunnelError(logs string) (msg, hint string) {
	l := strings.ToLower(logs)
	switch {
	case strings.Contains(l, "tls handshake with edge") || strings.Contains(l, "failed to dial") ||
		strings.Contains(l, "no recent network activity") || strings.Contains(l, "forcibly closed") ||
		strings.Contains(l, "unable to establish connection"):
		return "no se pudo conectar con Cloudflare: esta red bloquea el puerto 7844 del túnel",
			"Si estás en el WiFi del TEC, encendé Cloudflare WARP (o una VPN) en esta laptop y volvé a intentar."
	case strings.TrimSpace(logs) == "":
		return "cloudflared no respondió a tiempo", "Revisar la conexión a internet y volver a intentar."
	default:
		return "cloudflared falló", "Revisar la conexión a internet y volver a intentar."
	}
}

// findCloudflared busca el binario en PATH y, en Windows, donde lo deja winget (el PATH nuevo no
// llega a un coordinador ya abierto).
func findCloudflared() (string, error) {
	if p, err := exec.LookPath("cloudflared"); err == nil {
		return p, nil
	}
	if runtime.GOOS == "windows" {
		for _, dir := range []string{os.Getenv("ProgramFiles(x86)"), os.Getenv("ProgramFiles")} {
			if dir == "" {
				continue
			}
			p := filepath.Join(dir, "cloudflared", "cloudflared.exe")
			if _, err := os.Stat(p); err == nil {
				return p, nil
			}
		}
	}
	return "", errors.New("cloudflared no está instalado en esta máquina")
}

// ── Direcciones de la LAN para la tarjeta "Compartir" ─────────────────────────────────────

type ifaceAddr struct {
	Name string
	IP   net.IP
}

// virtualIfaceHints son adaptadores que tienen IP privada pero no sirven para que otra PC
// llegue hasta aquí: redes internas de VirtualBox, WSL/Hyper-V y la VPN de WARP.
var virtualIfaceHints = []string{"virtualbox", "vethernet", "wsl", "hyper-v", "warp", "vmware", "docker", "loopback"}

// lanIPv4s filtra las direcciones por las que otra máquina de la red puede llegar al coordinador.
func lanIPv4s(addrs []ifaceAddr) []string {
	var out []string
	for _, a := range addrs {
		ip4 := a.IP.To4()
		if ip4 == nil || ip4.IsLoopback() || !ip4.IsPrivate() {
			continue
		}
		// Red host-only por defecto de VirtualBox: en Windows el adaptador se llama "Ethernet N",
		// así que el nombre no delata nada; solo las VMs de Vagrant llegan por ahí.
		if ip4[0] == 192 && ip4[1] == 168 && ip4[2] == 56 {
			continue
		}
		name := strings.ToLower(a.Name)
		virtual := false
		for _, h := range virtualIfaceHints {
			if strings.Contains(name, h) {
				virtual = true
				break
			}
		}
		if virtual {
			continue
		}
		out = append(out, ip4.String())
	}
	return out
}

func systemLanIPv4s() []string {
	ifaces, err := net.Interfaces()
	if err != nil {
		return nil
	}
	var addrs []ifaceAddr
	for _, ifc := range ifaces {
		if ifc.Flags&net.FlagUp == 0 {
			continue
		}
		as, err := ifc.Addrs()
		if err != nil {
			continue
		}
		for _, a := range as {
			if ipn, ok := a.(*net.IPNet); ok {
				addrs = append(addrs, ifaceAddr{Name: ifc.Name, IP: ipn.IP})
			}
		}
	}
	return lanIPv4s(addrs)
}

// ShareInfo es la respuesta de GET /share: cómo llegar a este coordinador desde otra máquina.
type ShareInfo struct {
	PrimaryURL           string      `json:"primary_url"`
	LanURLs              []string    `json:"lan_urls"`
	Tunnel               TunnelState `json:"tunnel"`
	CloudflaredInstalled bool        `json:"cloudflared_installed"`
}

// buildShareInfo arma las URLs de la LAN. La primaria es la IP que ya se anuncia a los workers
// (MINIO_PUBLIC_ENDPOINT, autodetectada al arrancar); si no hay, la primera de la lista.
func buildShareInfo(ips []string, port string) ShareInfo {
	s := ShareInfo{}
	sort.Strings(ips)
	for _, ip := range ips {
		s.LanURLs = append(s.LanURLs, "http://"+net.JoinHostPort(ip, port))
	}
	if pub := os.Getenv("MINIO_PUBLIC_ENDPOINT"); pub != "" {
		h := pub
		if hh, _, err := net.SplitHostPort(pub); err == nil {
			h = hh
		}
		if h != "" && h != "localhost" && !strings.HasPrefix(h, "127.") {
			s.PrimaryURL = "http://" + net.JoinHostPort(h, port)
		}
	}
	if s.PrimaryURL == "" && len(s.LanURLs) > 0 {
		s.PrimaryURL = s.LanURLs[0]
	}
	return s
}
