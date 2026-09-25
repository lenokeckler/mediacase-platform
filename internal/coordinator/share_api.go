package coordinator

import (
	"net/http"
	"os"
)

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
