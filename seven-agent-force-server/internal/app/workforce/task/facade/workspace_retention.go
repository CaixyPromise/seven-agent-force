package facade

import (
	"context"
	"time"
)

// WorkspaceGCEligibility is the Server-owned retention decision for one
// Worker-local Task directory. It deliberately carries only durable state
// facts and safe reason codes: the Worker must never infer terminal state or
// active work from a directory name, a manifest, or a local process.
type WorkspaceGCEligibility struct {
	TaskID                 int64     `json:"taskId,string"`
	Terminal               bool      `json:"terminal"`
	TerminalAt             time.Time `json:"terminalAt,omitempty"`
	ActiveReferences       bool      `json:"activeReferences"`
	PortableStateAvailable bool      `json:"portableStateAvailable"`
	ReasonCode             string    `json:"reasonCode"`
}

// WorkspaceRetentionAuthority is the narrow cross-module contract used by
// the Worker Machine API. A future ReviewRun implementation must extend the
// Server-side checks before it can retain any local Reviewer workspace.
type WorkspaceRetentionAuthority interface {
	WorkspaceGCEligibility(context.Context, int64) (WorkspaceGCEligibility, error)
}
