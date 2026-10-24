package domain

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"strconv"
	"strings"
	"unicode/utf8"

	completioncheck "github.com/CaixyPromise/seven-agent/force/internal/app/workforce/completioncheck/facade"
	sharedtransport "github.com/CaixyPromise/seven-agent/force/internal/app/workforce/shared/transport"
	taskfacade "github.com/CaixyPromise/seven-agent/force/internal/app/workforce/task/facade"
)

var ErrInvalidRequest = errors.New("invalid open task request")

type CreateOpenTaskRequest struct {
	ProjectID                 int64                      `json:"-"`
	Name                      string                     `json:"name,omitempty"`
	Instruction               string                     `json:"instruction"`
	Tags                      []string                   `json:"tags,omitempty"`
	ReferenceAssetIDs         sharedtransport.Int64Slice `json:"referenceAssetIds,omitempty"`
	AdditionalCompletionCheck *completioncheck.Input     `json:"additionalCompletionCheck,omitempty"`
	ReviewEnabled             *bool                      `json:"reviewEnabled,omitempty"`
	DeliveryPreference        string                     `json:"deliveryPreference,omitempty"`
	ModelSelection            taskfacade.ModelSelection  `json:"modelSelection"`
	RuntimeReleaseID          int64                      `json:"runtimeReleaseId,string"`
	ExecutionTimeoutSeconds   int                        `json:"executionTimeoutSeconds,omitempty"`
	ExternalCorrelationID     string                     `json:"externalCorrelationId"`
}

func (r CreateOpenTaskRequest) NormalizeAndValidate() (CreateOpenTaskRequest, error) {
	r.Name = strings.TrimSpace(r.Name)
	r.Instruction = strings.TrimSpace(r.Instruction)
	r.DeliveryPreference = strings.TrimSpace(r.DeliveryPreference)
	r.ExternalCorrelationID = strings.TrimSpace(r.ExternalCorrelationID)
	if r.DeliveryPreference == "" {
		r.DeliveryPreference = "PROJECT_DEFAULT"
	}
	if r.ProjectID < 1 || r.ModelSelection.CredentialID < 1 || r.RuntimeReleaseID < 1 || r.ExecutionTimeoutSeconds < 0 || r.ExecutionTimeoutSeconds > 2147483647 || !utf8.ValidString(r.Name) || !utf8.ValidString(r.Instruction) || !utf8.ValidString(r.ExternalCorrelationID) {
		return CreateOpenTaskRequest{}, ErrInvalidRequest
	}
	if utf8.RuneCountInString(r.Name) > 120 || !taskfacade.ValidInstruction(r.Instruction) || utf8.RuneCountInString(r.ExternalCorrelationID) < 1 || utf8.RuneCountInString(r.ExternalCorrelationID) > 128 {
		return CreateOpenTaskRequest{}, ErrInvalidRequest
	}
	r.ModelSelection.Model = strings.TrimSpace(r.ModelSelection.Model)
	if r.ModelSelection.Model == "" || len(r.ModelSelection.Model) > 256 || strings.IndexFunc(r.ModelSelection.Model, func(value rune) bool { return value < 32 || value == 127 }) >= 0 {
		return CreateOpenTaskRequest{}, ErrInvalidRequest
	}
	if r.DeliveryPreference != "PROJECT_DEFAULT" && r.DeliveryPreference != "ASK_AFTER_GATES" && r.DeliveryPreference != "SAVE_ONLY" {
		return CreateOpenTaskRequest{}, ErrInvalidRequest
	}
	if len(r.Tags) > 20 || len(r.ReferenceAssetIDs) > 50 {
		return CreateOpenTaskRequest{}, ErrInvalidRequest
	}
	seenTags := make(map[string]struct{}, len(r.Tags))
	for index, tag := range r.Tags {
		tag = strings.TrimSpace(tag)
		if !utf8.ValidString(tag) || utf8.RuneCountInString(tag) < 1 || utf8.RuneCountInString(tag) > 64 {
			return CreateOpenTaskRequest{}, ErrInvalidRequest
		}
		if _, duplicate := seenTags[tag]; duplicate {
			return CreateOpenTaskRequest{}, ErrInvalidRequest
		}
		seenTags[tag] = struct{}{}
		r.Tags[index] = tag
	}
	for _, assetID := range r.ReferenceAssetIDs {
		if assetID < 1 {
			return CreateOpenTaskRequest{}, ErrInvalidRequest
		}
	}
	return r, nil
}

func (r CreateOpenTaskRequest) ProjectTaskRequest() taskfacade.ProjectTaskRequest {
	return taskfacade.ProjectTaskRequest{
		Instruction: r.Instruction, Name: r.Name, Tags: append([]string(nil), r.Tags...),
		ReferenceAssetIDs:         append(sharedtransport.Int64Slice(nil), r.ReferenceAssetIDs...),
		AdditionalCompletionCheck: r.AdditionalCompletionCheck,
		ReviewEnabled:             r.ReviewEnabled, DeliveryPreference: r.DeliveryPreference,
		ModelSelection: r.ModelSelection, RuntimeReleaseID: r.RuntimeReleaseID, ExecutionTimeoutSeconds: r.ExecutionTimeoutSeconds,
	}
}

func InternalIdempotencyKey(applicationID int64, clientKey string) string {
	sum := sha256.Sum256([]byte(strconv.FormatInt(applicationID, 10) + "\x00" + strings.TrimSpace(clientKey)))
	return "open:" + hex.EncodeToString(sum[:])
}

// Preserve the canonical decimal-string ID contract also for direct REST
// clients. encoding/json's ,string alone accepts signs and leading zeroes.
func (r *CreateOpenTaskRequest) UnmarshalJSON(body []byte) error {
	type plain CreateOpenTaskRequest
	var decoded plain
	decoder := json.NewDecoder(bytes.NewReader(body))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&decoded); err != nil {
		return ErrInvalidRequest
	}
	if err := decoder.Decode(new(any)); err != io.EOF {
		return ErrInvalidRequest
	}
	var wire struct {
		RuntimeReleaseID *string `json:"runtimeReleaseId"`
		ModelSelection   struct {
			CredentialID *string `json:"credentialId"`
		} `json:"modelSelection"`
	}
	if err := json.Unmarshal(body, &wire); err != nil {
		return ErrInvalidRequest
	}
	for _, id := range []struct {
		raw   *string
		value int64
	}{
		{wire.RuntimeReleaseID, decoded.RuntimeReleaseID}, {wire.ModelSelection.CredentialID, decoded.ModelSelection.CredentialID},
	} {
		if id.raw != nil && (id.value <= 0 || strconv.FormatInt(id.value, 10) != *id.raw) {
			return ErrInvalidRequest
		}
	}
	*r = CreateOpenTaskRequest(decoded)
	return nil
}
