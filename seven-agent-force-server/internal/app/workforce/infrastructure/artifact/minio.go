package artifact

import (
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	"time"

	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"

	"github.com/CaixyPromise/seven-agent/force/internal/app/workforce/runtimeconfig"
)

type Store struct {
	client *minio.Client
	bucket string
}

func Open(ctx context.Context, cfg config.ArtifactConfig) (*Store, error) {
	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.DialContext = (&net.Dialer{Timeout: 5 * time.Second, KeepAlive: 30 * time.Second}).DialContext
	transport.TLSHandshakeTimeout = 5 * time.Second
	transport.ResponseHeaderTimeout = 15 * time.Second
	transport.IdleConnTimeout = 30 * time.Second
	transport.ExpectContinueTimeout = time.Second
	client, err := minio.New(cfg.Endpoint, &minio.Options{
		Creds:      credentials.NewStaticV4(cfg.AccessKey, cfg.SecretKey, ""),
		Secure:     cfg.UseTLS,
		Transport:  transport,
		MaxRetries: 1,
	})
	if err != nil {
		return nil, fmt.Errorf("create minio client: %w", err)
	}
	exists, err := client.BucketExists(ctx, cfg.Bucket)
	if err != nil {
		return nil, fmt.Errorf("check artifact bucket: %w", err)
	}
	if !exists {
		if err := client.MakeBucket(ctx, cfg.Bucket, minio.MakeBucketOptions{}); err != nil {
			return nil, fmt.Errorf("create artifact bucket: %w", err)
		}
	}
	return &Store{client: client, bucket: cfg.Bucket}, nil
}

func (s *Store) Ready(ctx context.Context) error {
	if s == nil || s.client == nil {
		return fmt.Errorf("artifact store is unavailable")
	}
	_, err := s.client.BucketExists(ctx, s.bucket)
	return err
}

func (s *Store) Put(ctx context.Context, key, mediaType string, reader io.Reader, size int64) error {
	_, err := s.client.PutObject(ctx, s.bucket, key, reader, size, minio.PutObjectOptions{ContentType: mediaType})
	if err != nil {
		return fmt.Errorf("put artifact %q: %w", key, err)
	}
	return nil
}

func (s *Store) Get(ctx context.Context, key string) (*minio.Object, error) {
	object, _, err := s.GetWithSize(ctx, key)
	return object, err
}

// GetWithSize opens an object only after MinIO has returned its current
// metadata. Callers that publish a Content-Length can compare this size with
// their already-verified database contract before starting a response stream.
func (s *Store) GetWithSize(ctx context.Context, key string) (*minio.Object, int64, error) {
	object, err := s.client.GetObject(ctx, s.bucket, key, minio.GetObjectOptions{})
	if err != nil {
		return nil, 0, fmt.Errorf("get artifact %q: %w", key, err)
	}
	info, err := object.Stat()
	if err != nil {
		object.Close()
		return nil, 0, fmt.Errorf("stat artifact %q: %w", key, err)
	}
	return object, info.Size, nil
}
