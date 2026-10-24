package infrastructure

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/url"
	"strings"
	"time"

	"github.com/CaixyPromise/seven-agent/force/internal/app/file/domain"
	"github.com/CaixyPromise/seven-agent/force/internal/shared/config"
	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"
)

type StorageRouter struct {
	local *LocalStorage
}

type S3StrategyConfig struct {
	Endpoint        string `json:"endpoint"`
	Bucket          string `json:"bucket"`
	BucketName      string `json:"bucketName"`
	AccessKey       string `json:"accessKey"`
	AccessKeyID     string `json:"accessKeyId"`
	SecretKey       string `json:"secretKey"`
	SecretID        string `json:"secretId"`
	SecretAccessKey string `json:"secretAccessKey"`
	AccessKeySecret string `json:"accessKeySecret"`
	Region          string `json:"region"`
	UseSSL          bool   `json:"useSSL"`
	PathStyle       bool   `json:"pathStyle"`
	PublicBaseURL   string `json:"publicBaseUrl"`
}

func NewStorageRouter(cfg config.StorageConfig) (*StorageRouter, error) {
	local, err := NewLocalStorage(cfg)
	if err != nil {
		return nil, err
	}
	return &StorageRouter{local: local}, nil
}

func (r *StorageRouter) Save(ctx context.Context, strategy domain.StorageStrategy, storagePath string, reader io.Reader, contentType string) (domain.StoredObject, error) {
	switch normalizedProvider(strategy.ProviderType) {
	case "", domain.ProviderLocal:
		return r.local.Save(ctx, storagePath, reader, contentType)
	case domain.ProviderAWSS3, domain.ProviderAliyunOSS, domain.ProviderTencentCOS:
		client, cfg, err := r.s3Client(strategy)
		if err != nil {
			return domain.StoredObject{}, err
		}
		object, err := r.local.Save(ctx, "tmp/s3-buffer/"+time.Now().Format("20060102150405.000000000"), reader, contentType)
		if err != nil {
			return domain.StoredObject{}, err
		}
		defer r.local.Delete(object.StoragePath)
		opened, err := r.local.Open(domain.FileInfo{StoragePath: object.StoragePath, ContentType: contentType})
		if err != nil {
			return domain.StoredObject{}, err
		}
		defer opened.File.Close()
		_, err = client.PutObject(ctx, cfg.Bucket, storagePath, opened.File, object.Size, minio.PutObjectOptions{ContentType: contentType})
		if err != nil {
			return domain.StoredObject{}, err
		}
		object.StoragePath = storagePath
		return object, nil
	default:
		return domain.StoredObject{}, fmt.Errorf("unsupported storage provider %s", strategy.ProviderType)
	}
}

func (r *StorageRouter) Open(ctx context.Context, strategy domain.StorageStrategy, file domain.FileInfo) (domain.DownloadObject, error) {
	switch normalizedProvider(strategy.ProviderType) {
	case "", domain.ProviderLocal:
		return r.local.Open(file)
	case domain.ProviderAWSS3, domain.ProviderAliyunOSS, domain.ProviderTencentCOS:
		client, cfg, err := r.s3Client(strategy)
		if err != nil {
			return domain.DownloadObject{}, err
		}
		object, err := client.GetObject(ctx, cfg.Bucket, file.StoragePath, minio.GetObjectOptions{})
		if err != nil {
			return domain.DownloadObject{}, err
		}
		stat, err := object.Stat()
		if err != nil {
			_ = object.Close()
			return domain.DownloadObject{}, err
		}
		return domain.DownloadObject{File: object, Size: stat.Size, ModTime: stat.LastModified, ContentType: file.ContentType, Name: file.FileInnerName}, nil
	default:
		return domain.DownloadObject{}, fmt.Errorf("unsupported storage provider %s", strategy.ProviderType)
	}
}

func (r *StorageRouter) Delete(ctx context.Context, strategy domain.StorageStrategy, storagePath string) error {
	switch normalizedProvider(strategy.ProviderType) {
	case "", domain.ProviderLocal:
		return r.local.Delete(storagePath)
	case domain.ProviderAWSS3, domain.ProviderAliyunOSS, domain.ProviderTencentCOS:
		client, cfg, err := r.s3Client(strategy)
		if err != nil {
			return err
		}
		return client.RemoveObject(ctx, cfg.Bucket, storagePath, minio.RemoveObjectOptions{})
	default:
		return fmt.Errorf("unsupported storage provider %s", strategy.ProviderType)
	}
}

func (r *StorageRouter) PublicURL(strategy domain.StorageStrategy, storagePath string) string {
	switch normalizedProvider(strategy.ProviderType) {
	case "", domain.ProviderLocal:
		return r.local.PublicURL(storagePath)
	case domain.ProviderAWSS3, domain.ProviderAliyunOSS, domain.ProviderTencentCOS:
		cfg, err := parseS3Config(strategy)
		if err != nil || strings.TrimSpace(cfg.PublicBaseURL) == "" {
			return ""
		}
		base := strings.TrimRight(cfg.PublicBaseURL, "/")
		escaped := (&url.URL{Path: "/" + strings.TrimLeft(storagePath, "/")}).EscapedPath()
		return base + escaped
	default:
		return ""
	}
}

func (r *StorageRouter) PresignPut(ctx context.Context, strategy domain.StorageStrategy, storagePath, contentType string, ttl time.Duration) (string, error) {
	switch normalizedProvider(strategy.ProviderType) {
	case "", domain.ProviderLocal:
		return "", fmt.Errorf("local storage does not support presigned upload")
	case domain.ProviderAWSS3, domain.ProviderAliyunOSS, domain.ProviderTencentCOS:
		client, cfg, err := r.s3Client(strategy)
		if err != nil {
			return "", err
		}
		if ttl <= 0 {
			ttl = 15 * time.Minute
		}
		signed, err := client.PresignedPutObject(ctx, cfg.Bucket, storagePath, ttl)
		if err != nil {
			return "", err
		}
		return signed.String(), nil
	default:
		return "", fmt.Errorf("unsupported storage provider %s", strategy.ProviderType)
	}
}

func (r *StorageRouter) Health(ctx context.Context, strategy domain.StorageStrategy) error {
	switch normalizedProvider(strategy.ProviderType) {
	case "", domain.ProviderLocal:
		return nil
	case domain.ProviderAWSS3, domain.ProviderAliyunOSS, domain.ProviderTencentCOS:
		client, cfg, err := r.s3Client(strategy)
		if err != nil {
			return err
		}
		exists, err := client.BucketExists(ctx, cfg.Bucket)
		if err != nil {
			return err
		}
		if !exists {
			return fmt.Errorf("bucket %s does not exist", cfg.Bucket)
		}
		return nil
	default:
		return fmt.Errorf("unsupported storage provider %s", strategy.ProviderType)
	}
}

func normalizedProvider(provider string) string {
	return strings.ToUpper(strings.TrimSpace(provider))
}

func (r *StorageRouter) s3Client(strategy domain.StorageStrategy) (*minio.Client, S3StrategyConfig, error) {
	cfg, err := parseS3Config(strategy)
	if err != nil {
		return nil, cfg, err
	}
	client, err := minio.New(cfg.Endpoint, &minio.Options{Creds: credentials.NewStaticV4(cfg.AccessKey, cfg.SecretKey, ""), Secure: cfg.UseSSL, Region: cfg.Region})
	return client, cfg, err
}

func parseS3Config(strategy domain.StorageStrategy) (S3StrategyConfig, error) {
	var cfg S3StrategyConfig
	if err := json.Unmarshal([]byte(strategy.ConfigCiphertext), &cfg); err != nil {
		return cfg, fmt.Errorf("invalid s3 storage config: %w", err)
	}
	endpoint := strings.TrimSpace(cfg.Endpoint)
	if strings.HasPrefix(endpoint, "https://") {
		cfg.UseSSL = true
	}
	cfg.Endpoint = strings.TrimPrefix(strings.TrimPrefix(endpoint, "http://"), "https://")
	cfg.Bucket = firstNonBlank(cfg.Bucket, cfg.BucketName)
	cfg.AccessKey = firstNonBlank(cfg.AccessKey, cfg.AccessKeyID, cfg.SecretID)
	cfg.SecretKey = firstNonBlank(cfg.SecretKey, cfg.SecretAccessKey, cfg.AccessKeySecret)
	if strings.TrimSpace(cfg.Endpoint) == "" || strings.TrimSpace(cfg.Bucket) == "" || strings.TrimSpace(cfg.AccessKey) == "" || strings.TrimSpace(cfg.SecretKey) == "" {
		return cfg, fmt.Errorf("s3 storage config requires endpoint, bucket, accessKey and secretKey")
	}
	return cfg, nil
}

func firstNonBlank(values ...string) string {
	for _, value := range values {
		if trimmed := strings.TrimSpace(value); trimmed != "" {
			return trimmed
		}
	}
	return ""
}
