package coordinator

import (
	"net/http"
	"os"
)

// getShare dice cómo llegar a este coordinador desde otra máquina: las URLs de la LAN (para
// pegarlas a un compañero en el mismo WiFi) y el estado del túnel (para otra red).
func (a *API) getShare(w http.ResponseWriter, r *http.Request) {
	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}
	info := buildShareInfo(systemLanIPv4s(), port)
	if a.tunnel != nil {
		info.Tunnel = a.tunnel.State()
	} else {
		info.Tunnel = TunnelState{Status: "off"}
	}
	_, err := findCloudflared()
	info.CloudflaredInstalled = err == nil
	writeJSON(w, http.StatusOK, info)
}

// startTunnel abre los túneles (coordinador y MinIO). Responde enseguida con "starting";
// el dashboard consulta /share hasta ver "on" o "error".
func (a *API) startTunnel(w http.ResponseWriter, r *http.Request) {
	if a.tunnel == nil {
		http.Error(w, "túnel no disponible en este coordinador", http.StatusNotImplemented)
		return
	}
	if err := a.tunnel.Start(); err != nil {
		writeJSON(w, http.StatusServiceUnavailable, a.tunnel.State())
		return
	}
	writeJSON(w, http.StatusAccepted, a.tunnel.State())
}

func (a *API) stopTunnel(w http.ResponseWriter, r *http.Request) {
	if a.tunnel != nil {
		_ = a.tunnel.Stop()
	}
	writeJSON(w, http.StatusOK, TunnelState{Status: "off"})
}
