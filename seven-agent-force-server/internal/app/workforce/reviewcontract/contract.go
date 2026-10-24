// Package reviewcontract contains the platform-owned independent review
// protocol. Project owners provide only review_prompt; these execution rules
// are not a configurable product surface.
package reviewcontract

import "encoding/json"

const (
	SystemContract             = "independent-reviewer/system/v1"
	ReportSchemaVersion        = "review-report/v1"
	StructuredOutputContractV1 = "independent-reviewer/structured-output/v1"
	HardTimeoutSeconds         = 60 * 60
	TransientRetryLimit        = 3
	AutomaticRevisionLimit     = 3
)

// RuntimeContract is the fixed document passed to the Reviewer adapter.
// It describes evidence and output expectations without carrying project
// policy; the project-specific review prompt lives in review requirements.
func RuntimeContract() json.RawMessage {
	return json.RawMessage(`{"version":"independent-review/v1","systemContract":"independent-reviewer/system/v1","reportSchemaVersion":"review-report/v1"}`)
}
