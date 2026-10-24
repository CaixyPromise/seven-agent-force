package domain

import (
	"encoding/json"
	"errors"
	"strconv"
	"strings"
	"testing"
)

func TestCanonicalizeReportAcceptsSafePassAndHasStableDigest(t *testing.T) {
	raw := []byte(`{"schemaVersion":"review-report/v1","verdict":"PASS","score":99,"summary":"looks good","reviewTests":[{"command":"git status --short","exitCode":0,"status":"PASSED","summary":"passed"}]}`)
	report, digest, err := CanonicalizeReport(raw)
	if err != nil {
		t.Fatalf("canonicalize safe report: %v", err)
	}
	if len(digest) != 64 || report.Verdict != "PASS" || report.ReviewTests[0].Command != "git status --short" {
		t.Fatalf("unsafe canonical report: %#v digest=%q", report, digest)
	}
	again, againDigest, err := CanonicalizeReport(raw)
	if err != nil || againDigest != digest {
		t.Fatalf("canonical digest is not stable: report=%#v digest=%q err=%v", again, againDigest, err)
	}
}

func TestCanonicalizeReportRejectsSensitiveCandidateText(t *testing.T) {
	for _, text := range []string{
		"TOKEN=supersecret",
		"AWS_SECRET_ACCESS_KEY=another-secret",
		"Authorization: Bearer bearer-secret",
		"curl https://user:password@example.invalid",
		"sk-abcdefghijklmnopqrstuvwxyz",
	} {
		raw := []byte(`{"schemaVersion":"review-report/v1","verdict":"REVISE","summary":` + strconv.Quote(text) + `}`)
		if _, _, err := CanonicalizeReport(raw); !errors.Is(err, ErrInvalidReport) {
			t.Fatalf("accepted sensitive candidate %q: %v", text, err)
		}
	}
}

func TestCanonicalizeReportRejectsRawProviderPayloadAndUnsafeCommand(t *testing.T) {
	for _, raw := range [][]byte{
		[]byte(`{"schemaVersion":"review-report/v1","verdict":"REVISE","summary":"{\"model\":\"untrusted\",\"output\":\"provider body\"}"}`),
		[]byte(`{"schemaVersion":"review-report/v1","verdict":"REVISE","summary":"safe","reviewTests":[{"command":"curl https://provider.invalid","exitCode":0,"status":"PASSED","summary":"unsafe command"}]}`),
		[]byte(`{"schemaVersion":"review-report/v1","verdict":"REVISE","summary":"` + strings.Repeat("A", 128) + `"}`),
	} {
		if _, _, err := CanonicalizeReport(raw); !errors.Is(err, ErrInvalidReport) {
			t.Fatalf("accepted raw provider payload or unsafe command: %s", raw)
		}
	}
}

func TestCanonicalizeReportRejectsOversizedCandidateBeforeDecoding(t *testing.T) {
	raw := []byte(`{"schemaVersion":"review-report/v1","verdict":"REVISE","summary":"` + strings.Repeat("x", MaxCandidateReportBytes) + `"}`)
	if len(raw) <= MaxCandidateReportBytes {
		t.Fatalf("oversized fixture is only %d bytes", len(raw))
	}
	if _, _, err := CanonicalizeReport(raw); !errors.Is(err, ErrInvalidReport) {
		t.Fatalf("oversized candidate accepted: %v", err)
	}
}

func TestCanonicalizeReportRejectsContradictionAndPathEscape(t *testing.T) {
	cases := []map[string]any{
		{"schemaVersion": "review-report/v1", "verdict": "PASS", "summary": "bad", "findings": []any{map[string]any{"id": "f1", "severity": "BLOCKING", "category": "CORRECTNESS", "title": "bad", "message": "bad", "evidence": map[string]any{"path": "main.go", "line": 1}}}},
		{"schemaVersion": "review-report/v1", "verdict": "REVISE", "summary": "bad", "findings": []any{map[string]any{"id": "f1", "severity": "WARNING", "category": "CORRECTNESS", "title": "bad", "message": "bad", "evidence": map[string]any{"path": "../secret", "line": 1}}}},
		{"schemaVersion": "review-report/v1", "verdict": "PASS", "summary": "bad", "limitations": []any{map[string]any{"code": "UNABLE_TO_REPRODUCE", "summary": "offline dependency missing"}}},
	}
	for _, item := range cases {
		raw, _ := json.Marshal(item)
		if _, _, err := CanonicalizeReport(raw); err == nil {
			t.Fatalf("accepted invalid report: %s", raw)
		}
	}
}
