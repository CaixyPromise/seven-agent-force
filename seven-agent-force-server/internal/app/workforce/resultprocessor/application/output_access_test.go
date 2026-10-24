package application

import (
	"context"
	"errors"
	"io"
	"strings"
	"testing"

	"github.com/CaixyPromise/seven-agent/force/internal/app/workforce/resultprocessor/facade"
	"github.com/CaixyPromise/seven-agent/force/internal/app/workforce/resultprocessor/infrastructure"
)

type outputRepositoryStub struct {
	items   []infrastructure.AvailableOutputRecord
	item    infrastructure.AvailableOutputRecord
	listErr error
	getErr  error
}

func (s outputRepositoryStub) ListAvailableOutputs(context.Context, int64, int64) ([]infrastructure.AvailableOutputRecord, error) {
	return s.items, s.listErr
}

func (s outputRepositoryStub) AvailableOutput(context.Context, int64, int64, int64) (infrastructure.AvailableOutputRecord, error) {
	return s.item, s.getErr
}

type outputObjectReaderStub struct {
	content string
	size    int64
	err     error
	key     string
	closed  bool
}

func (s *outputObjectReaderStub) Open(_ context.Context, key string) (io.ReadCloser, int64, error) {
	s.key = key
	if s.err != nil {
		return nil, 0, s.err
	}
	size := s.size
	if size == 0 {
		size = int64(len(s.content))
	}
	return &trackingReadCloser{Reader: strings.NewReader(s.content), closed: &s.closed}, size, nil
}

type trackingReadCloser struct {
	io.Reader
	closed *bool
}

func (r *trackingReadCloser) Close() error {
	*r.closed = true
	return nil
}

func TestListAvailableOutputsSanitizesPublicMetadata(t *testing.T) {
	repository := outputRepositoryStub{items: []infrastructure.AvailableOutputRecord{{
		Output: facade.AvailableOutput{
			ID: 8_500_000_000_000_000_001, TaskID: 8_500_000_000_000_000_002,
			Name: "../danger\r\n\x00\"report.md", OutputKind: "PATH", Status: "AVAILABLE",
			MediaType: "not a media type", SHA256: strings.Repeat("a", 64), SizeBytes: 7,
		},
		ObjectKey: "private/object/key",
	}}}
	service := &Service{outputStore: repository}

	items, err := service.ListAvailableOutputs(context.Background(), 101, 8_500_000_000_000_000_002)
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 1 {
		t.Fatalf("items = %d, want 1", len(items))
	}
	item := items[0]
	if item.Name != ".._dangerreport.md" {
		t.Fatalf("sanitized name = %q", item.Name)
	}
	if item.MediaType != "application/octet-stream" {
		t.Fatalf("media type = %q", item.MediaType)
	}
}

func TestOpenAvailableOutputReturnsVerifiedStream(t *testing.T) {
	record := infrastructure.AvailableOutputRecord{
		Output: facade.AvailableOutput{
			ID: 8_500_000_000_000_000_011, TaskID: 8_500_000_000_000_000_012,
			Name: "result", OutputKind: "PATH", Status: "AVAILABLE",
			MediaType: "text/markdown; charset=utf-8", SHA256: strings.Repeat("b", 64), SizeBytes: 6,
		},
		ObjectKey: "tasks/private/result",
	}
	objects := &outputObjectReaderStub{content: "result"}
	service := &Service{outputStore: outputRepositoryStub{item: record}, outputObjects: objects}

	download, err := service.OpenAvailableOutput(context.Background(), 101, record.Output.TaskID, record.Output.ID)
	if err != nil {
		t.Fatal(err)
	}
	defer download.Content.Close()
	content, err := io.ReadAll(download.Content)
	if err != nil {
		t.Fatal(err)
	}
	if string(content) != "result" || objects.key != record.ObjectKey {
		t.Fatalf("content=%q key=%q", content, objects.key)
	}
	if download.Output.MediaType != "text/markdown" {
		t.Fatalf("media type = %q", download.Output.MediaType)
	}
}

func TestOpenAvailableOutputMapsNotFoundWithoutOpeningObject(t *testing.T) {
	objects := &outputObjectReaderStub{}
	service := &Service{
		outputStore:   outputRepositoryStub{getErr: infrastructure.ErrNotFound},
		outputObjects: objects,
	}

	_, err := service.OpenAvailableOutput(context.Background(), 101, 201, 301)
	if !errors.Is(err, facade.ErrOutputNotFound) {
		t.Fatalf("error = %v, want ErrOutputNotFound", err)
	}
	if objects.key != "" {
		t.Fatalf("object store opened %q for missing output", objects.key)
	}
}

func TestOpenAvailableOutputFailsClosedOnRepositoryIdentityDrift(t *testing.T) {
	record := infrastructure.AvailableOutputRecord{
		Output: facade.AvailableOutput{
			ID: 301, TaskID: 999, Name: "result", OutputKind: "PATH", Status: "AVAILABLE",
			MediaType: "text/plain", SHA256: strings.Repeat("c", 64), SizeBytes: 6,
		},
		ObjectKey: "secret/object/key",
	}
	objects := &outputObjectReaderStub{}
	service := &Service{outputStore: outputRepositoryStub{item: record}, outputObjects: objects}

	_, err := service.OpenAvailableOutput(context.Background(), 101, 201, record.Output.ID)
	if !errors.Is(err, facade.ErrOutputNotFound) {
		t.Fatalf("error = %v, want ErrOutputNotFound", err)
	}
	if objects.key != "" {
		t.Fatalf("object store opened %q for drifted output", objects.key)
	}
}

func TestOpenAvailableOutputHidesObjectStoreFailure(t *testing.T) {
	record := infrastructure.AvailableOutputRecord{
		Output: facade.AvailableOutput{
			ID: 301, TaskID: 201, Name: "result", OutputKind: "PATH", Status: "AVAILABLE",
			MediaType: "text/plain", SHA256: strings.Repeat("c", 64), SizeBytes: 6,
		},
		ObjectKey: "secret/object/key",
	}
	objects := &outputObjectReaderStub{err: errors.New("backend included secret/object/key")}
	service := &Service{outputStore: outputRepositoryStub{item: record}, outputObjects: objects}

	_, err := service.OpenAvailableOutput(context.Background(), 101, record.Output.TaskID, record.Output.ID)
	if !errors.Is(err, facade.ErrOutputUnavailable) {
		t.Fatalf("error = %v, want ErrOutputUnavailable", err)
	}
	if strings.Contains(err.Error(), record.ObjectKey) || strings.Contains(err.Error(), "backend") {
		t.Fatalf("error leaks object-store detail: %v", err)
	}
}

func TestOpenAvailableOutputRejectsCurrentObjectSizeDriftBeforeStreaming(t *testing.T) {
	record := infrastructure.AvailableOutputRecord{
		Output: facade.AvailableOutput{
			ID: 301, TaskID: 201, Name: "result", OutputKind: "PATH", Status: "AVAILABLE",
			MediaType: "text/plain", SHA256: strings.Repeat("c", 64), SizeBytes: 6,
		},
		ObjectKey: "secret/object/key",
	}
	objects := &outputObjectReaderStub{content: "short", size: 5}
	service := &Service{outputStore: outputRepositoryStub{item: record}, outputObjects: objects}

	_, err := service.OpenAvailableOutput(context.Background(), 101, record.Output.TaskID, record.Output.ID)
	if !errors.Is(err, facade.ErrOutputUnavailable) {
		t.Fatalf("error = %v, want ErrOutputUnavailable", err)
	}
	if !objects.closed {
		t.Fatal("size-drifted object stream was not closed")
	}
}

func TestSanitizeOutputNameFallsBackAndBoundsUTF8(t *testing.T) {
	if got := sanitizeOutputName("\r\n/\\\x00\""); got != "output" {
		t.Fatalf("fallback name = %q", got)
	}
	got := sanitizeOutputName(strings.Repeat("界", 100))
	if len(got) > maxPublicOutputNameBytes || !strings.HasPrefix(got, "界") {
		t.Fatalf("bounded UTF-8 name length=%d value=%q", len(got), got)
	}
}
