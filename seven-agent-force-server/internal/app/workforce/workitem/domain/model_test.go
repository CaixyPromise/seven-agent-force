package domain

import (
	"encoding/json"
	"testing"
)

func validPlan() []byte {
	return []byte(`{"schema":"seven.force.development-plan/v1","title":"方案","summary":"目标","goals":["完成"],"nonGoals":[],"approach":"探索后实现","affectedAreas":[{"path":"src","reason":"实现"}],"steps":[{"key":"1","description":"实现"}],"acceptanceCriteria":["可验证"],"risks":[],"openQuestions":[{"key":"q","question":"范围？","blocking":true}],"markdown":"说明","source":{"commit":"untrusted"}}`)
}
func TestPlanValidationAndApprovalBinding(t *testing.T) {
	p, err := ParsePlan(validPlan())
	if err != nil {
		t.Fatal(err)
	}
	if p.CandidateSource != nil || len(p.AffectedAreas) != 1 || !p.HasBlockingQuestions() {
		t.Fatal("source must be platform-owned and blocking question retained")
	}
	rev := Revision{ID: 123, Plan: p}
	_, rev.Digest = rev.DigestBody()
	item := Item{Status: "AWAITING_PLAN_APPROVAL", Version: 4, CurrentPlan: &rev}
	if item.CanApprove(4, 123, rev.Digest) == nil {
		t.Fatal("blocking question allowed")
	}
	p.OpenQuestions[0].Resolution = "确定"
	rev.Plan = p
	_, rev.Digest = rev.DigestBody()
	if item.CanApprove(4, 123, rev.Digest) != nil {
		t.Fatal("resolved plan rejected")
	}
	for _, x := range []struct {
		v, id  int64
		digest string
	}{{3, 123, rev.Digest}, {4, 124, rev.Digest}, {4, 123, "stale"}} {
		if item.CanApprove(x.v, x.id, x.digest) == nil {
			t.Fatal("stale decision accepted")
		}
	}
}
func TestPlanRejectsUnknownFieldsAndOversize(t *testing.T) {
	var obj map[string]any
	_ = json.Unmarshal(validPlan(), &obj)
	obj["surprise"] = "unreviewed"
	b, _ := json.Marshal(obj)
	if _, err := ParsePlan(b); err == nil {
		t.Fatal("unknown field accepted")
	}
	if _, err := ParsePlan(make([]byte, MaxPlanBytes+1)); err == nil {
		t.Fatal("oversize accepted")
	}
}
