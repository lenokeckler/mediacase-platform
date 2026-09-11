// internal/storage/minio.go
// Cliente MinIO para subir archivos procesados y generar URLs de descarga.
package storage

import (
	"context"
	"fmt"
	"log"
	"mime"
	"net/http"
	"os"
	"path/filepath"

	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"
)

const defaultBucket = "results"

// MinIOClient encapsula el cliente de MinIO con helpers de gestión de bucket.
type MinIOClient struct {
	client *minio.Client
	bucket string
}

// NewMinIOClient crea un MinIOClient leyendo las variables de entorno.
func NewMinIOClient() (*MinIOClient, error) {
	endpoint  := getEnv("MINIO_ENDPOINT",   "minio:9000")
	accessKey := getEnv("MINIO_ACCESS_KEY", "minioadmin")
	secretKey := getEnv("MINIO_SECRET_KEY", "minioadmin")
	bucket    := getEnv("MINIO_BUCKET",     defaultBucket)
	useSSL    := getEnv("MINIO_USE_SSL",    "false") == "true"

	client, err := minio.New(endpoint, &minio.Options{
		Creds:  credentials.NewStaticV4(accessKey, secretKey, ""),
		Secure: useSSL,
	})
	if err != nil {
		return nil, fmt.Errorf("minio.New: %w", err)
	}

	mc := &MinIOClient{client: client, bucket: bucket}
	if err := mc.ensureBucket(context.Background()); err != nil {
		return nil, err
	}
	log.Printf("[minio] bucket %q listo en %s", bucket, endpoint)
	return mc, nil
}

// ensureBucket crea el bucket si no existe. Tolerante a race conditions en despliegues concurrentes.
func (m *MinIOClient) ensureBucket(ctx context.Context) error {
	exists, err := m.client.BucketExists(ctx, m.bucket)
	if err != nil {
		return fmt.Errorf("bucket check: %w", err)
	}
	
	if !exists {
		err = m.client.MakeBucket(ctx, m.bucket, minio.MakeBucketOptions{})
		if err != nil {
			// Solución a Race Condition: Verificar si otro worker lo creó justo ahora
			existsNow, existsErr := m.client.BucketExists(ctx, m.bucket)
			if existsErr == nil && existsNow {
				log.Printf("[minio] bucket %q ya existe (creado por otro worker concurrente)", m.bucket)
				return nil
			}
			return fmt.Errorf("make bucket: %w", err)
		}
		log.Printf("[minio] bucket %q creado", m.bucket)

		// Política pública de lectura
		policy := fmt.Sprintf(`{
			"Version":"2012-10-17",
			"Statement":[{
				"Effect":"Allow",
				"Principal":{"AWS":["*"]},
				"Action":["s3:GetObject"],
				"Resource":["arn:aws:s3:::%s/*"]
			}]
		}`, m.bucket)
		
		if err := m.client.SetBucketPolicy(ctx, m.bucket, policy); err != nil {
			log.Printf("[minio] advertencia: no se pudo aplicar política pública: %v", err)
		}
	}
	return nil
}

// Upload copia localPath a MinIO y retorna una URL firmada.
func (m *MinIOClient) Upload(ctx context.Context, jobID, localPath string) (string, error) {
	f, err := os.Open(localPath)
	if err != nil {
		return "", fmt.Errorf("open file: %w", err)
	}
	defer f.Close()

	fi, err := f.Stat()
	if err != nil {
		return "", fmt.Errorf("stat file: %w", err)
	}

	ext         := filepath.Ext(localPath)
	contentType := mime.TypeByExtension(ext)
	if contentType == "" {
		contentType = "application/octet-stream"
	}

	objectName := fmt.Sprintf("jobs/%s/%s", jobID, filepath.Base(localPath))

	_, err = m.client.PutObject(ctx, m.bucket, objectName, f, fi.Size(),
		minio.PutObjectOptions{ContentType: contentType})
	if err != nil {
		return "", fmt.Errorf("put object: %w", err)
	}

	// The bucket has a public read policy, so a direct URL is sufficient and
	// avoids the SignatureDoesNotMatch error that occurs when a presigned URL
	// is generated against the internal hostname (minio:9000) but accessed via
	// the public hostname (localhost:9000) — the signature covers the host.
	pubEndpoint := getEnv("MINIO_PUBLIC_ENDPOINT", getEnv("MINIO_ENDPOINT", "minio:9000"))
	directURL := fmt.Sprintf("http://%s/%s/%s", pubEndpoint, m.bucket, objectName)
	return directURL, nil
}

// Ping verifica la conectividad con MinIO.
func (m *MinIOClient) Ping(ctx context.Context) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet,
		fmt.Sprintf("http://%s/minio/health/live",
			getEnv("MINIO_ENDPOINT", "minio:9000")), nil)
	if err != nil {
		return err
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("minio health: %d", resp.StatusCode)
	}
	return nil
}

func getEnv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}