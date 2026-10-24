package controller

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/CaixyPromise/seven-agent/force/internal/app/workforce/project/domain"
	"github.com/CaixyPromise/seven-agent/force/internal/common/response"
	"github.com/cloudwego/hertz/pkg/app"
)

func TestProjectViewExposesBusinessConfigurationWithoutInfrastructureBindings(t *testing.T) {
	ctx := &app.RequestContext{}
	response.Success(ctx, projectView(domain.Project{
		ID:                             9_001,
		TenantID:                       9_002,
		ProjectKey:                     "seven-workforce",
		Name:                           "Seven Agent Force",
		Kind:                           "CODEBASE",
		Status:                         "ACTIVE",
		VersionID:                      7,
		RepositoryProviderType:         "GITHUB",
		RepositoryDisplayURL:           "https://github.com/CaixyPromise/seven-workforce.git",
		RepositoryCredentialCiphertext: "ciphertext",
		RepositoryCredentialVersion:    1,
		RepositorySourceState:          "READY",
		RepositoryCapabilities:         domain.RepositoryCapabilities{Read: true, Push: true, DraftReview: true},
		DefaultRefName:                 "main",
		TechStack:                      []string{"Go", "TypeScript"},
		CompletionCheck:                json.RawMessage(`{"mode":"COMMAND","command":"make check"}`),
		CompletionCheckDigest:          strings.Repeat("a", 64),
		ResultProcessorReleaseID:       9_004,
		ResultProcessorParameters:      json.RawMessage(`{"private":"value"}`),
		ReviewPrompt:                   "重点检查回归风险。",
		DeliveryMode:                   "NEW_BRANCH",
		DeliveryAuthorization:          "AUTO_AFTER_GATES",
		NotificationEnabled:            true,
		NotificationGroupRouteID:       9_006,
		NotificationGroupRouteName:     "研发通知群",
	}))

	var envelope struct {
		Data map[string]any `json:"data"`
	}
	if err := json.Unmarshal(ctx.Response.Body(), &envelope); err != nil {
		t.Fatalf("decode Project response: %v; body=%s", err, ctx.Response.Body())
	}
	body := string(ctx.Response.Body())
	for _, required := range []string{
		`"id":"9001"`,
		`"versionId":"7"`,
		`"displayUrl":"https://github.com/CaixyPromise/seven-workforce.git"`,
		`"credentialConfigured":true`,
		`"command":"make check"`,
		`"reviewPrompt":"重点检查回归风险。"`,
		`"groupRouteName":"研发通知群"`,
	} {
		if !strings.Contains(body, required) {
			t.Fatalf("Project response lost business field %q: %s", required, body)
		}
	}
	for _, forbidden := range []string{
		"completionCheckDigest",
		"resultProcessorReleaseId",
		"resultProcessorParameters",
		"reviewProfileId",
		"providerAlias",
		"providerModel",
		"runtimeReleaseId",
		"harnessAlias",
		"workerId",
		"selectionDigest",
		"private",
		"ciphertext",
	} {
		if strings.Contains(body, forbidden) {
			t.Fatalf("Project response leaked internal field %q: %s", forbidden, body)
		}
	}
}

func TestProjectViewOmitsDeliveryAuthorizationWhenDeliveryIsDisabled(t *testing.T) {
	ctx := &app.RequestContext{}
	response.Success(ctx, projectView(domain.Project{
		ID:                    9_001,
		TenantID:              9_002,
		ProjectKey:            "general",
		Name:                  "General",
		Kind:                  "GENERAL",
		Status:                "ACTIVE",
		VersionID:             1,
		DeliveryMode:          "NONE",
		DeliveryAuthorization: "NONE",
		CompletionCheck:       json.RawMessage(`{"mode":"NONE"}`),
		TechStack:             []string{},
		NotificationEvents:    []string{},
	}))

	var envelope struct {
		Data struct {
			Delivery map[string]any `json:"delivery"`
		} `json:"data"`
	}
	if err := json.Unmarshal(ctx.Response.Body(), &envelope); err != nil {
		t.Fatalf("decode Project response: %v; body=%s", err, ctx.Response.Body())
	}
	if envelope.Data.Delivery["mode"] != "NONE" {
		t.Fatalf("unexpected delivery mode: %#v", envelope.Data.Delivery)
	}
	if _, exists := envelope.Data.Delivery["authorization"]; exists {
		t.Fatalf("disabled delivery must not expose an authorization: %#v", envelope.Data.Delivery)
	}
	if _, exists := envelope.Data.Delivery["resultBranchPattern"]; exists {
		t.Fatalf("disabled delivery must not expose a branch pattern: %#v", envelope.Data.Delivery)
	}
}
