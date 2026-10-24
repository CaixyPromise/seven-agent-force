package control

import (
	"context"
	"errors"

	taskdomain "github.com/CaixyPromise/seven-agent/force/internal/app/workforce/task/domain"
	taskinfra "github.com/CaixyPromise/seven-agent/force/internal/app/workforce/task/infrastructure"
)

// EvidenceRecorder is the Worker-side Finalizer adapter. It keeps the
// Finalizer independent from PostgreSQL while preserving its existing
// deterministic artifact construction and MinIO byte verification.
type EvidenceRecorder struct {
	client  *Client
	attempt taskdomain.Attempt
}

func NewEvidenceRecorder(client *Client, attempt taskdomain.Attempt) *EvidenceRecorder {
	return &EvidenceRecorder{client: client, attempt: attempt}
}

func (r *EvidenceRecorder) validate(taskID, attemptID int64) error {
	if r == nil || r.client == nil || taskID != r.attempt.TaskID || attemptID != r.attempt.ID {
		return errors.New("finalizer evidence identity mismatch")
	}
	return nil
}

func (r *EvidenceRecorder) CheckEvidence(ctx context.Context, taskID, attemptID int64) ([]taskinfra.CheckEvidence, error) {
	if err := r.validate(taskID, attemptID); err != nil {
		return nil, err
	}
	checks, _, err := r.client.Evidence(ctx, r.attempt)
	return checks, err
}

func (r *EvidenceRecorder) Events(ctx context.Context, taskID int64, afterID int64, limit int) ([]taskdomain.Event, error) {
	if err := r.validate(taskID, r.attempt.ID); err != nil {
		return nil, err
	}
	_, events, err := r.client.Evidence(ctx, r.attempt)
	if err != nil {
		return nil, err
	}
	filtered := make([]taskdomain.Event, 0, len(events))
	for _, event := range events {
		if event.ID > afterID {
			filtered = append(filtered, event)
			if len(filtered) >= limit {
				break
			}
		}
	}
	return filtered, nil
}

func (r *EvidenceRecorder) RecordArtifact(ctx context.Context, artifact taskdomain.Artifact) error {
	if err := r.validate(artifact.TaskID, artifact.AttemptID); err != nil {
		return err
	}
	return r.client.RecordArtifact(ctx, r.attempt, artifact)
}
