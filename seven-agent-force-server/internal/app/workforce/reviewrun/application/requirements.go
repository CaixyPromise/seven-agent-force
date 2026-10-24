package application

import (
	"encoding/json"
	"strconv"
	"unicode"
	"unicode/utf8"

	"github.com/CaixyPromise/seven-agent/force/internal/app/workforce/reviewrun/domain"
	taskfacade "github.com/CaixyPromise/seven-agent/force/internal/app/workforce/task/facade"
)

// reviewRequirements is an explicit allowlist, not serialization of a Task
// snapshot. The Reviewer receives declarative text and read-only reference
// metadata, never the executable command/script or host/object storage locations.
// ContainerPath is the deterministic target for separately verified material;
// its presence is not evidence of availability or of the Reviewer reading it.
func reviewRequirements(input taskfacade.IndependentReviewInput) ([]byte, error) {
	if !domain.ValidCompletionCheckDigest(input.CompletionCheckDigest) || !validCriteriaText(input.Instructions) || !validCriteriaText(input.ReviewPrompt) || len(input.References) > 64 {
		return nil, domain.ErrInvalidReviewInput
	}
	if input.CompletionCheckDigest == "" && (input.Instructions != "" || len(input.References) != 0) {
		return nil, domain.ErrInvalidReviewInput
	}
	type reference struct {
		ReferenceAssetID int64  `json:"referenceAssetId,string"`
		Name             string `json:"name"`
		ContentSHA256    string `json:"contentSha256"`
		SizeBytes        int64  `json:"sizeBytes,string"`
		ReadOnly         bool   `json:"readOnly"`
		ContainerPath    string `json:"containerPath"`
	}
	refs := make([]reference, 0, len(input.References))
	seen := make(map[int64]bool, len(input.References))
	for _, ref := range input.References {
		if ref.ReferenceAssetID < 1 || seen[ref.ReferenceAssetID] || ref.SizeBytes < 0 || ref.ContentSHA256 == "" || !domain.ValidCompletionCheckDigest(ref.ContentSHA256) {
			return nil, domain.ErrInvalidReviewInput
		}
		seen[ref.ReferenceAssetID] = true
		refs = append(refs, reference{
			ReferenceAssetID: ref.ReferenceAssetID, Name: ref.Name,
			ContentSHA256: ref.ContentSHA256, SizeBytes: ref.SizeBytes, ReadOnly: true,
			ContainerPath: "/review/references/" + strconv.FormatInt(ref.ReferenceAssetID, 10) + "/content",
		})
	}
	return json.Marshal(struct {
		Instruction           string      `json:"instruction"`
		CompletionCheckDigest string      `json:"completionCheckDigest,omitempty"`
		Instructions          string      `json:"instructions,omitempty"`
		ReviewPrompt          string      `json:"reviewPrompt,omitempty"`
		References            []reference `json:"references,omitempty"`
	}{input.Instruction, input.CompletionCheckDigest, input.Instructions, input.ReviewPrompt, refs})
}

func validCriteriaText(value string) bool {
	if len(value) > 16*1024 || !utf8.ValidString(value) {
		return false
	}
	for _, r := range value {
		if unicode.IsControl(r) && r != '\n' && r != '\t' {
			return false
		}
	}
	return true
}
