// internal/coordinator/dataset_meta.go
// Metadatos del dataset (consigna §9: "metadatos asociados vía JSON, BD u otra estructura
// equivalente"): GET /dataset los agrega a cada entrada y GET /dataset/test-cases expone los
// casos de prueba ya armados en el manifest, para que el dashboard y `ingest cases --test-cases`
// no dependan de tener el archivo local.
package coordinator

import (
	"context"
	"encoding/json"
	"log"
	"net/http"
	"os"
	"sync"
	"time"

	"github.com/lenokeckler/mediacase-platform/internal/ingest"
	"github.com/lenokeckler/mediacase-platform/internal/storage"
)

// datasetManifestKey es la clave del manifest en el bucket dataset/: la sube `ingest upload`.
const datasetManifestKey = ".manifest.json"

// datasetManifestRefresh es cada cuánto se revisa si el manifest del bucket cambió.
const datasetManifestRefresh = 30 * time.Second

// manifestCache guarda el último manifest leído (de MinIO o, si no está, del archivo local) para
// no bajarlo ni volver a parsearlo en cada GET /dataset.
type manifestCache struct {
	mu            sync.Mutex
	files         map[string]ingest.ManifestFile
	testCases     []ingest.TestCase
	source        string    // "minio" | "local" | ""
	minioModified time.Time // LastModified del objeto en MinIO cuando source == "minio"
	checkedAt     time.Time
}

// datasetManifestPath es el archivo local de respaldo cuando el bucket no tiene manifest todavía
// (coordinador recién levantado sin `ingest upload`, o desarrollo sin MinIO).
func datasetManifestPath() string {
	if p := os.Getenv("DATASET_MANIFEST"); p != "" {
		return p
	}
	return "dataset/manifest.json"
}

// get devuelve los archivos del manifest (por clave) y los casos de prueba, refrescando desde
// MinIO cuando pasó el intervalo y el objeto cambió; si MinIO no tiene el manifest, cae al
// archivo local.
func (c *manifestCache) get(ctx context.Context, minio *storage.MinIOClient) (map[string]ingest.ManifestFile, []ingest.TestCase) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.files != nil && time.Since(c.checkedAt) < datasetManifestRefresh {
		return c.files, c.testCases
	}
	c.checkedAt = time.Now()

	if minio != nil {
		if _, modified, err := minio.StatObject(ctx, storage.DatasetBucket, datasetManifestKey); err == nil {
			if c.source == "minio" && c.files != nil && modified.Equal(c.minioModified) {
				return c.files, c.testCases // sin cambios
			}
			if raw, err := minio.GetObjectBytes(ctx, storage.DatasetBucket, datasetManifestKey); err == nil {
				var m ingest.Manifest
				if err := json.Unmarshal(raw, &m); err == nil {
					c.setFrom(&m)
					c.source, c.minioModified = "minio", modified
					return c.files, c.testCases
				}
				log.Printf("[dataset] manifest de MinIO inválido: %v", err)
			} else {
				log.Printf("[dataset] no se pudo bajar el manifest de MinIO: %v", err)
			}
		}
	}
	if c.source == "minio" && c.files != nil {
		return c.files, c.testCases // MinIO no respondió esta vez: se conserva lo último bueno
	}

	m, err := ingest.LoadManifest(datasetManifestPath())
	if err != nil {
		if c.files == nil {
			c.files, c.testCases = map[string]ingest.ManifestFile{}, nil
		}
		return c.files, c.testCases
	}
	c.setFrom(m)
	c.source = "local"
	return c.files, c.testCases
}

func (c *manifestCache) setFrom(m *ingest.Manifest) {
	files := make(map[string]ingest.ManifestFile, len(m.Files))
	for _, f := range m.Files {
		files[f.Key] = f
	}
	c.files = files
	c.testCases = m.TestCases
}

// datasetTestCases devuelve los casos de prueba definidos en el manifest (test_cases): la
// generación automática ya armada, lista para revisar o enviar tal cual con `ingest cases
// --test-cases`.
func (a *API) datasetTestCases(w http.ResponseWriter, r *http.Request) {
	_, testCases := a.datasetManifest.get(r.Context(), a.minio)
	if testCases == nil {
		testCases = []ingest.TestCase{}
	}
	writeJSON(w, http.StatusOK, testCases)
}
