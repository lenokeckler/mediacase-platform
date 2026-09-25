package coordinator

import (
	"net"
	"strings"
	"testing"
)

func TestParseTunnelURL(t *testing.T) {
	line := `2026-09-12T00:19:12Z INF +--------------------------------------------------------------------------------------------+
2026-09-12T00:19:12Z INF |  https://bool-arab-exists-handles.trycloudflare.com                                        |`
	got, ok := parseTunnelURL(line)
	if !ok || got != "https://bool-arab-exists-handles.trycloudflare.com" {
		t.Fatalf("got %q ok=%v", got, ok)
	}
	if _, ok := parseTunnelURL("INF Registered tunnel connection connIndex=0"); ok {
		t.Errorf("una línea sin URL no debe dar URL")
	}
}

func TestClassifyTunnelError_RedBloqueadaSugiereWARP(t *testing.T) {
	logs := `ERR Failed to dial a quic connection error="failed to dial to edge with quic: timeout: no recent network activity"
ERR Serve tunnel error error="TLS handshake with edge error: read tcp 172.24.87.192:63364->198.41.192.227:7844: wsarecv: An existing connection was forcibly closed by the remote host."`
	msg, hint := classifyTunnelError(logs)
	if !strings.Contains(msg, "Cloudflare") {
		t.Errorf("mensaje: %q", msg)
	}
	if !strings.Contains(hint, "WARP") {
		t.Errorf("con el 7844 bloqueado la pista debe mencionar WARP: %q", hint)
	}
}

func TestClassifyTunnelError_SinPistaCuandoNoEsLaRed(t *testing.T) {
	msg, hint := classifyTunnelError("ERR something else went wrong")
	if msg == "" {
		t.Errorf("siempre hay mensaje")
	}
	if strings.Contains(hint, "WARP") {
		t.Errorf("un error que no es de red no debe culpar a la red: %q", hint)
	}
}

func TestLanIPv4s_FiltraLoopbackVirtualesYNoPrivadas(t *testing.T) {
	in := []ifaceAddr{
		{Name: "Wi-Fi", IP: net.ParseIP("172.24.87.192")},
		{Name: "Loopback Pseudo-Interface 1", IP: net.ParseIP("127.0.0.1")},
		{Name: "VirtualBox Host-Only Ethernet Adapter", IP: net.ParseIP("192.168.56.1")},
		{Name: "Ethernet 3", IP: net.ParseIP("192.168.56.1")},
		{Name: "vEthernet (WSL (Hyper-V firewall))", IP: net.ParseIP("172.31.0.1")},
		{Name: "CloudflareWARP", IP: net.ParseIP("172.16.0.2")},
		{Name: "Wi-Fi", IP: net.ParseIP("fe80::1")},
		{Name: "Ethernet", IP: net.ParseIP("8.8.8.8")},
		{Name: "eth0", IP: net.ParseIP("192.168.1.20")},
	}
	got := lanIPv4s(in)
	want := []string{"172.24.87.192", "192.168.1.20"}
	if strings.Join(got, ",") != strings.Join(want, ",") {
		t.Errorf("got %v, want %v", got, want)
	}
}

func TestShareInfo_PrimariaDesdeMinIOPublico(t *testing.T) {
	t.Setenv("MINIO_PUBLIC_ENDPOINT", "192.168.1.20:9000")
	s := buildShareInfo([]string{"172.24.87.192", "192.168.1.20"}, "8080")
	if s.PrimaryURL != "http://192.168.1.20:8080" {
		t.Errorf("la primaria debe ser la IP anunciada a los workers: %q", s.PrimaryURL)
	}
	if len(s.LanURLs) != 2 || s.LanURLs[0] != "http://172.24.87.192:8080" {
		t.Errorf("lan_urls: %v", s.LanURLs)
	}
	t.Setenv("MINIO_PUBLIC_ENDPOINT", "localhost:9000")
	s = buildShareInfo([]string{"172.24.87.192"}, "8080")
	if s.PrimaryURL != "http://172.24.87.192:8080" {
		t.Errorf("sin IP anunciada, la primaria es la primera de la LAN: %q", s.PrimaryURL)
	}
}

func TestTunnel_EstadoInicialApagado(t *testing.T) {
	tn := NewTunnel("8080", "9000")
	st := tn.State()
	if st.Status != "off" || tn.MinIOHost() != "" {
		t.Errorf("estado inicial: %+v host=%q", st, tn.MinIOHost())
	}
	if err := tn.Stop(); err != nil {
		t.Errorf("Stop sin túnel abierto no es error: %v", err)
	}
}
