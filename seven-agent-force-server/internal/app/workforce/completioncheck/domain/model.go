// Package domain owns task-scoped check inputs and conservative comparison.
// A process exit or an Agent claim alone never proves historical independence.
package domain

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"path"
	"sort"
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"
)

const (
	MaxTextBytes    = 16 << 10
	MaxScriptBytes  = 64 << 10
	MaxCommandBytes = 8 << 10
)

var ErrInvalid = errors.New("COMPLETION_CHECK_INVALID")

type Input struct {
	Mode              string  `json:"mode"`
	Command           string  `json:"command,omitempty"`
	Script            string  `json:"script,omitempty"`
	Instructions      string  `json:"instructions,omitempty"`
	ReferenceAssetIDs []int64 `json:"-"`
}

type wireInput struct {
	Mode              string   `json:"mode"`
	Command           string   `json:"command,omitempty"`
	Script            string   `json:"script,omitempty"`
	Instructions      string   `json:"instructions,omitempty"`
	ReferenceAssetIDs []string `json:"referenceAssetIds,omitempty"`
}

func (i Input) MarshalJSON() ([]byte, error) {
	ids := make([]string, len(i.ReferenceAssetIDs))
	for n, id := range i.ReferenceAssetIDs {
		if id <= 0 {
			return nil, ErrInvalid
		}
		ids[n] = strconv.FormatInt(id, 10)
	}
	return json.Marshal(wireInput{i.Mode, i.Command, i.Script, i.Instructions, ids})
}

func (i *Input) UnmarshalJSON(b []byte) error {
	if !utf8.Valid(b) || !uniqueObjectFields(b) {
		return ErrInvalid
	}
	var w wireInput
	d := json.NewDecoder(bytes.NewReader(b))
	d.DisallowUnknownFields()
	if err := d.Decode(&w); err != nil {
		return ErrInvalid
	}
	if err := d.Decode(new(any)); err != io.EOF {
		return ErrInvalid
	}
	ids := make([]int64, len(w.ReferenceAssetIDs))
	for n, value := range w.ReferenceAssetIDs {
		id, err := strconv.ParseInt(value, 10, 64)
		if err != nil || id <= 0 || strconv.FormatInt(id, 10) != value {
			return ErrInvalid
		}
		ids[n] = id
	}
	normalized, err := Normalize(Input{w.Mode, w.Command, w.Script, w.Instructions, ids})
	if err != nil {
		return err
	}
	*i = normalized
	return nil
}

// Reject duplicate top-level fields before decoding so different parsers
// cannot disagree about the executable or the input identity.
func uniqueObjectFields(b []byte) bool {
	d := json.NewDecoder(bytes.NewReader(b))
	token, err := d.Token()
	if err != nil || token != json.Delim('{') {
		return false
	}
	seen := map[string]bool{}
	for d.More() {
		token, err = d.Token()
		key, ok := token.(string)
		if err != nil || !ok || seen[key] {
			return false
		}
		seen[key] = true
		var value json.RawMessage
		if d.Decode(&value) != nil {
			return false
		}
	}
	if _, err = d.Token(); err != nil {
		return false
	}
	return d.Decode(new(any)) == io.EOF
}

func Normalize(i Input) (Input, error) {
	i.Mode = strings.ToUpper(strings.TrimSpace(i.Mode))
	if i.Mode == "" {
		i.Mode = "NONE"
	}
	i.Command = normalizeText(i.Command)
	i.Script = normalizeText(i.Script)
	i.Instructions = normalizeText(i.Instructions)
	if !validText(i.Command, MaxCommandBytes) || !validText(i.Script, MaxScriptBytes) || !validText(i.Instructions, MaxTextBytes) || len(i.ReferenceAssetIDs) > 64 {
		return Input{}, ErrInvalid
	}
	switch i.Mode {
	case "NONE":
		if i.Command != "" || i.Script != "" {
			return Input{}, ErrInvalid
		}
	case "COMMAND":
		if i.Command == "" || i.Script != "" {
			return Input{}, ErrInvalid
		}
	case "SCRIPT":
		if i.Script == "" || i.Command != "" {
			return Input{}, ErrInvalid
		}
	default:
		return Input{}, ErrInvalid
	}
	ids := append([]int64(nil), i.ReferenceAssetIDs...)
	sort.Slice(ids, func(a, b int) bool { return ids[a] < ids[b] })
	for n, id := range ids {
		if id <= 0 || (n > 0 && id == ids[n-1]) {
			return Input{}, ErrInvalid
		}
	}
	i.ReferenceAssetIDs = ids
	return i, nil
}

func Digest(i Input) (string, error) {
	n, err := Normalize(i)
	if err != nil {
		return "", err
	}
	b, err := json.Marshal(n)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:]), nil
}
func normalizeText(s string) string { return strings.TrimSpace(strings.ReplaceAll(s, "\r\n", "\n")) }
func validText(s string, max int) bool {
	if !utf8.ValidString(s) || len(s) > max {
		return false
	}
	for _, r := range s {
		if unicode.IsControl(r) && r != '\n' && r != '\t' {
			return false
		}
	}
	return true
}

type Status string

const (
	Passed        Status = "PASSED"
	Failed        Status = "FAILED"
	Unavailable   Status = "UNAVAILABLE"
	NoTests       Status = "NO_TESTS"
	NotConfigured Status = "NOT_CONFIGURED"
	TimedOut      Status = "TIMED_OUT"
)

type Failure struct {
	Key         string `json:"key"`
	Path        string `json:"path"`
	Fingerprint string `json:"fingerprint"`
}
type Observation struct {
	DefinitionDigest    string `json:"definitionDigest,omitempty"`
	DiagnosticDigest    string `json:"diagnosticDigest,omitempty"`
	DiagnosticKnown     bool   `json:"diagnosticKnown,omitempty"`
	DiagnosticTruncated bool   `json:"diagnosticTruncated,omitempty"`
	ReasonCode          string `json:"reasonCode,omitempty"`
	ExitCode            *int   `json:"exitCode,omitempty"`
	WorkspaceDigest     string `json:"workspaceDigest,omitempty"`
	// PASSED means the configured check command succeeded. It only asserts
	// tests ran when TestCountKnown is true and TestCount is positive.
	TestCountKnown bool      `json:"testCountKnown,omitempty"`
	TestCount      int       `json:"testCount,omitempty"`
	CheckDigest    string    `json:"checkDigest"`
	Status         Status    `json:"status"`
	Failures       []Failure `json:"failures,omitempty"`
}

// ComparisonContext must be derived by the trusted application, never decoded
// from an Agent request. Independence requires dependency evidence, not just a
// different filename or equal error count.
type ComparisonContext struct {
	Mandatory                      bool
	DefinitionChanged              bool
	ChangedPaths                   []string
	IndependentTargetChecksPassed  bool
	HistoricalIndependenceVerified bool
}
type Disposition string

const (
	Proceed                Disposition = "PROCEED"
	Repair                 Disposition = "REPAIR"
	AskUser                Disposition = "ASK_USER"
	Blocked                Disposition = "BLOCKED"
	NoCheck                Disposition = "NO_CHECK"
	ProceedWithLimitations Disposition = "PROCEED_WITH_LIMITATIONS"
)

type Comparison struct {
	Disposition Disposition `json:"disposition"`
	ReasonCode  string      `json:"reasonCode"`
}

func outcome(d Disposition, code string) Comparison { return Comparison{d, code} }

func Compare(before, after Observation, c ComparisonContext) Comparison {
	for _, observation := range []Observation{before, after} {
		if observation.TestCount < 0 || (!observation.TestCountKnown && observation.TestCount != 0) ||
			(observation.Status == Passed && (len(observation.Failures) != 0 || observation.TestCountKnown && observation.TestCount == 0)) {
			return outcome(AskUser, "CHECK_EVIDENCE_INVALID")
		}
	}
	if c.DefinitionChanged {
		return outcome(AskUser, "CHECK_DEFINITION_CHANGED")
	}
	if before.Status == NotConfigured && after.Status == NotConfigured && !c.Mandatory {
		return outcome(NoCheck, "CHECK_NOT_CONFIGURED")
	}
	if !validDigest(before.CheckDigest) || before.CheckDigest != after.CheckDigest {
		return outcome(AskUser, "CHECK_IDENTITY_CHANGED")
	}
	if !validStatus(before.Status) || !validStatus(after.Status) {
		return outcome(AskUser, "CHECK_EVIDENCE_INVALID")
	}
	if after.Status == Passed {
		return outcome(Proceed, "CHECK_PASSED")
	}
	if c.Mandatory {
		return outcome(Blocked, "MANDATORY_CHECK_FAILED")
	}
	if after.Status != Failed {
		return outcome(AskUser, "CHECK_UNAVAILABLE")
	}
	if before.Status == Passed {
		return outcome(Repair, "CHECK_NEW_FAILURE")
	}
	if before.Status == Failed && c.IndependentTargetChecksPassed && c.HistoricalIndependenceVerified && sameFailures(before.Failures, after.Failures, c.ChangedPaths) {
		return outcome(ProceedWithLimitations, "CHECK_HISTORICAL_FAILURE")
	}
	return outcome(AskUser, "CHECK_ATTRIBUTION_UNCERTAIN")
}
func validStatus(s Status) bool {
	switch s {
	case Passed, Failed, Unavailable, NoTests, NotConfigured, TimedOut:
		return true
	}
	return false
}
func validDigest(s string) bool {
	if len(s) != 64 {
		return false
	}
	for _, c := range s {
		if !(c >= '0' && c <= '9' || c >= 'a' && c <= 'f') {
			return false
		}
	}
	return true
}
func sameFailures(a, b []Failure, changed []string) bool {
	if len(a) == 0 || len(a) != len(b) || len(a) > 256 {
		return false
	}
	signatures := make(map[Failure]bool, len(a))
	paths := make(map[string]bool, len(changed))
	for _, p := range changed {
		if !safePath(p) {
			return false
		}
		paths[p] = true
	}
	for _, f := range a {
		if !safePath(f.Path) || !validDigest(f.Fingerprint) || !validText(f.Key, 256) || f.Key == "" || signatures[f] || paths[f.Path] {
			return false
		}
		signatures[f] = true
	}
	seen := make(map[Failure]bool, len(b))
	for _, f := range b {
		if !signatures[f] || seen[f] {
			return false
		}
		seen[f] = true
	}
	return true
}
func safePath(p string) bool {
	if !utf8.ValidString(p) || len(p) > 4096 {
		return false
	}
	for _, r := range p {
		if unicode.IsControl(r) {
			return false
		}
	}
	return p != "" && p != "." && !strings.ContainsAny(p, "\\\x00") && !strings.HasPrefix(p, "/") && p != ".." && !strings.HasPrefix(p, "../") && path.Clean(p) == p
}
