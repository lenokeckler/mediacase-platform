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

// DatasetBucket guarda los archivos de ENTRADA de los casos. Los workers bajan de aquí
// lo que les toca procesar, así ningún nodo necesita una copia local del dataset.
const DatasetBucket = "dataset"

// MinIOClient encapsula el cliente de MinIO con helpers de gestión de bucket.
type MinIOClient struct {
	client *minio.Client
	bucket string
}

// NewMinIOClient crea un MinIOClient leyendo las variables de entorno.
func NewMinIOClient() (*MinIOClient, error) {
	endpoint := getEnv("MINIO_ENDPOINT", "minio:9000")
	accessKey := getEnv("MINIO_ACCESS_KEY", "minioadmin")
	secretKey := getEnv("MINIO_SECRET_KEY", "minioadmin")
	bucket := getEnv("MINIO_BUCKET", defaultBucket)
	useSSL := getEnv("MINIO_USE_SSL", "false") == "true"

	client, err := minio.New(endpoint, &minio.Options{
		Creds:  credentials.NewStaticV4(accessKey, secretKey, ""),
		Secure: useSSL,
	})
	if err != nil {
		return nil, fmt.Errorf("minio.New: %w", err)
	}

	mc := &MinIOClient{client: client, bucket: bucket}
	if err := mc.EnsureBucket(context.Background(), bucket); err != nil {
		return nil, err
	}
	log.Printf("[minio] bucket %q listo en %s", bucket, endpoint)
	return mc, nil
}

// EnsureBucket crea el bucket si no existe, con lectura pública. Tolerante a race
// conditions cuando varios workers arrancan a la vez.
func (m *MinIOClient) EnsureBucket(ctx context.Context, bucket string) error {
	exists, err := m.client.BucketExists(ctx, bucket)
	if err != nil {
		return fmt.Errorf("bucket check: %w", err)
	}

	if !exists {
		err = m.client.MakeBucket(ctx, bucket, minio.MakeBucketOptions{})
		if err != nil {
			// Solución a Race Condition: Verificar si otro worker lo creó justo ahora
			existsNow, existsErr := m.client.BucketExists(ctx, bucket)
			if existsErr == nil && existsNow {
				log.Printf("[minio] bucket %q ya existe (creado por otro worker concurrente)", bucket)
				return nil
			}
			return fmt.Errorf("make bucket: %w", err)
		}
		log.Printf("[minio] bucket %q creado", bucket)

		// Política pública de lectura
		policy := fmt.Sprintf(`{
			"Version":"2012-10-17",
			"Statement":[{
				"Effect":"Allow",
				"Principal":{"AWS":["*"]},
				"Action":["s3:GetObject"],
				"Resource":["arn:aws:s3:::%s/*"]
			}]
		}`, bucket)

		if err := m.client.SetBucketPolicy(ctx, bucket, policy); err != nil {
			log.Printf("[minio] advertencia: no se pudo aplicar política pública: %v", err)
		}
	}
	return nil
}

// UploadObject sube localPath a bucket/objectKey. Crea el bucket si hace falta.
func (m *MinIOClient) UploadObject(ctx context.Context, bucket, objectKey, localPath string) error {
	if err := m.EnsureBucket(ctx, bucket); err != nil {
		return err
	}
	f, err := os.Open(localPath)
	if err != nil {
		return fmt.Errorf("open file: %w", err)
	}
	defer f.Close()
	fi, err := f.Stat()
	if err != nil {
		return fmt.Errorf("stat file: %w", err)
	}
	contentType := mime.TypeByExtension(filepath.Ext(localPath))
	if contentType == "" {
		contentType = "application/octet-stream"
	}
	_, err = m.client.PutObject(ctx, bucket, objectKey, f, fi.Size(),
		minio.PutObjectOptions{ContentType: contentType})
	if err != nil {
		return fmt.Errorf("put object %s/%s: %w", bucket, objectKey, err)
	}
	return nil
}

// Download baja bucket/objectKey a destDir y retorna la ruta local.
// Conserva el nombre y la extensión originales para que ffmpeg los reconozca.
func (m *MinIOClient) Download(ctx context.Context, bucket, objectKey, destDir string) (string, error) {
	if err := os.MkdirAll(destDir, 0o755); err != nil {
		return "", fmt.Errorf("mkdir %s: %w", destDir, err)
	}
	local := filepath.Join(destDir, filepath.Base(objectKey))
	if err := m.client.FGetObject(ctx, bucket, objectKey, local, minio.GetObjectOptions{}); err != nil {
		return "", fmt.Errorf("get object %s/%s: %w", bucket, objectKey, err)
	}
	return local, nil
}

// Upload sube un resultado al bucket de resultados y retorna su URL pública.
func (m *MinIOClient) Upload(ctx context.Context, jobID, localPath string) (string, error) {
	objectName := fmt.Sprintf("jobs/%s/%s", jobID, filepath.Base(localPath))
	if err := m.UploadObject(ctx, m.bucket, objectName, localPath); err != nil {
		return "", err
	}
	// El bucket tiene lectura pública, así que basta una URL directa. Se usa el endpoint
	// público (no el interno) porque una URL firmada contra "minio:9000" no valida desde afuera.
	pubEndpoint := getEnv("MINIO_PUBLIC_ENDPOINT", getEnv("MINIO_ENDPOINT", "minio:9000"))
	return fmt.Sprintf("http://%s/%s/%s", pubEndpoint, m.bucket, objectName), nil
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
