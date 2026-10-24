package mcpserver

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	completioncheck "github.com/CaixyPromise/seven-agent/force/internal/app/workforce/completioncheck/facade"
	sharedtransport "github.com/CaixyPromise/seven-agent/force/internal/app/workforce/shared/transport"
	taskfacade "github.com/CaixyPromise/seven-agent/force/internal/app/workforce/task/facade"
)

type requestIdentity struct {
	authorization string
	capabilities  Capabilities
}

type identityContextKey struct{}

func withIdentity(ctx context.Context, identity requestIdentity) context.Context {
	return context.WithValue(ctx, identityContextKey{}, identity)
}

func identityFromContext(ctx context.Context) (requestIdentity, bool) {
	identity, ok := ctx.Value(identityContextKey{}).(requestIdentity)
	return identity, ok
}

type createTaskInput struct {
	Tags                      []string                   `json:"tags,omitempty"`
	ReferenceAssetIDs         sharedtransport.Int64Slice `json:"referenceAssetIds,omitempty"`
	AdditionalCompletionCheck *completioncheck.Input     `json:"additionalCompletionCheck,omitempty"`
	ReviewEnabled             *bool                      `json:"reviewEnabled,omitempty"`
	DeliveryPreference        string                     `json:"deliveryPreference,omitempty"`
	ModelSelection            taskfacade.ModelSelection  `json:"modelSelection"`
	RuntimeReleaseID          int64                      `json:"runtimeReleaseId,string"`
	ExecutionTimeoutSeconds   int                        `json:"executionTimeoutSeconds,omitempty"`

	IdempotencyKey        string `json:"idempotencyKey"`
	ProjectID             string `json:"projectId"`
	Name                  string `json:"name"`
	Instruction           string `json:"instruction"`
	ExternalCorrelationID string `json:"externalCorrelationId"`
}

type taskIDInput struct {
	TaskID string `json:"taskId"`
}

type readOutputInput struct {
	TaskID   string `json:"taskId"`
	OutputID string `json:"outputId"`
}

type listOutputsOutput struct {
	TaskID  string   `json:"taskId"`
	Outputs []Output `json:"outputs"`
}

type verifiedOutput struct {
	OutputID  string `json:"outputId"`
	Name      string `json:"name"`
	MediaType string `json:"mediaType"`
	SHA256    string `json:"sha256"`
	SizeBytes string `json:"sizeBytes"`
	Verified  bool   `json:"verified"`
}

type toolErrorView struct {
	Error toolError `json:"error"`
}

type toolError struct {
	Code      string `json:"code"`
	Message   string `json:"message"`
	Retryable bool   `json:"retryable"`
}

type modelOptionsInput struct {
	ProjectID string `json:"projectId"`
}
type runtimeOptionsInput struct {
	ProjectID         string `json:"projectId"`
	ModelCredentialID string `json:"modelCredentialId"`
}

func registerTools(server *mcp.Server, gateway WorkforceGateway, maxTextBytes int64) {
	mcp.AddTool(server, &mcp.Tool{Name: "list_model_options", Description: "List configured model credential references for an authorized active Project; no provider calls or secrets.", InputSchema: objectSchema(map[string]any{"projectId": stringLimits("Project ID.", 1, 19, `^[1-9][0-9]{0,18}$`)}, []string{"projectId"})}, func(ctx context.Context, _ *mcp.CallToolRequest, input modelOptionsInput) (*mcp.CallToolResult, any, error) {
		identity, ok := identityFromContext(ctx)
		if !ok {
			return toolFailure("UNAUTHORIZED", "The API key is invalid or expired.", false), nil, nil
		}
		projectID, valid := parsePublicID(input.ProjectID)
		if !valid {
			return toolFailure("INVALID_REQUEST", "The project ID is invalid.", false), nil, nil
		}
		result, err := gateway.ListModelOptions(ctx, identity.authorization, projectID)
		if err != nil {
			return gatewayToolFailure(err), nil, nil
		}
		return structuredSuccess(result), nil, nil
	})
	mcp.AddTool(server, &mcp.Tool{Name: "list_runtime_options", Description: "List compatible published RuntimeRelease references for a Project and model credential. Discovery does not reserve a runtime; creation revalidates.", InputSchema: objectSchema(map[string]any{"projectId": stringLimits("Project ID.", 1, 19, `^[1-9][0-9]{0,18}$`), "modelCredentialId": stringLimits("Model credential reference.", 1, 19, `^[1-9][0-9]{0,18}$`)}, []string{"projectId", "modelCredentialId"})}, func(ctx context.Context, _ *mcp.CallToolRequest, input runtimeOptionsInput) (*mcp.CallToolResult, any, error) {
		identity, ok := identityFromContext(ctx)
		if !ok {
			return toolFailure("UNAUTHORIZED", "The API key is invalid or expired.", false), nil, nil
		}
		projectID, valid := parsePublicID(input.ProjectID)
		credentialID, credentialValid := parsePublicID(input.ModelCredentialID)
		if !valid || !credentialValid {
			return toolFailure("INVALID_REQUEST", "The project or credential ID is invalid.", false), nil, nil
		}
		result, err := gateway.ListRuntimeOptions(ctx, identity.authorization, projectID, credentialID)
		if err != nil {
			return gatewayToolFailure(err), nil, nil
		}
		return structuredSuccess(result), nil, nil
	})

	mcp.AddTool(server, &mcp.Tool{Name: "create_task", Description: "Create an asynchronous Project task. Requires modelSelection (credentialId reference and model), runtimeReleaseId and an idempotency key. Use list_model_options and list_runtime_options to discover authorized selections; never supply a credential secret.", InputSchema: createTaskSchema()},
		func(ctx context.Context, _ *mcp.CallToolRequest, input createTaskInput) (*mcp.CallToolResult, any, error) {
			identity, ok := identityFromContext(ctx)
			if !ok {
				return toolFailure("UNAUTHORIZED", "The API key is invalid or expired.", false), nil, nil
			}
			projectID, valid := parsePublicID(input.ProjectID)
			if !valid {
				return toolFailure("INVALID_REQUEST", "The project ID is invalid.", false), nil, nil
			}
			result, err := gateway.CreateTask(ctx, identity.authorization, CreateTaskRequest{
				IdempotencyKey: input.IdempotencyKey, ProjectID: projectID, Name: input.Name,
				Instruction: input.Instruction, ExternalCorrelationID: input.ExternalCorrelationID,
				Tags: input.Tags, ReferenceAssetIDs: input.ReferenceAssetIDs, AdditionalCompletionCheck: input.AdditionalCompletionCheck,
				ReviewEnabled: input.ReviewEnabled, DeliveryPreference: input.DeliveryPreference,
				ModelSelection: input.ModelSelection, RuntimeReleaseID: input.RuntimeReleaseID, ExecutionTimeoutSeconds: input.ExecutionTimeoutSeconds,
			})
			if err != nil {
				return gatewayToolFailure(err), nil, nil
			}
			return structuredSuccess(result), nil, nil
		})

	mcp.AddTool(server, &mcp.Tool{Name: "get_task", Description: "Get the public state and safe result summary for a task.", InputSchema: taskIDSchema()},
		func(ctx context.Context, _ *mcp.CallToolRequest, input taskIDInput) (*mcp.CallToolResult, any, error) {
			identity, ok := identityFromContext(ctx)
			if !ok {
				return toolFailure("UNAUTHORIZED", "The API key is invalid or expired.", false), nil, nil
			}
			taskID, valid := parsePublicID(input.TaskID)
			if !valid {
				return toolFailure("INVALID_REQUEST", "The task ID is invalid.", false), nil, nil
			}
			result, err := gateway.GetTask(ctx, identity.authorization, taskID)
			if err != nil {
				return gatewayToolFailure(err), nil, nil
			}
			return structuredSuccess(result), nil, nil
		})

	mcp.AddTool(server, &mcp.Tool{Name: "list_task_outputs", Description: "List immediately available declared outputs for a task.", InputSchema: taskIDSchema()},
		func(ctx context.Context, _ *mcp.CallToolRequest, input taskIDInput) (*mcp.CallToolResult, any, error) {
			identity, ok := identityFromContext(ctx)
			if !ok {
				return toolFailure("UNAUTHORIZED", "The API key is invalid or expired.", false), nil, nil
			}
			taskID, valid := parsePublicID(input.TaskID)
			if !valid {
				return toolFailure("INVALID_REQUEST", "The task ID is invalid.", false), nil, nil
			}
			outputs, err := gateway.ListOutputs(ctx, identity.authorization, taskID)
			if err != nil {
				return gatewayToolFailure(err), nil, nil
			}
			if outputs == nil {
				outputs = make([]Output, 0)
			}
			return structuredSuccess(listOutputsOutput{TaskID: input.TaskID, Outputs: outputs}), nil, nil
		})

	mcp.AddTool(server, &mcp.Tool{Name: "read_task_output", Description: "Read and verify a small UTF-8 text or JSON task output.", InputSchema: readOutputSchema()},
		func(ctx context.Context, _ *mcp.CallToolRequest, input readOutputInput) (*mcp.CallToolResult, any, error) {
			identity, ok := identityFromContext(ctx)
			if !ok {
				return toolFailure("UNAUTHORIZED", "The API key is invalid or expired.", false), nil, nil
			}
			taskID, taskValid := parsePublicID(input.TaskID)
			outputID, outputValid := parsePublicID(input.OutputID)
			if !taskValid || !outputValid {
				return toolFailure("INVALID_REQUEST", "The task or output ID is invalid.", false), nil, nil
			}
			outputs, err := gateway.ListOutputs(ctx, identity.authorization, taskID)
			if err != nil {
				return gatewayToolFailure(err), nil, nil
			}
			output, found := findOutput(outputs, taskID, outputID)
			if !found || output.Status != "AVAILABLE" {
				return toolFailure("OUTPUT_NOT_FOUND", "The output was not found.", false), nil, nil
			}
			if !supportedTextMediaType(output.MediaType) {
				return toolFailure("OUTPUT_MEDIA_TYPE_UNSUPPORTED", "Only text and JSON outputs can be read through MCP.", false), nil, nil
			}
			if output.SizeBytes < 0 || output.SizeBytes > maxTextBytes {
				return toolFailure("OUTPUT_TOO_LARGE", "The output exceeds the MCP text size limit.", false), nil, nil
			}
			stream, err := gateway.OpenOutput(ctx, identity.authorization, taskID, outputID)
			if err != nil {
				return gatewayToolFailure(err), nil, nil
			}
			if stream.Content == nil {
				return toolFailure("OUTPUT_UNAVAILABLE", "The output is temporarily unavailable.", true), nil, nil
			}
			defer stream.Content.Close()
			content, err := io.ReadAll(io.LimitReader(stream.Content, maxTextBytes+1))
			if err != nil {
				return toolFailure("OUTPUT_UNAVAILABLE", "The output is temporarily unavailable.", true), nil, nil
			}
			if int64(len(content)) > maxTextBytes {
				return toolFailure("OUTPUT_TOO_LARGE", "The output exceeds the MCP text size limit.", false), nil, nil
			}
			if int64(len(content)) != output.SizeBytes || !validSHA256(content, output.SHA256) {
				return toolFailure("OUTPUT_INTEGRITY_FAILED", "The output failed integrity verification.", false), nil, nil
			}
			if !utf8.Valid(content) {
				return toolFailure("OUTPUT_INVALID_TEXT", "The output is not valid UTF-8 text.", false), nil, nil
			}
			metadata := verifiedOutput{
				OutputID: input.OutputID, Name: output.Name, MediaType: output.MediaType,
				SHA256: strings.ToLower(output.SHA256), SizeBytes: strconv.FormatInt(output.SizeBytes, 10), Verified: true,
			}
			return &mcp.CallToolResult{
				Content:           []mcp.Content{&mcp.TextContent{Text: string(content)}},
				StructuredContent: metadata,
			}, nil, nil
		})
}

func createTaskSchema() map[string]any {
	return objectSchema(map[string]any{
		"idempotencyKey": stringLimits("A caller-defined idempotency key.", 1, 128, ""),
		"projectId":      stringLimits("The decimal ID of the Workforce Project that owns the task.", 1, 19, `^[1-9][0-9]{0,18}$`),
		"name":           stringLimits("An optional short task name.", 0, 120, ""),
		"modelSelection": objectSchema(map[string]any{
			"credentialId": stringLimits("Authorized model credential reference; never a secret.", 1, 19, `^[1-9][0-9]{0,18}$`),
			"model":        stringLimits("Model configured on the selected credential.", 1, 256, ""),
		}, []string{"credentialId", "model"}),
		"runtimeReleaseId":        stringLimits("Compatible published RuntimeRelease ID.", 1, 19, `^[1-9][0-9]{0,18}$`),
		"tags":                    map[string]any{"type": "array", "maxItems": 20, "items": stringLimits("Tag.", 1, 64, "")},
		"referenceAssetIds":       map[string]any{"type": "array", "maxItems": 50, "items": stringLimits("Reference asset ID.", 1, 19, `^[1-9][0-9]{0,18}$`)},
		"reviewEnabled":           map[string]any{"type": "boolean"},
		"deliveryPreference":      map[string]any{"type": "string", "enum": []string{"PROJECT_DEFAULT", "ASK_AFTER_GATES", "SAVE_ONLY"}},
		"executionTimeoutSeconds": map[string]any{"type": "integer", "minimum": 0, "maximum": 2147483647},
		"additionalCompletionCheck": objectSchema(map[string]any{
			"mode": map[string]any{"type": "string"}, "command": map[string]any{"type": "string"}, "script": map[string]any{"type": "string"}, "instructions": map[string]any{"type": "string"},
			"referenceAssetIds": map[string]any{"type": "array", "items": stringLimits("Reference asset ID.", 1, 19, `^[1-9][0-9]{0,18}$`)},
		}, []string{"mode"}),
		"instruction":           stringLimits("The task instruction.", 1, 20_000, ""),
		"externalCorrelationId": stringLimits("The caller's stable correlation identifier.", 1, 128, ""),
	}, []string{"idempotencyKey", "projectId", "instruction", "externalCorrelationId", "modelSelection", "runtimeReleaseId"})
}

func taskIDSchema() map[string]any {
	return objectSchema(map[string]any{
		"taskId": stringLimits("A decimal int64 task ID.", 1, 19, `^[1-9][0-9]{0,18}$`),
	}, []string{"taskId"})
}

func readOutputSchema() map[string]any {
	return objectSchema(map[string]any{
		"taskId":   stringLimits("A decimal int64 task ID.", 1, 19, `^[1-9][0-9]{0,18}$`),
		"outputId": stringLimits("A decimal int64 output ID.", 1, 19, `^[1-9][0-9]{0,18}$`),
	}, []string{"taskId", "outputId"})
}

func objectSchema(properties map[string]any, required []string) map[string]any {
	if properties == nil {
		properties = map[string]any{}
	}
	return map[string]any{"type": "object", "properties": properties, "required": required, "additionalProperties": false}
}

func stringLimits(description string, min, max int, pattern string) map[string]any {
	schema := map[string]any{"type": "string", "description": description, "minLength": min, "maxLength": max}
	if pattern != "" {
		schema["pattern"] = pattern
	}
	return schema
}

func parsePublicID(value string) (int64, bool) {
	if len(value) == 0 || len(value) > 19 || value[0] == '0' {
		return 0, false
	}
	for _, char := range value {
		if char < '0' || char > '9' {
			return 0, false
		}
	}
	id, err := strconv.ParseInt(value, 10, 64)
	return id, err == nil && id > 0
}

func findOutput(outputs []Output, taskID, outputID int64) (Output, bool) {
	_ = taskID
	for _, output := range outputs {
		if output.OutputID == outputID {
			return output, true
		}
	}
	return Output{}, false
}

func supportedTextMediaType(value string) bool {
	mediaType, _, err := mime.ParseMediaType(strings.TrimSpace(value))
	if err != nil {
		return false
	}
	return strings.HasPrefix(strings.ToLower(mediaType), "text/") || strings.EqualFold(mediaType, "application/json")
}

func validSHA256(content []byte, declared string) bool {
	declared = strings.TrimSpace(declared)
	if len(declared) != sha256.Size*2 {
		return false
	}
	decoded, err := hex.DecodeString(declared)
	if err != nil || len(decoded) != sha256.Size {
		return false
	}
	sum := sha256.Sum256(content)
	return bytes.Equal(decoded, sum[:])
}

func structuredSuccess(value any) *mcp.CallToolResult {
	raw, _ := json.Marshal(value)
	return &mcp.CallToolResult{
		Content:           []mcp.Content{&mcp.TextContent{Text: string(raw)}},
		StructuredContent: value,
	}
}

func gatewayToolFailure(err error) *mcp.CallToolResult {
	var gatewayError *GatewayError
	if errors.As(err, &gatewayError) {
		code, message, retryable := safeToolError(gatewayError.Code)
		return toolFailure(code, message, retryable)
	}
	return toolFailure("INTERNAL_ERROR", "The request could not be completed.", false)
}

func safeToolError(code string) (string, string, bool) {
	switch code {
	case "INVALID_REQUEST", "IDEMPOTENCY_KEY_REQUIRED":
		return code, "The request is invalid.", false
	case "PROJECT_NOT_FOUND":
		return code, "The project was not found.", false
	case "PROJECT_ARCHIVED":
		return code, "The project no longer accepts new tasks.", false
	case "PROJECT_SOURCE_NOT_READY":
		return code, "The project source is not ready.", false
	case "PROJECT_CONFIGURATION_INVALID":
		return code, "The project configuration must be updated before creating a task.", false
	case "UNAUTHORIZED", "APPLICATION_DISABLED", "TASK_RUNTIME_SELECTION_REQUIRED", "TASK_RUNTIME_SELECTION_UNAVAILABLE", "TASK_RUNTIME_SELECTION_INCOMPATIBLE":
		return code, publicGatewayMessage(code), false
	case "SELECTION_UNAVAILABLE":
		return code, publicGatewayMessage(code), true
	case "SCENARIO_UNAVAILABLE":
		return code, "The scenario is temporarily unavailable.", true
	case "IDEMPOTENCY_CONFLICT":
		return code, "The idempotency key was already used with a different request.", false
	case "TASK_NOT_FOUND":
		return code, "The task was not found.", false
	case "OUTPUT_NOT_FOUND":
		return code, "The output was not found.", false
	case "OUTPUT_UNAVAILABLE":
		return code, "The output is temporarily unavailable.", true
	case "OPEN_API_UNAVAILABLE":
		return code, "The Workforce API is temporarily unavailable.", true
	default:
		return "INTERNAL_ERROR", "The request could not be completed.", false
	}
}

func toolFailure(code, message string, retryable bool) *mcp.CallToolResult {
	view := toolErrorView{Error: toolError{Code: code, Message: message, Retryable: retryable}}
	return &mcp.CallToolResult{
		IsError:           true,
		Content:           []mcp.Content{&mcp.TextContent{Text: fmt.Sprintf("%s: %s", code, message)}},
		StructuredContent: view,
	}
}
