package domain

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestInputNormalizationAndDigest(t *testing.T) {
	a, err := Normalize(Input{Mode: "COMMAND", Command: "  pnpm test \r\n"})
	if err != nil || a.Command != "pnpm test" {
		t.Fatalf("normalize: %v", err)
	}
	first, _ := Digest(a)
	second, _ := Digest(Input{Mode: "COMMAND", Command: "pnpm test"})
	if first != second || len(first) != 64 {
		t.Fatal("digest not canonical")
	}
	changed, _ := Digest(Input{Mode: "COMMAND", Command: "pnpm test", Instructions: "also verify pagination boundaries"})
	if changed == first {
		t.Fatal("check instructions missing from identity")
	}
}

func TestInputRejectsContradictoryOrInvalidData(t *testing.T) {
	for _, input := range []Input{
		{Mode: "COMMAND"}, {Mode: "SCRIPT", Script: "echo ok", Command: "true"},
		{Mode: "NONE", Command: "true"}, {Mode: "shell"},
		{Mode: "SCRIPT", Script: "echo\x00bad"}, {Instructions: strings.Repeat("a", MaxTextBytes+1)},
		{ReferenceAssetIDs: []int64{0}}, {ReferenceAssetIDs: []int64{1, 1}},
	} {
		if _, err := Normalize(input); err == nil {
			t.Fatalf("accepted invalid input: mode=%s", input.Mode)
		}
	}
}

func TestReferenceIDsStrictDecimalStrings(t *testing.T) {
	for _, wire := range []string{`{"referenceAssetIds":[123]}`, `{"referenceAssetIds":["01"]}`, `{"referenceAssetIds":["+1"]}`, `{"referenceAssetIds":["9223372036854775808"]}`, `{"unknown":true}`} {
		var input Input
		if json.Unmarshal([]byte(wire), &input) == nil {
			t.Fatal("accepted invalid wire")
		}
	}
	var input Input
	if err := json.Unmarshal([]byte(`{"referenceAssetIds":["8000000000000000002"]}`), &input); err != nil {
		t.Fatal(err)
	}
	b, _ := json.Marshal(input)
	if !strings.Contains(string(b), `"8000000000000000002"`) {
		t.Fatal("lost decimal string identity")
	}
}

func TestInputRejectsParserAmbiguity(t *testing.T) {
	for _, wire := range [][]byte{
		[]byte(`{"mode":"NONE","mode":"COMMAND","command":"true"}`),
		[]byte(`{"mode":"NONE"} {"mode":"NONE"}`),
		[]byte("{\"instructions\":\"\xff\"}"),
		[]byte(`null`), []byte(`[]`),
	} {
		var input Input
		if json.Unmarshal(wire, &input) == nil {
			t.Fatal("ambiguous input accepted")
		}
	}
}

func TestBaselineComparisonIsFailClosed(t *testing.T) {
	check := strings.Repeat("a", 64)
	passed := Observation{CheckDigest: check, Status: Passed}
	failed := Observation{CheckDigest: check, Status: Failed, Failures: []Failure{{Key: "TS2322", Path: "src/old.ts", Fingerprint: strings.Repeat("b", 64)}}}
	cases := []struct {
		name          string
		before, after Observation
		context       ComparisonContext
		want          Disposition
	}{
		{"new regression", passed, failed, ComparisonContext{}, Repair},
		{"fixed", failed, passed, ComparisonContext{}, Proceed},
		{"historical cannot infer independence", failed, failed, ComparisonContext{}, AskUser},
		{"count equality not proof", failed, Observation{CheckDigest: check, Status: Failed}, ComparisonContext{}, AskUser},
		{"missing command", Observation{CheckDigest: check, Status: Unavailable}, Observation{CheckDigest: check, Status: Unavailable}, ComparisonContext{}, AskUser},
		{"not configured", Observation{Status: NotConfigured}, Observation{Status: NotConfigured}, ComparisonContext{}, NoCheck},
		{"mandatory failed", failed, failed, ComparisonContext{Mandatory: true}, Blocked},
		{"definition drift", passed, Observation{CheckDigest: strings.Repeat("c", 64), Status: Passed}, ComparisonContext{}, AskUser},
		{"tampered check", passed, passed, ComparisonContext{DefinitionChanged: true}, AskUser},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := Compare(c.before, c.after, c.context); got.Disposition != c.want {
				t.Fatalf("got %s want %s", got.Disposition, c.want)
			}
		})
	}
}

func TestKnownHistoricalFailureNeedsIndependentTargetEvidence(t *testing.T) {
	o := Observation{CheckDigest: strings.Repeat("a", 64), Status: Failed, Failures: []Failure{{Key: "T", Path: "old.go", Fingerprint: strings.Repeat("b", 64)}}}
	c := ComparisonContext{IndependentTargetChecksPassed: true, HistoricalIndependenceVerified: true, ChangedPaths: []string{"new.go"}}
	if Compare(o, o, c).Disposition != ProceedWithLimitations {
		t.Fatal("historical evidence should permit nonmandatory continuation")
	}
	c.ChangedPaths = []string{"old.go"}
	if Compare(o, o, c).Disposition != AskUser {
		t.Fatal("changed failing path cannot be waived")
	}
	c.ChangedPaths = nil
	c.Mandatory = true
	if Compare(o, o, c).Disposition != Blocked {
		t.Fatal("mandatory cannot be waived")
	}
}

func TestHistoricalComparisonCannotUseUntrustedOrDuplicateFailures(t *testing.T) {
	digest := strings.Repeat("a", 64)
	good := Failure{Key: "TS2322", Path: "src/old.ts", Fingerprint: strings.Repeat("b", 64)}
	c := ComparisonContext{HistoricalIndependenceVerified: true, IndependentTargetChecksPassed: true}
	for _, failures := range [][]Failure{
		nil, {good, good}, {{Key: "T", Path: "../outside", Fingerprint: good.Fingerprint}},
		{{Key: "T", Path: "/tmp/old", Fingerprint: good.Fingerprint}},
		{{Key: "T", Path: "src/old\n.ts", Fingerprint: good.Fingerprint}},
		{{Key: "T", Path: "src/old.ts", Fingerprint: ""}},
	} {
		o := Observation{CheckDigest: digest, Status: Failed, Failures: failures}
		if Compare(o, o, c).Disposition != AskUser {
			t.Fatal("unusable evidence permitted continuation")
		}
	}
	before := Observation{CheckDigest: digest, Status: Failed, Failures: []Failure{good}}
	c.ChangedPaths = []string{"./src/old.ts"}
	if Compare(before, before, c).Disposition != AskUser {
		t.Fatal("noncanonical changed path bypassed failure intersection")
	}
	c.ChangedPaths = nil
	changed := good
	changed.Fingerprint = strings.Repeat("c", 64)
	after := Observation{CheckDigest: digest, Status: Failed, Failures: []Failure{changed}}
	if Compare(before, after, c).Disposition != AskUser {
		t.Fatal("equal count hid a different failure")
	}
	for _, status := range []Status{NoTests, TimedOut, Unavailable, Status("UNKNOWN")} {
		after.Status = status
		if Compare(before, after, c).Disposition != AskUser {
			t.Fatal("missing execution became success")
		}
	}
}

func TestReferenceOrderDoesNotChangeSemanticDigest(t *testing.T) {
	a, _ := Digest(Input{ReferenceAssetIDs: []int64{3, 1}})
	b, _ := Digest(Input{ReferenceAssetIDs: []int64{1, 3}})
	if a != b {
		t.Fatal("unordered reference set changed digest")
	}
	ids := []int64{3, 1}
	_, _ = Normalize(Input{ReferenceAssetIDs: ids})
	if ids[0] != 3 {
		t.Fatal("normalization mutated caller input")
	}
}

func TestContradictoryCheckEvidenceNeverPasses(t *testing.T) {
	before := Observation{Status: Passed, CheckDigest: strings.Repeat("a", 64)}
	for _, after := range []Observation{
		{Status: Passed, CheckDigest: before.CheckDigest, Failures: []Failure{{Key: "ERROR"}}},
		{Status: Passed, CheckDigest: before.CheckDigest, TestCountKnown: true},
		{Status: Passed, CheckDigest: before.CheckDigest, TestCount: 2},
	} {
		if Compare(before, after, ComparisonContext{}).Disposition != AskUser {
			t.Fatal("contradictory evidence became success")
		}
	}
}
