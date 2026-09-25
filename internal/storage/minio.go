package storage

import (
	"context"
	"fmt"
	"io"
	"log"
	"mime"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"
)

const defaultBucket = "results"

const DatasetBucket = "dataset"

type MinIOClient struct {
	client *minio.Client
	bucket string
}

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

func (m *MinIOClient) EnsureBucket(ctx context.Context, bucket string) error {
	exists, err := m.client.BucketExists(ctx, bucket)
	if err != nil {
		return fmt.Errorf("bucket check: %w", err)
	}

	if !exists {
		err = m.client.MakeBucket(ctx, bucket, minio.MakeBucketOptions{})
		if err != nil {

			existsNow, existsErr := m.client.BucketExists(ctx, bucket)
			if existsErr == nil && existsNow {
				log.Printf("[minio] bucket %q ya existe (creado por otro worker concurrente)", bucket)
				return nil
			}
			return fmt.Errorf("make bucket: %w", err)
		}
		log.Printf("[minio] bucket %q creado", bucket)

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

func (m *MinIOClient) GetHead(ctx context.Context, bucket, objectKey string, n int64) ([]byte, error) {
	info, err := m.client.StatObject(ctx, bucket, objectKey, minio.StatObjectOptions{})
	if err != nil {
		return nil, fmt.Errorf("stat object %s/%s: %w", bucket, objectKey, err)
	}
	if info.Size == 0 {
		return nil, nil
	}
	opts := minio.GetObjectOptions{}
	if info.Size > n {
		if err := opts.SetRange(0, n-1); err != nil {
			return nil, fmt.Errorf("set range %s/%s: %w", bucket, objectKey, err)
		}
	}
	obj, err := m.client.GetObject(ctx, bucket, objectKey, opts)
	if err != nil {
		return nil, fmt.Errorf("get object %s/%s: %w", bucket, objectKey, err)
	}
	defer obj.Close()
	limit := n
	if info.Size < limit {
		limit = info.Size
	}
	data, err := io.ReadAll(io.LimitReader(obj, limit))
	if err != nil {
		return nil, fmt.Errorf("read head %s/%s: %w", bucket, objectKey, err)
	}
	return data, nil
}

func (m *MinIOClient) StatObject(ctx context.Context, bucket, objectKey string) (size int64, lastModified time.Time, err error) {
	info, err := m.client.StatObject(ctx, bucket, objectKey, minio.StatObjectOptions{})
	if err != nil {
		return 0, time.Time{}, fmt.Errorf("stat object %s/%s: %w", bucket, objectKey, err)
	}
	return info.Size, info.LastModified, nil
}

func (m *MinIOClient) GetObjectBytes(ctx context.Context, bucket, objectKey string) ([]byte, error) {
	obj, err := m.client.GetObject(ctx, bucket, objectKey, minio.GetObjectOptions{})
	if err != nil {
		return nil, fmt.Errorf("get object %s/%s: %w", bucket, objectKey, err)
	}
	defer obj.Close()
	data, err := io.ReadAll(obj)
	if err != nil {
		return nil, fmt.Errorf("read object %s/%s: %w", bucket, objectKey, err)
	}
	return data, nil
}

type ObjectInfo struct {
	Key          string
	Size         int64
	LastModified time.Time
}

func (m *MinIOClient) ListObjects(ctx context.Context, bucket, prefix string) ([]ObjectInfo, error) {
	exists, err := m.client.BucketExists(ctx, bucket)
	if err != nil {
		return nil, fmt.Errorf("bucket check: %w", err)
	}
	if !exists {
		return []ObjectInfo{}, nil
	}
	var out []ObjectInfo
	for o := range m.client.ListObjects(ctx, bucket, minio.ListObjectsOptions{Prefix: prefix, Recursive: true}) {
		if o.Err != nil {
			return nil, o.Err
		}
		out = append(out, ObjectInfo{Key: o.Key, Size: o.Size, LastModified: o.LastModified})
	}
	if out == nil {
		out = []ObjectInfo{}
	}
	return out, nil
}

func (m *MinIOClient) Upload(ctx context.Context, jobID, localPath string) (string, error) {
	objectName := fmt.Sprintf("jobs/%s/%s", jobID, filepath.Base(localPath))
	if err := m.UploadObject(ctx, m.bucket, objectName, localPath); err != nil {
		return "", err
	}

	pubEndpoint := getEnv("MINIO_PUBLIC_ENDPOINT", getEnv("MINIO_ENDPOINT", "minio:9000"))
	return fmt.Sprintf("%s://%s/%s/%s", scheme(), pubEndpoint, m.bucket, objectName), nil
}

func (m *MinIOClient) Ping(ctx context.Context) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet,
		fmt.Sprintf("%s://%s/minio/health/live", scheme(),
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

func scheme() string {
	if getEnv("MINIO_USE_SSL", "false") == "true" {
		return "https"
	}
	return "http"
}

func getEnv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
