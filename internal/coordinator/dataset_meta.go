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

const datasetManifestKey = ".manifest.json"

const datasetManifestRefresh = 30 * time.Second

type manifestCache struct {
	mu            sync.Mutex
	files         map[string]ingest.ManifestFile
	testCases     []ingest.TestCase
	source        string
	minioModified time.Time
	checkedAt     time.Time
}

func datasetManifestPath() string {
	if p := os.Getenv("DATASET_MANIFEST"); p != "" {
		return p
	}
	return "dataset/manifest.json"
}

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
				return c.files, c.testCases
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
		return c.files, c.testCases
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

func (a *API) datasetTestCases(w http.ResponseWriter, r *http.Request) {
	_, testCases := a.datasetManifest.get(r.Context(), a.minio)
	if testCases == nil {
		testCases = []ingest.TestCase{}
	}
	writeJSON(w, http.StatusOK, testCases)
}
