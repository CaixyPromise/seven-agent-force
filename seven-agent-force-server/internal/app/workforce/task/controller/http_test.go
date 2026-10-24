package controller

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	interactionfacade "github.com/CaixyPromise/seven-agent/force/internal/app/workforce/interaction/facade"
	"github.com/CaixyPromise/seven-agent/force/internal/app/workforce/task/domain"
	"github.com/CaixyPromise/seven-agent/force/internal/common/response"
	"github.com/CaixyPromise/seven-agent/force/internal/shared/securitycontext"
	"github.com/cloudwego/hertz/pkg/app"
)

type taskSummaryFixture struct {
	t           *testing.T
	canContinue bool
	err         error
}

func (f taskSummaryFixture) TaskSummaries(_ context.Context, tenant, user int64, ids []int64, canContinue bool) (map[int64]interactionfacade.TaskSummary, error) {
	if tenant != 9002 || user != 7 || len(ids) != 1 || ids[0] != 9001 || canContinue != f.canContinue {
		f.t.Fatalf("summary scope mismatch: tenant=%d user=%d ids=%v continue=%v", tenant, user, ids, canContinue)
	}
	return map[int64]interactionfacade.TaskSummary{9001: {PendingActionCount: 1, FirstPendingActionKind: "CONVERSATION_ITEM", FirstPendingActionID: 9003}}, f.err
}

func TestTaskDetailSummaryKeepsTerminalDeliveryPendingAndScope(t *testing.T) {
	for _, canContinue := range []bool{true, false} {
		c := &app.RequestContext{}
		permissions := []string{}
		if canContinue {
			permissions = append(permissions, "workforce:task:continue")
		}
		securitycontext.Set(c, &securitycontext.UserContext{UserID: 7, Permissions: permissions})
		h := &Handler{interactions: taskSummaryFixture{t: t, canContinue: canContinue}}
		items, err := h.taskResponsesWithSummaries(context.Background(), c, []domain.Task{{ID: 9001, TenantID: 9002, Status: domain.TaskSucceeded}})
		if err != nil || len(items) != 1 || items[0].PendingActionCount != 1 || items[0].FirstPendingActionID != 9003 || items[0].Status != domain.TaskSucceeded {
			t.Fatalf("terminal delivery summary lost: %v, %v", items, err)
		}
		unavailable := errors.New("fixture summary unavailable")
		h.interactions = taskSummaryFixture{t: t, canContinue: canContinue, err: unavailable}
		if _, err := h.taskResponsesWithSummaries(context.Background(), c, []domain.Task{{ID: 9001, TenantID: 9002}}); !errors.Is(err, unavailable) {
			t.Fatalf("summary failure became a false zero: %v", err)
		}
	}
}

func TestTaskCreateErrorCollapsesRawPersistenceDetail(t *testing.T) {
	raw := errors.New("pq: check violation while inserting snapshot; dsn=postgres://operator:secret@db.internal/workforce")
	status, code, message := taskCreateError(raw)
	if status != 500 || code != "TASK_CREATION_FAILED" {
		t.Fatalf("raw error mapping=(%d,%q,%q), want 500/TASK_CREATION_FAILED", status, code, message)
	}
	context := &app.RequestContext{}
	writeError(context, status, code, message)
	body := string(context.Response.Body())
	for _, forbidden := range []string{"pq:", "dsn=", "secret", "db.internal"} {
		if strings.Contains(body, forbidden) {
			t.Fatalf("create response leaked %q: %s", forbidden, body)
		}
	}
}

func TestTaskCreateErrorExposesSafeFinalSelectionReasons(t *testing.T) {
	for _, reason := range []string{"MODEL_CREDENTIAL_NOT_FOUND", "MODEL_CREDENTIAL_DISABLED", "TASK_MODEL_NOT_IN_CATALOG", "TASK_MODEL_SELECTION_REQUIRED"} {
		status, code, message := taskCreateError(errors.New(reason))
		if status != 400 || code != reason || message != "task request is invalid" {
			t.Fatalf("reason %s: %d %s %s", reason, status, code, message)
		}
	}
}

func TestTaskCreatorResponseExposesOnlyIndependentReviewBusinessSummary(t *testing.T) {
	task := domain.Task{
		ID:            9_001,
		TenantID:      9_002,
		Name:          "safe review response",
		ReviewEnabled: true,
	}
	body, err := json.Marshal(taskCreatorResponse(task))
	if err != nil {
		t.Fatalf("marshal task creator response: %v", err)
	}
	serialized := string(body)
	for _, forbidden := range []string{
		"executionSnapshot", "reviewProfileId", "reviewProfileDigest", "reviewerImageDigest",
		"harnessAlias", "harnessReleaseId", "hookSelectionMode", "hookBundleReleaseId",
		"providerAlias", "providerModel", "harnessTimeoutSeconds", "validationCommands",
		"resultProcessorSelectionMode", "resultProcessorParameters", "resultProcessorConfigOverrides",
		"reviewRequiredAdapterCapabilities", "reviewRequiredProviderCapabilities", "rubricReleaseId",
		"rubricReleaseDigest", "reviewSystemContract", "reviewReportSchemaVersion", "reviewHardTimeoutSeconds",
		"reviewTransientRetryLimit", "reviewAutomaticRevisionLimit", "reviewCommandSandboxPolicy", "reviewNetworkSandboxPolicy",
		"structuredOutput", "sha256:",
	} {
		if strings.Contains(serialized, forbidden) {
			t.Fatalf("task creator response leaked %q: %s", forbidden, serialized)
		}
	}
	if !strings.Contains(serialized, `"independentReview":{"required":true}`) {
		t.Fatalf("task creator response did not expose required business summary: %s", serialized)
	}
	if !strings.Contains(serialized, `"id":"9001"`) || !strings.Contains(serialized, `"tenantId":"9002"`) {
		t.Fatalf("task creator response lost decimal-string identities: %s", serialized)
	}
}

func TestTaskCreatorResponseKeepsReviewDisabledExplicit(t *testing.T) {
	body, err := json.Marshal(taskCreatorResponse(domain.Task{ID: 9_006, ReviewEnabled: false}))
	if err != nil {
		t.Fatalf("marshal disabled task creator response: %v", err)
	}
	if strings.Contains(string(body), "executionSnapshot") || !strings.Contains(string(body), `"independentReview":{"required":false}`) {
		t.Fatalf("disabled task creator response is not a safe explicit summary: %s", body)
	}
}

func TestTaskCreatorResponseExposesOnlySafeResolvedAssignmentDiagnostics(t *testing.T) {
	task := domain.Task{
		ID:       9_011,
		TenantID: 9_012,
		Execution: &domain.ExecutionView{
			ExecutionNodeID: 9_013,
			WorkerID:        9_014,
			Resolved: &domain.ResolvedExecutionView{
				RuntimeReleaseID:      9_015,
				RuntimeAlias:          "codex-production",
				RuntimeImageDigest:    "sha256:" + strings.Repeat("a", 64),
				AdapterID:             "codex-app-server",
				AdapterVersion:        "0.147.0",
				AdapterProtocol:       "seven.agent-adapter/v1",
				AdapterCapabilities:   []string{"interaction.request", "tool.request"},
				ModelCredentialID:     9_016,
				CredentialVersion:     3,
				ModelCredentialName:   "local-api",
				ModelProtocol:         "OPENAI",
				PrimaryModel:          "gpt-5.6-luna",
				WorkerImageContract:   "seven.worker-image/v1",
				IsolationProfile:      "seven.worker-image/v1",
				SelectionDigest:       strings.Repeat("c", 64),
				ExecutionDeadlineSecs: 3600,
			},
		},
	}
	body, err := json.Marshal(taskCreatorResponse(task))
	if err != nil {
		t.Fatalf("marshal task diagnostics: %v", err)
	}
	serialized := string(body)
	for _, required := range []string{`"runtimeAlias":"codex-production"`, `"modelCredentialName":"local-api"`, `"modelProtocol":"OPENAI"`, `"primaryModel":"gpt-5.6-luna"`, `"selectionDigest":"`} {
		if !strings.Contains(serialized, required) {
			t.Fatalf("task response lost safe assignment diagnostic %q: %s", required, serialized)
		}
	}
	for _, forbidden := range []string{"providerKey", "apiKey", "secretCiphertext", "secretEdek", "gatewayToken", "rawProviderConfig", "executionSnapshot"} {
		if strings.Contains(strings.ToLower(serialized), strings.ToLower(forbidden)) {
			t.Fatalf("task response leaked private assignment field %q: %s", forbidden, serialized)
		}
	}
}

func TestTaskCreatorResponseStaysFlatThroughResponseJSONNormalization(t *testing.T) {
	ctx := &app.RequestContext{}
	response.Success(ctx, []taskResponse{taskCreatorResponse(domain.Task{
		ID:            9_101,
		TenantID:      9_102,
		Name:          "flat task wire contract",
		Status:        domain.TaskReviewing,
		ReviewEnabled: true,
	})})

	var body struct {
		Data []map[string]any `json:"data"`
	}
	if err := json.Unmarshal(ctx.Response.Body(), &body); err != nil {
		t.Fatalf("decode response body: %v; body=%s", err, ctx.Response.Body())
	}
	if len(body.Data) != 1 {
		t.Fatalf("response task count=%d, want 1", len(body.Data))
	}
	task := body.Data[0]
	if _, nested := task["Task"]; nested {
		t.Fatalf("Task response regressed to nested Task object: %#v", task)
	}
	if got := task["status"]; got != string(domain.TaskReviewing) {
		t.Fatalf("flat status=%#v, want %q", got, domain.TaskReviewing)
	}
	if got := task["id"]; got != "9101" {
		t.Fatalf("flat decimal-string id=%#v, want %q", got, "9101")
	}
	independentReview, ok := task["independentReview"].(map[string]any)
	if !ok || independentReview["required"] != true {
		t.Fatalf("independent review summary=%#v, want explicit required=true", task["independentReview"])
	}
}
