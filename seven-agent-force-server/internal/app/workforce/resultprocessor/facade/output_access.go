package facade

import (
	"context"
	"errors"
	"io"
)

var (
	ErrOutputNotFound    = errors.New("result output not found")
	ErrOutputUnavailable = errors.New("result output unavailable")
)

// AvailableOutput is the bounded, cross-module view of one verified Result
// Processor output. Artifact persistence identities and object-store locators
// intentionally remain inside the Result Processor implementation.
type AvailableOutput struct {
	ID         int64
	TaskID     int64
	Name       string
	OutputKind string
	Status     string
	MediaType  string
	SHA256     string
	SizeBytes  int64
}

// OutputDownload owns a verified stream. The caller transfers Content to the
// HTTP response; the response transport closes it after the stream completes.
type OutputDownload struct {
	Output  AvailableOutput
	Content io.ReadCloser
}

// ResultOutputAccess hides Result Output/Artifact relation validation and
// object-store access behind the smallest interface needed by public adapters.
type ResultOutputAccess interface {
	ListAvailableOutputs(ctx context.Context, tenantID, taskID int64) ([]AvailableOutput, error)
	OpenAvailableOutput(ctx context.Context, tenantID, taskID, outputID int64) (OutputDownload, error)
}
