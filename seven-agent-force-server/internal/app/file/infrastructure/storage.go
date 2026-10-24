package infrastructure

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"net/url"
	"os"
	"path/filepath"
	"strings"

	"github.com/CaixyPromise/seven-agent/force/internal/app/file/domain"
	"github.com/CaixyPromise/seven-agent/force/internal/shared/config"
)

type LocalStorage struct {
	root       string
	staticPath string
}

func NewLocalStorage(cfg config.StorageConfig) (*LocalStorage, error) {
	root := strings.TrimSpace(cfg.Location)
	if root == "" {
		root = "uploads"
	}
	abs, err := filepath.Abs(root)
	if err != nil {
		return nil, err
	}
	if cfg.ForceCreated {
		if err := os.MkdirAll(abs, 0o755); err != nil {
			return nil, err
		}
	}
	staticPath := strings.TrimSpace(cfg.StaticPath)
	if staticPath == "" {
		staticPath = "/static"
	}
	if !strings.HasPrefix(staticPath, "/") {
		staticPath = "/" + staticPath
	}
	return &LocalStorage{root: abs, staticPath: strings.TrimRight(staticPath, "/")}, nil
}

func (s *LocalStorage) Save(ctx context.Context, relPath string, reader io.Reader, contentType string) (domain.StoredObject, error) {
	if s == nil {
		return domain.StoredObject{}, fmt.Errorf("local storage is not configured")
	}
	clean, err := s.cleanPath(relPath)
	if err != nil {
		return domain.StoredObject{}, err
	}
	target := filepath.Join(s.root, clean)
	if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
		return domain.StoredObject{}, err
	}
	tmp := target + ".tmp"
	file, err := os.Create(tmp)
	if err != nil {
		return domain.StoredObject{}, err
	}
	hash := sha256.New()
	size, copyErr := copyWithContext(ctx, io.MultiWriter(file, hash), reader)
	closeErr := file.Close()
	if copyErr != nil {
		_ = os.Remove(tmp)
		return domain.StoredObject{}, copyErr
	}
	if closeErr != nil {
		_ = os.Remove(tmp)
		return domain.StoredObject{}, closeErr
	}
	if err := os.Rename(tmp, target); err != nil {
		_ = os.Remove(tmp)
		return domain.StoredObject{}, err
	}
	return domain.StoredObject{
		StoragePath: clean,
		Size:        size,
		SHA256:      hex.EncodeToString(hash.Sum(nil)),
		ContentType: contentType,
	}, nil
}

func (s *LocalStorage) Open(file domain.FileInfo) (domain.DownloadObject, error) {
	if s == nil {
		return domain.DownloadObject{}, fmt.Errorf("local storage is not configured")
	}
	clean, err := s.cleanPath(file.StoragePath)
	if err != nil {
		return domain.DownloadObject{}, err
	}
	path := filepath.Join(s.root, clean)
	opened, err := os.Open(path)
	if err != nil {
		return domain.DownloadObject{}, err
	}
	stat, err := opened.Stat()
	if err != nil {
		_ = opened.Close()
		return domain.DownloadObject{}, err
	}
	return domain.DownloadObject{
		File:        opened,
		Size:        stat.Size(),
		ModTime:     stat.ModTime(),
		ContentType: file.ContentType,
		Name:        file.FileInnerName,
	}, nil
}

func (s *LocalStorage) Delete(storagePath string) error {
	if s == nil {
		return nil
	}
	clean, err := s.cleanPath(storagePath)
	if err != nil {
		return err
	}
	if err := os.Remove(filepath.Join(s.root, clean)); err != nil && !os.IsNotExist(err) {
		return err
	}
	return nil
}

func (s *LocalStorage) PublicURL(storagePath string) string {
	if s == nil {
		return ""
	}
	clean, err := s.cleanPath(storagePath)
	if err != nil {
		return ""
	}
	return s.staticPath + "/" + url.PathEscape(strings.ReplaceAll(clean, string(filepath.Separator), "/"))
}

func (s *LocalStorage) cleanPath(relPath string) (string, error) {
	relPath = strings.TrimSpace(strings.ReplaceAll(relPath, "\\", "/"))
	if relPath == "" {
		return "", fmt.Errorf("storage path is empty")
	}
	if strings.HasPrefix(relPath, "/") {
		return "", fmt.Errorf("absolute storage path is not allowed")
	}
	clean := filepath.Clean(relPath)
	if clean == "." || strings.HasPrefix(clean, ".."+string(filepath.Separator)) || clean == ".." {
		return "", fmt.Errorf("invalid storage path")
	}
	return clean, nil
}

func copyWithContext(ctx context.Context, dst io.Writer, src io.Reader) (int64, error) {
	buf := make([]byte, 64*1024)
	var written int64
	for {
		select {
		case <-ctx.Done():
			return written, ctx.Err()
		default:
		}
		n, readErr := src.Read(buf)
		if n > 0 {
			w, writeErr := dst.Write(buf[:n])
			written += int64(w)
			if writeErr != nil {
				return written, writeErr
			}
			if w != n {
				return written, io.ErrShortWrite
			}
		}
		if readErr == io.EOF {
			return written, nil
		}
		if readErr != nil {
			return written, readErr
		}
	}
}
