package facade

import "context"

// IndependentReviewInput is the immutable, execution-relevant Task text that
// a Reviewer may receive.  It deliberately excludes mutable workspace paths,
// credentials, provider configuration and delivery state.
type IndependentReviewInput struct {
	TenantID              int64
	TaskID                int64
	TaskTurnID            int64
	Instruction           string
	CompletionCheckDigest string
	Instructions          string
	ReviewPrompt          string
	References            []IndependentReviewReference
}

type IndependentReviewReference struct {
	ReferenceAssetID int64  `json:"referenceAssetId,string"`
	Name             string `json:"name"`
	ContentSHA256    string `json:"contentSha256"`
	SizeBytes        int64  `json:"sizeBytes,string"`
}

// IndependentReviewInputReader is the only Task-to-Reviewer read contract.
// ReviewRun orchestration must not reach into Task's persistence store.
type IndependentReviewInputReader interface {
	IndependentReviewInput(context.Context, int64, int64, int64) (IndependentReviewInput, error)
}
