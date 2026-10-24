package facade

// EvaluationReplayIdentity identifies the exact pending Supervisor cycle.
// Candidate and persisted contract digests may differ after Server checks.
type EvaluationReplayIdentity struct {
	CycleID       int64  `json:"cycleId,string"`
	CycleNumber   int    `json:"cycleNumber"`
	SessionDigest string `json:"sessionDigest"`
}

// Found=false is returned only after current execution authority is checked.
type EvaluationReplay struct {
	Found     bool                `json:"found"`
	Directive EvaluationDirective `json:"directive"`
}
