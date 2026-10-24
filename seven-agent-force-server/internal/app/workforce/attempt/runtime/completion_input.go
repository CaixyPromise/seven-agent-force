package runtime

import (
	"encoding/json"
	"errors"
	"path/filepath"
	"strconv"

	check "github.com/CaixyPromise/seven-agent/force/internal/app/workforce/completioncheck/facade"
	"github.com/CaixyPromise/seven-agent/force/internal/app/workforce/task/domain"
)

func (d *Docker) materializeCompletionCheckInput(item domain.WorkItem, attempt domain.Attempt, workspace Workspace, containerTaskRoot string) (string, error) {
	if item.CompletionCheck == nil {
		return item.Instruction, nil
	}
	digest, err := check.FrozenDigest(*item.CompletionCheck, item.CompletionCheckReferences)
	if err != nil || digest != item.CompletionCheckDigest || item.TaskID <= 0 || attempt.ID <= 0 {
		return "", errors.New("CHECK_FROZEN_IDENTITY_INVALID")
	}
	containerPath := filepath.Join(containerTaskRoot, "input", "attempts", strconv.FormatInt(attempt.ID, 10), "completion-check.json")
	type reference struct {
		ID     int64  `json:"id,string"`
		Path   string `json:"path"`
		Digest string `json:"contentSha256"`
	}
	refs := make([]reference, 0, len(item.CompletionCheckReferences))
	for _, ref := range item.CompletionCheckReferences {
		name := filepath.Base(ref.Name)
		if name == "" || name == "." || name == ".." {
			return "", errors.New("CHECK_REFERENCE_INVALID")
		}
		refs = append(refs, reference{ref.ReferenceAssetID, filepath.Join(containerTaskRoot, "input", "references", strconv.FormatInt(ref.ReferenceAssetID, 10), name), ref.ContentSHA256})
	}
	body, err := json.Marshal(struct {
		Schema     string       `json:"schema"`
		Input      *check.Input `json:"completionCheck"`
		Digest     string       `json:"completionCheckDigest"`
		References []reference  `json:"references"`
	}{"seven.completion-check-input/v1", item.CompletionCheck, digest, refs})
	if err != nil {
		return "", errors.New("CHECK_INPUT_INVALID")
	}
	storage, err := d.persistentWorkspace()
	if err != nil {
		return "", err
	}
	target := filepath.Join(workspace.InputRoot, "attempts", strconv.FormatInt(attempt.ID, 10), "completion-check.json")
	if err = storage.ensureDirectory(filepath.Dir(target), 0755); err != nil {
		return "", err
	}
	if err = storage.WriteAtomic(target, body, 0444); err != nil {
		return "", err
	}
	return item.Instruction + "\n\n[Task completion criteria]\nRead the immutable task-local checks, acceptance notes, references and review focus at " + containerPath + " (digest " + digest + "). Preserve the original task scope and mandatory checks. Do not weaken checks or repair unrelated historical problems to obtain a passing result. The platform runs the completion check independently; reading this file is not test evidence.\n", nil
}
