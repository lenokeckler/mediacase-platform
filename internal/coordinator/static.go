package coordinator

import (
	"net/http"
	"os"
	"path/filepath"
	"strings"
)

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
			http.NotFound(w, r)
			return
		}
		http.ServeFile(w, r, index)
	})
}

var apiPrefixes = []string{
	"/jobs", "/jobs/", "/cases", "/cases/", "/workers", "/workers/",
	"/stats", "/ws", "/upload", "/dataset", "/dataset/test-cases", "/connect", "/download/", "/metrics",
}

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
