package domain

import (
	"testing"
	"time"
)

func TestAdditiveRevisionPreservesExistingContract(t *testing.T) {
	base := Input{Mode: "COMMAND", Command: "go test ./...", Instructions: "verify old", ReferenceAssetIDs: []int64{1}}
	next := base
	next.Instructions += "\nverify additional behavior"
	next.ReferenceAssetIDs = []int64{1, 2}
	if _, e := AdditiveRevision(base, next); e != nil {
		t.Fatal(e)
	}
	for _, mutate := range []func(*Input){func(i *Input) { i.Command = "go test ./new" }, func(i *Input) { i.Command += "\ntrue" }, func(i *Input) { i.Mode = "NONE"; i.Command = "" }, func(i *Input) { i.Mode = "SCRIPT"; i.Command = ""; i.Script = "true" }, func(i *Input) { i.Instructions = "verify additional behavior" }, func(i *Input) { i.ReferenceAssetIDs = []int64{2} }} {
		bad := next
		mutate(&bad)
		if _, e := AdditiveRevision(base, bad); e == nil {
			t.Fatal("old check contract removed", bad)
		}
	}
	if _, e := AdditiveRevision(base, base); e == nil {
		t.Fatal("no-op accepted")
	}
	script := Input{Mode: "SCRIPT", Script: "exit 1", Instructions: "old"}
	extended := script
	extended.Instructions += "\nnew"
	if _, e := AdditiveRevision(script, extended); e != nil {
		t.Fatal(e)
	}
	if _, e := AdditiveRevision(Input{Mode: "NONE", Instructions: "old"}, Input{Mode: "COMMAND", Command: "go test ./...", Instructions: "old\nnew"}); e != nil {
		t.Fatal(e)
	}
}
func TestRevisionStateApplyOnce(t *testing.T) {
	s := RevisionState{Status: RevisionPending, Version: 1}
	if s.Apply(2, 3, time.Now()) == nil {
		t.Fatal("stale allowed")
	}
	if e := s.Apply(1, 3, time.Now()); e != nil || s.Status != RevisionApplied || s.Version != 2 || s.AppliedTurnID != 3 || s.AppliedAt == nil {
		t.Fatal(s, e)
	}
	if s.Apply(2, 4, time.Now()) == nil {
		t.Fatal("second apply")
	}
}
