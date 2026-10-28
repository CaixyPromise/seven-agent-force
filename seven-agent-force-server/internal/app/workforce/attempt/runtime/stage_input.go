package runtime

import (
	"encoding/json"
	"errors"
	task "github.com/CaixyPromise/seven-agent/force/internal/app/workforce/task/domain"
	"os"
	"path/filepath"
	"strconv"
)

func (d *Docker) materializeStageInput(item task.WorkItem, attempt task.Attempt, space Workspace, containerRoot, instruction string) (string, error) {
	purpose := item.ExecutionSnapshot.StagePurpose
	if purpose == "" {
		return instruction, nil
	}
	if purpose != "PLAN" && purpose != "IMPLEMENT" {
		return "", errors.New("STAGE_PURPOSE_INVALID")
	}
	if purpose == "PLAN" {
		p := filepath.Join(filepath.Dir(space.SpecPath), "planning-source")
		info, e := os.Lstat(p)
		if e != nil || !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
			return "", errors.New("PLANNING_SOURCE_MISSING")
		}
		instruction += "\nRead project source at " + filepath.Join(containerRoot, "input", "attempts", strconv.FormatInt(attempt.ID, 10), "planning-source") + ". This source is read-only; do not implement changes. Write plan.json in the writable workspace using schema seven.force.development-plan/v1 with title, summary, goals, nonGoals, approach, affectedAreas (path, reason), steps (key, description), acceptanceCriteria, risks, openQuestions (key, question, blocking, optional resolution), and markdown."
		if len(item.ExecutionSnapshot.ApprovedPlan) == 0 {
			return instruction, nil
		}
		if len(item.ExecutionSnapshot.ApprovedPlan) > 256*1024 || !json.Valid(item.ExecutionSnapshot.ApprovedPlan) {
			return "", errors.New("PREVIOUS_PLAN_INVALID")
		}
		storage, e := d.persistentWorkspace()
		if e != nil {
			return "", e
		}
		if e = storage.WriteAtomic(filepath.Join(filepath.Dir(space.SpecPath), "previous-plan.json"), item.ExecutionSnapshot.ApprovedPlan, 0444); e != nil {
			return "", e
		}
		return instruction + "\nThe previous plan for revision is at " + filepath.Join(containerRoot, "input", "attempts", strconv.FormatInt(attempt.ID, 10), "previous-plan.json") + ".", nil
	}
	body := item.ExecutionSnapshot.ApprovedPlan
	if len(body) == 0 || len(body) > 256*1024 || !json.Valid(body) {
		return "", errors.New("APPROVED_PLAN_MISSING")
	}
	storage, e := d.persistentWorkspace()
	if e != nil {
		return "", e
	}
	if e = storage.WriteAtomic(filepath.Join(filepath.Dir(space.SpecPath), "approved-plan.json"), body, 0444); e != nil {
		return "", e
	}
	return instruction + "\nThe exact human-approved plan is at " + filepath.Join(containerRoot, "input", "attempts", strconv.FormatInt(attempt.ID, 10), "approved-plan.json") + ". Its goals and acceptance criteria are authoritative; implementation details may be explored freely.", nil
}
