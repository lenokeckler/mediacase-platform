package coordinator

import (
	"net/http"
	"os"
	"path/filepath"
	"strings"
)

// El coordinador sirve él mismo el dashboard compilado (dashboard/dist): una sola URL para
// la interfaz, la API (bajo /api/) y /connect. Así ningún nodo necesita nginx ni Docker para
// ver el sistema, y desde otra máquina basta con http://<ip-del-coordinador>:8080.

// spaHandler sirve los archivos del dashboard y devuelve index.html para cualquier ruta que
// no sea un archivo (recarga de una pestaña, rutas del lado del cliente).
func spaHandler(dir string) http.Handler {
	fs := http.FileServer(http.Dir(dir))
	index := filepath.Join(dir, "index.html")
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if _, err := os.Stat(index); err != nil {
			http.Error(w, "dashboard no compilado: correr scripts/build-dashboard.ps1", http.StatusNotFound)
			return
		}
		p := filepath.Join(dir, filepath.Clean("/"+r.URL.Path))
		if st, err := os.Stat(p); err == nil && !st.IsDir() {
			fs.ServeHTTP(w, r)
			return
		}
		if strings.HasPrefix(r.URL.Path, "/assets/") {
			http.NotFound(w, r) // un asset inexistente no debe devolver index.html
			return
		}
		http.ServeFile(w, r, index)
	})
}

// apiPrefixes son las rutas de la API que siguen existiendo sin el prefijo /api
// (las usan el worker, el cliente CLI, los scripts de prueba y la página /connect).
var apiPrefixes = []string{
	"/jobs", "/jobs/", "/cases", "/cases/", "/workers", "/workers/",
	"/stats", "/ws", "/upload", "/dataset", "/connect", "/download/", "/metrics",
}

// Handler compone la API (en / y bajo /api/), el WebSocket y el dashboard estático.
func (a *API) Handler(staticDir string) http.Handler {
	api := a.Router()
	root := http.NewServeMux()
	root.Handle("/api/", http.StripPrefix("/api", api))
	for _, p := range apiPrefixes {
		root.Handle(p, api)
	}
	root.Handle("/", spaHandler(staticDir))
	return root
}
