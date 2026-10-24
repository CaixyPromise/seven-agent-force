// Package facade is the only cross-module Result Processor contract. It keeps
// Task creation independent from the management Store implementation.
package facade

import (
	"context"
	"encoding/json"
)

type Execution struct {
	ReleaseID               int64           `json:"releaseId,string"`
	Digest                  string          `json:"digest"`
	ArchiveObjectKey        string          `json:"archiveObjectKey"`
	Manifest                json.RawMessage `json:"manifest"`
	EnvironmentConfigDigest string          `json:"environmentConfigDigest"`
	// Environment is the canonical, non-secret execution configuration frozen
	// into TaskTurn #1. SECRET values are never present here; only their exact
	// ref/version identities are included in SecretVersionIdentities.
	Environment             json.RawMessage `json:"environment"`
	SecretVersionIdentities json.RawMessage `json:"secretVersionIdentities"`
	Parameters              json.RawMessage `json:"parameters"`
	MaxEvaluationCycles     int             `json:"maxEvaluationCycles"`
	ContinuationPolicy      string          `json:"continuationPolicy"`
	DeliveryPolicy          string          `json:"deliveryPolicy"`
}

type Resolver interface {
	ResolveExecution(ctx context.Context, tenantID int64, mode string, releaseID int64, parameters, configOverrides json.RawMessage) (Execution, error)
}
