package domain

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"path"
	"regexp"
	"strings"
	"unicode"
	"unicode/utf8"
)

// Report limits deliberately keep untrusted Reviewer output small enough to
// validate, persist and present without becoming a second log transport.
const (
	MaxCandidateReportBytes = 256 << 10
	maxReportTextBytes      = 2 << 10
	maxCommandBytes         = 512
	maxReportItems          = 64
	maxFindingItems         = 32
)

var ErrInvalidReport = errors.New("REVIEW_REPORT_INVALID")

type CandidateReport struct {
	SchemaVersion string       `json:"schemaVersion"`
	Verdict       string       `json:"verdict"`
	Score         *int         `json:"score,omitempty"`
	Summary       string       `json:"summary"`
	Findings      []Finding    `json:"findings,omitempty"`
	ReviewTests   []ReviewTest `json:"reviewTests,omitempty"`
	Limitations   []Limitation `json:"limitations,omitempty"`
}

type Finding struct {
	ID                 string   `json:"id"`
	Severity           string   `json:"severity"`
	Category           string   `json:"category"`
	Title              string   `json:"title"`
	Message            string   `json:"message"`
	Evidence           Evidence `json:"evidence"`
	SuggestedDirection string   `json:"suggestedDirection,omitempty"`
}

type Evidence struct {
	Path string `json:"path"`
	Line int    `json:"line"`
}

type ReviewTest struct {
	Command  string `json:"command"`
	ExitCode int    `json:"exitCode"`
	Status   string `json:"status"`
	Summary  string `json:"summary"`
}

type Limitation struct {
	Code    string `json:"code"`
	Summary string `json:"summary"`
}

// UsageFacts is intentionally a closed allowlist. It is the only Provider
// observation that can cross the ReviewRun persistence boundary.
type UsageFacts struct {
	Requests      int64 `json:"requests,omitempty"`
	InputTokens   int64 `json:"inputTokens,omitempty"`
	OutputTokens  int64 `json:"outputTokens,omitempty"`
	DurationMilli int64 `json:"durationMillis,omitempty"`
}

// CanonicalReport is the only report representation stored by the Server.
// json.Marshal on this struct gives a stable field order, which makes its
// digest a meaningful replay/fencing fact rather than a digest of arbitrary
// Reviewer formatting.
type CanonicalReport = CandidateReport

func CanonicalizeReport(raw []byte) (CanonicalReport, string, error) {
	if len(raw) == 0 || len(raw) > MaxCandidateReportBytes {
		return CanonicalReport{}, "", ErrInvalidReport
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	var report CandidateReport
	if err := decoder.Decode(&report); err != nil || decoder.More() {
		return CanonicalReport{}, "", ErrInvalidReport
	}
	var trailing any
	if err := decoder.Decode(&trailing); err != io.EOF {
		return CanonicalReport{}, "", ErrInvalidReport
	}
	if err := validateAndSanitizeReport(&report); err != nil {
		return CanonicalReport{}, "", err
	}
	encoded, err := json.Marshal(report)
	if err != nil {
		return CanonicalReport{}, "", ErrInvalidReport
	}
	sum := sha256.Sum256(encoded)
	return report, hex.EncodeToString(sum[:]), nil
}

var (
	// Keep canonical reports useful without making them a transport for shell
	// output or credentials. The key pattern deliberately permits common
	// prefixes/suffixes (AWS_SECRET_ACCESS_KEY, access_token) and consumes the
	// optional Bearer scheme together with its value.
	secretAssignmentText = regexp.MustCompile(`(?i)\b(?:[a-z0-9]+[_-])*(?:api[_-]?key|token|password|secret(?:[_-][a-z0-9]+){0,3}|authorization)\b\s*[:=]\s*(?:bearer\s+)?[^\s,;]+`)
	// URLs may contain basic-auth credentials without an obvious key name.
	credentialURLText = regexp.MustCompile(`(?i)(https?://)[^/\s:@]+:[^/\s@]+@`)
	// Redact well-known high-risk bearer prefixes even if an untrusted reviewer
	// emits a token without naming the field that contains it.
	secretTokenText = regexp.MustCompile(`\b(?:sk|pk|ghp|github_pat|xox[baprs]|AKIA)[-_A-Za-z0-9]{16,}\b`)
	// A canonical report may explain a conclusion, but it is never a storage
	// tunnel for a raw provider request/response or an opaque transcript.
	rawProviderPayloadText = regexp.MustCompile(`(?i)(?:^|[\s{,])"?(?:model|messages|choices|response|output|usage|tool_calls|prompt|completion)"?\s*:`)
	longOpaqueText         = regexp.MustCompile(`[A-Za-z0-9+/=_-]{128,}`)
)

func validateAndSanitizeReport(report *CandidateReport) error {
	if report == nil || report.SchemaVersion != "review-report/v1" || !oneOf(report.Verdict, "PASS", "REVISE", "REJECT") || !validText(report.Summary) {
		return ErrInvalidReport
	}
	if report.Score != nil && (*report.Score < 0 || *report.Score > 100) {
		return ErrInvalidReport
	}
	if len(report.Findings) > maxFindingItems || len(report.ReviewTests) > maxReportItems || len(report.Limitations) > maxReportItems {
		return ErrInvalidReport
	}
	hasBlocking := false
	hasUnable := false
	for i := range report.Findings {
		item := &report.Findings[i]
		if !validIdentifier(item.ID) || !oneOf(item.Severity, "INFO", "WARNING", "BLOCKING") || !oneOf(item.Category, "REQUIREMENT", "CORRECTNESS", "SECURITY", "TESTING", "MAINTAINABILITY") || !validText(item.Title) || !validText(item.Message) || !validEvidence(item.Evidence) || !validOptionalText(item.SuggestedDirection) {
			return ErrInvalidReport
		}
		item.Title = sanitizeText(item.Title)
		item.Message = sanitizeText(item.Message)
		item.SuggestedDirection = sanitizeText(item.SuggestedDirection)
		hasBlocking = hasBlocking || item.Severity == "BLOCKING"
	}
	for i := range report.ReviewTests {
		item := &report.ReviewTests[i]
		if !validCommand(item.Command) || !oneOf(item.Status, "PASSED", "FAILED", "SKIPPED", "UNABLE_TO_REPRODUCE") || !validText(item.Summary) || item.ExitCode < -1 || item.ExitCode > 255 {
			return ErrInvalidReport
		}
		item.Command = sanitizeText(item.Command)
		item.Summary = sanitizeText(item.Summary)
		hasUnable = hasUnable || item.Status == "UNABLE_TO_REPRODUCE"
	}
	for i := range report.Limitations {
		item := &report.Limitations[i]
		if !oneOf(item.Code, "UNABLE_TO_REPRODUCE", "TIME_BOUNDED", "NO_PROVIDER_RESPONSE") || !validText(item.Summary) {
			return ErrInvalidReport
		}
		item.Summary = sanitizeText(item.Summary)
		hasUnable = hasUnable || item.Code == "UNABLE_TO_REPRODUCE"
	}
	if report.Verdict == "PASS" && (hasBlocking || hasUnable) {
		return ErrInvalidReport
	}
	report.Summary = sanitizeText(report.Summary)
	return nil
}

func validEvidence(value Evidence) bool {
	if value.Path == "" || len(value.Path) > 1024 || value.Line < 1 || strings.HasPrefix(value.Path, "/") || strings.Contains(value.Path, "\\") || path.Clean(value.Path) != value.Path || value.Path == "." || strings.HasPrefix(value.Path, "../") {
		return false
	}
	for _, character := range value.Path {
		if unicode.IsControl(character) {
			return false
		}
	}
	return true
}

func validText(value string) bool {
	if len(value) == 0 || len(value) > maxReportTextBytes || !utf8.ValidString(value) || containsSensitiveText(value) || rawProviderPayloadText.MatchString(value) || longOpaqueText.MatchString(value) {
		return false
	}
	for _, character := range value {
		if unicode.IsControl(character) {
			return false
		}
	}
	return true
}

func validCommand(value string) bool {
	if !validText(value) || len(value) > maxCommandBytes {
		return false
	}
	// Keep the retained command fact a compact, offline reproduction label,
	// not an arbitrary shell transcript or a network/credential invocation.
	if strings.ContainsAny(value, "|&;<>`$\\") {
		return false
	}
	for _, part := range strings.Fields(value) {
		if strings.EqualFold(part, "curl") || strings.EqualFold(part, "wget") || strings.HasPrefix(part, "http://") || strings.HasPrefix(part, "https://") || strings.HasPrefix(strings.ToLower(part), "authorization") {
			return false
		}
	}
	return true
}

func validOptionalText(value string) bool {
	return value == "" || validText(value)
}

func validIdentifier(value string) bool {
	return len(value) > 0 && len(value) <= 128 && validText(value)
}

func containsSensitiveText(value string) bool {
	return secretAssignmentText.MatchString(value) || credentialURLText.MatchString(value) || secretTokenText.MatchString(value)
}

func sanitizeText(value string) string {
	return strings.TrimSpace(value)
}

func oneOf(value string, allowed ...string) bool {
	for _, candidate := range allowed {
		if value == candidate {
			return true
		}
	}
	return false
}
