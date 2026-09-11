package coordinator

import (
	"encoding/json"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/lenokeckler/mediacase-platform/internal/cases"
	"github.com/lenokeckler/mediacase-platform/internal/storage"
)

// Entradas del sistema: viven en el bucket "dataset" de MinIO. El dashboard sube archivos
// aquí (POST /upload) y lista los existentes (GET /dataset) para armar un caso.

const maxUploadMemory = 64 << 20 // lo que se mantiene en RAM; el resto va a disco temporal

var unsafeChars = regexp.MustCompile(`[^A-Za-z0-9._-]+`)

// safeKey convierte un nombre de archivo en una clave de objeto segura y legible.
func safeKey(name string) string {
	base := filepath.Base(strings.ReplaceAll(name, "\\", "/"))
	base = unsafeChars.ReplaceAllString(base, "_")
	return strings.Trim(base, "._")
}

// uploadFiles: multipart con uno o más campos "file". Cada archivo se valida por tipo
// (routing) y se sube a dataset/<clave>. Devuelve las claves, en el mismo orden.
func (a *API) uploadFiles(w http.ResponseWriter, r *http.Request) {
	if a.minio == nil {
		http.Error(w, "MinIO no disponible en el coordinador", http.StatusServiceUnavailable)
		return
	}
	if err := r.ParseMultipartForm(maxUploadMemory); err != nil {
		http.Error(w, "cannot parse form: "+err.Error(), http.StatusBadRequest)
		return
	}
	headers := r.MultipartForm.File["file"]
	if len(headers) == 0 {
		http.Error(w, "missing file field", http.StatusBadRequest)
		return
	}

	// Validar todo antes de subir nada: la respuesta dice exactamente qué archivo no sirve.
	keys := make([]string, len(headers))
	for i, h := range headers {
		k := safeKey(h.Filename)
		if k == "" {
			http.Error(w, "nombre de archivo inválido: "+h.Filename, http.StatusBadRequest)
			return
		}
		if _, err := cases.DetectFileType(k); err != nil {
			http.Error(w, "archivo "+h.Filename+": "+err.Error(), http.StatusBadRequest)
			return
		}
		keys[i] = k
	}

	tmpDir, err := os.MkdirTemp("", "mediacase-upload-")
	if err != nil {
		http.Error(w, "tmp: "+err.Error(), http.StatusInternalServerError)
		return
	}
	defer os.RemoveAll(tmpDir)

	for i, h := range headers {
		src, err := h.Open()
		if err != nil {
			http.Error(w, "open "+h.Filename+": "+err.Error(), http.StatusBadRequest)
			return
		}
		tmp := filepath.Join(tmpDir, keys[i])
		dst, err := os.Create(tmp)
		if err != nil {
			src.Close()
			http.Error(w, "tmp file: "+err.Error(), http.StatusInternalServerError)
			return
		}
		_, copyErr := io.Copy(dst, src)
		dst.Close()
		src.Close()
		if copyErr != nil {
			http.Error(w, "write "+h.Filename+": "+copyErr.Error(), http.StatusInternalServerError)
			return
		}
		if err := a.minio.UploadObject(r.Context(), storage.DatasetBucket, keys[i], tmp); err != nil {
			log.Printf("[upload] %s: %v", keys[i], err)
			http.Error(w, "upload "+h.Filename+": "+err.Error(), http.StatusBadGateway)
			return
		}
		log.Printf("[upload] dataset/%s (%d bytes) desde %s", keys[i], h.Size, r.RemoteAddr)
	}
	writeJSON(w, http.StatusCreated, map[string]any{"keys": keys})
}

type datasetItem struct {
	Key          string    `json:"key"`
	Size         int64     `json:"size_bytes"`
	Type         string    `json:"type"`
	LastModified time.Time `json:"last_modified"`
}

// listDataset lista las entradas disponibles en el bucket (opcionalmente bajo un prefijo).
func (a *API) listDataset(w http.ResponseWriter, r *http.Request) {
	if a.minio == nil {
		http.Error(w, "MinIO no disponible en el coordinador", http.StatusServiceUnavailable)
		return
	}
	objs, err := a.minio.ListObjects(r.Context(), storage.DatasetBucket, r.URL.Query().Get("prefix"))
	if err != nil {
		http.Error(w, "minio: "+err.Error(), http.StatusBadGateway)
		return
	}
	items := make([]datasetItem, 0, len(objs))
	for _, o := range objs {
		t := "other"
		if ft, err := cases.DetectFileType(o.Key); err == nil {
			t = string(ft)
		}
		items = append(items, datasetItem{Key: o.Key, Size: o.Size, Type: t, LastModified: o.LastModified})
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(items)
}
