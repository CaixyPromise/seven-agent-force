package application

import (
	"context"
	"encoding/hex"
	"errors"
	"io"
	"mime"
	"strings"
	"unicode/utf8"

	artifactstore "github.com/CaixyPromise/seven-agent/force/internal/app/workforce/infrastructure/artifact"
	"github.com/CaixyPromise/seven-agent/force/internal/app/workforce/resultprocessor/facade"
	"github.com/CaixyPromise/seven-agent/force/internal/app/workforce/resultprocessor/infrastructure"
)

const maxPublicOutputNameBytes = 128

type availableOutputRepository interface {
	ListAvailableOutputs(ctx context.Context, tenantID, taskID int64) ([]infrastructure.AvailableOutputRecord, error)
	AvailableOutput(ctx context.Context, tenantID, taskID, outputID int64) (infrastructure.AvailableOutputRecord, error)
}

type outputObjectReader interface {
	Open(ctx context.Context, objectKey string) (io.ReadCloser, int64, error)
}

type artifactOutputReader struct {
	store *artifactstore.Store
}

func (r artifactOutputReader) Open(ctx context.Context, objectKey string) (io.ReadCloser, int64, error) {
	if r.store == nil {
		return nil, 0, facade.ErrOutputUnavailable
	}
	return r.store.GetWithSize(ctx, objectKey)
}

func (s *Service) ListAvailableOutputs(ctx context.Context, tenantID, taskID int64) ([]facade.AvailableOutput, error) {
	if s == nil || s.outputStore == nil {
		return nil, facade.ErrOutputUnavailable
	}
	records, err := s.outputStore.ListAvailableOutputs(ctx, tenantID, taskID)
	if err != nil {
		return nil, err
	}
	if len(records) > maxAvailableOutputCount {
		records = records[:maxAvailableOutputCount]
	}
	items := make([]facade.AvailableOutput, 0, len(records))
	for _, record := range records {
		if !validAvailableOutputRecord(record, taskID) {
			continue
		}
		items = append(items, publicAvailableOutput(record.Output))
	}
	return items, nil
}

func (s *Service) OpenAvailableOutput(ctx context.Context, tenantID, taskID, outputID int64) (facade.OutputDownload, error) {
	if s == nil || s.outputStore == nil {
		return facade.OutputDownload{}, facade.ErrOutputUnavailable
	}
	record, err := s.outputStore.AvailableOutput(ctx, tenantID, taskID, outputID)
	if errors.Is(err, infrastructure.ErrNotFound) {
		return facade.OutputDownload{}, facade.ErrOutputNotFound
	}
	if err != nil {
		return facade.OutputDownload{}, err
	}
	if !validAvailableOutputRecord(record, taskID) || record.Output.ID != outputID {
		return facade.OutputDownload{}, facade.ErrOutputNotFound
	}
	if s.outputObjects == nil {
		return facade.OutputDownload{}, facade.ErrOutputUnavailable
	}
	content, objectSize, err := s.outputObjects.Open(ctx, record.ObjectKey)
	if err != nil || content == nil {
		// The object-store adapter can include a private object key in its error.
		// Keep that detail inside this module and return only the stable public
		// classification to callers.
		return facade.OutputDownload{}, facade.ErrOutputUnavailable
	}
	if objectSize != record.Output.SizeBytes {
		_ = content.Close()
		return facade.OutputDownload{}, facade.ErrOutputUnavailable
	}
	return facade.OutputDownload{Output: publicAvailableOutput(record.Output), Content: content}, nil
}

func publicAvailableOutput(output facade.AvailableOutput) facade.AvailableOutput {
	output.Name = sanitizeOutputName(output.Name)
	output.MediaType = sanitizeOutputMediaType(output.MediaType)
	return output
}

const maxAvailableOutputCount = 64

func validAvailableOutputRecord(record infrastructure.AvailableOutputRecord, taskID int64) bool {
	output := record.Output
	if output.ID <= 0 || output.TaskID != taskID || output.Name == "" || output.Status != "AVAILABLE" || record.ObjectKey == "" || output.SizeBytes < 0 {
		return false
	}
	if output.OutputKind != "PATH" && output.OutputKind != "ARCHIVE" {
		return false
	}
	decoded, err := hex.DecodeString(output.SHA256)
	return err == nil && len(decoded) == sha256DigestBytes
}

const sha256DigestBytes = 32

func sanitizeOutputMediaType(value string) string {
	mediaType, _, err := mime.ParseMediaType(strings.TrimSpace(value))
	if err != nil || mediaType == "" || strings.ContainsAny(mediaType, "\r\n\x00") {
		return "application/octet-stream"
	}
	return strings.ToLower(mediaType)
}

func sanitizeOutputName(value string) string {
	value = strings.Map(func(r rune) rune {
		switch {
		case r < 0x20 || r == 0x7f:
			return -1
		case r == '/' || r == '\\':
			return '_'
		case r == '"':
			return -1
		default:
			return r
		}
	}, value)
	value = strings.TrimSpace(value)
	value = truncateUTF8(value, maxPublicOutputNameBytes)
	if value == "" || value == "." || value == ".." || strings.Trim(value, "._ ") == "" {
		return "output"
	}
	return value
}

func truncateUTF8(value string, limit int) string {
	if limit < 1 || len(value) <= limit {
		return value
	}
	value = value[:limit]
	for !utf8.ValidString(value) {
		value = value[:len(value)-1]
	}
	return value
}

var _ facade.ResultOutputAccess = (*Service)(nil)
