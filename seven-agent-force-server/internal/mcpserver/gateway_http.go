package mcpserver

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

const maxGatewayResponseBytes = int64(1 << 20)

type gatewayOperation uint8

const (
	operationCapabilities gatewayOperation = iota
	operationModelOptions
	operationRuntimeOptions
	operationCreateTask
	operationGetTask
	operationListOutputs
	operationOpenOutput
)

type HTTPGatewayConfig struct {
	BaseURL string
	Timeout time.Duration
}

type HTTPGateway struct {
	baseURL *url.URL
	client  *http.Client
}

func NewHTTPGateway(config HTTPGatewayConfig) (*HTTPGateway, error) {
	base, err := url.Parse(strings.TrimSpace(config.BaseURL))
	if err != nil || base.Scheme == "" || base.Host == "" || base.User != nil || base.RawQuery != "" || base.Fragment != "" {
		return nil, errors.New("open API base URL must be an absolute HTTP(S) URL without credentials, query, or fragment")
	}
	if base.Scheme != "http" && base.Scheme != "https" {
		return nil, errors.New("open API base URL scheme must be http or https")
	}
	base.Path = strings.TrimRight(base.Path, "/") + "/"
	timeout := config.Timeout
	if timeout <= 0 {
		timeout = 15 * time.Second
	}
	transport := &http.Transport{
		Proxy:                 nil,
		DialContext:           (&net.Dialer{Timeout: 5 * time.Second, KeepAlive: 30 * time.Second}).DialContext,
		ForceAttemptHTTP2:     true,
		MaxIdleConns:          64,
		MaxIdleConnsPerHost:   32,
		IdleConnTimeout:       90 * time.Second,
		TLSHandshakeTimeout:   5 * time.Second,
		ResponseHeaderTimeout: timeout,
	}
	return &HTTPGateway{
		baseURL: base,
		client: &http.Client{
			Timeout:       timeout,
			Transport:     transport,
			CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
		},
	}, nil
}

type frameworkEnvelope struct {
	Code    int             `json:"code"`
	Data    json.RawMessage `json:"data"`
	Message string          `json:"message"`
}

func (g *HTTPGateway) GetCapabilities(ctx context.Context, authorization string) (Capabilities, error) {
	var result Capabilities
	if err := g.doJSON(ctx, operationCapabilities, http.MethodGet, "open/workforce/v1/capabilities", authorization, "", nil, &result); err != nil {
		return Capabilities{}, err
	}
	if result.Schema != "seven.workforce.open-capabilities/v2" || result.ApplicationID <= 0 || !result.TaskSubmission.RequiresProjectID || !result.TaskSubmission.SupportsOutputs {
		return Capabilities{}, &GatewayError{Code: "INVALID_OPEN_RESPONSE", Message: "The Workforce API returned an invalid response.", HTTPStatus: http.StatusBadGateway}
	}
	return result, nil
}

func (g *HTTPGateway) CreateTask(ctx context.Context, authorization string, request CreateTaskRequest) (Task, error) {
	var result Task
	body := request
	err := g.doJSON(ctx, operationCreateTask, http.MethodPost, "open/workforce/v1/projects/"+strconv.FormatInt(request.ProjectID, 10)+"/tasks", authorization, request.IdempotencyKey, body, &result)
	if err == nil && (request.ProjectID <= 0 || result.TaskID <= 0 || result.TaskTurnID <= 0 || result.ProjectID != request.ProjectID) {
		err = &GatewayError{Code: "INVALID_OPEN_RESPONSE", Message: "The Workforce API returned an invalid response.", HTTPStatus: http.StatusBadGateway}
	}
	return result, err
}

func (g *HTTPGateway) GetTask(ctx context.Context, authorization string, taskID int64) (Task, error) {
	var result Task
	err := g.doJSON(ctx, operationGetTask, http.MethodGet, "open/workforce/v1/tasks/"+strconv.FormatInt(taskID, 10), authorization, "", nil, &result)
	if err == nil && result.TaskID != taskID {
		err = &GatewayError{Code: "INVALID_OPEN_RESPONSE", Message: "The Workforce API returned an invalid response.", HTTPStatus: http.StatusBadGateway}
	}
	return result, err
}

func (g *HTTPGateway) ListOutputs(ctx context.Context, authorization string, taskID int64) ([]Output, error) {
	var result struct {
		Schema  string   `json:"schema"`
		TaskID  int64    `json:"taskId,string"`
		Outputs []Output `json:"outputs"`
	}
	err := g.doJSON(ctx, operationListOutputs, http.MethodGet, "open/workforce/v1/tasks/"+strconv.FormatInt(taskID, 10)+"/outputs", authorization, "", nil, &result)
	if err != nil {
		return nil, err
	}
	if result.Schema != "seven.workforce.open-task-outputs/v1" || result.TaskID != taskID {
		return nil, &GatewayError{Code: "INVALID_OPEN_RESPONSE", Message: "The Workforce API returned an invalid response.", HTTPStatus: http.StatusBadGateway}
	}
	if result.Outputs == nil {
		result.Outputs = make([]Output, 0)
	}
	if len(result.Outputs) > 64 {
		return nil, &GatewayError{Code: "INVALID_OPEN_RESPONSE", Message: "The Workforce API returned an invalid response.", HTTPStatus: http.StatusBadGateway}
	}
	for _, output := range result.Outputs {
		if output.OutputID <= 0 || output.SizeBytes < 0 {
			return nil, &GatewayError{Code: "INVALID_OPEN_RESPONSE", Message: "The Workforce API returned an invalid response.", HTTPStatus: http.StatusBadGateway}
		}
	}
	return result.Outputs, nil
}

func (g *HTTPGateway) OpenOutput(ctx context.Context, authorization string, taskID, outputID int64) (OutputStream, error) {
	endpoint := "open/workforce/v1/tasks/" + strconv.FormatInt(taskID, 10) + "/outputs/" + strconv.FormatInt(outputID, 10) + "/download"
	req, err := g.newRequest(ctx, http.MethodGet, endpoint, authorization, "", nil)
	if err != nil {
		return OutputStream{}, err
	}
	resp, err := g.client.Do(req)
	if err != nil {
		return OutputStream{}, &GatewayError{Code: "OUTPUT_UNAVAILABLE", Message: "The output is temporarily unavailable.", Retryable: true, HTTPStatus: http.StatusServiceUnavailable}
	}
	if resp.StatusCode != http.StatusOK {
		defer resp.Body.Close()
		return OutputStream{}, decodeGatewayError(resp, operationOpenOutput)
	}
	return OutputStream{Content: resp.Body}, nil
}

func (g *HTTPGateway) doJSON(ctx context.Context, operation gatewayOperation, method, endpoint, authorization, idempotencyKey string, body, target any) error {
	req, err := g.newRequest(ctx, method, endpoint, authorization, idempotencyKey, body)
	if err != nil {
		return err
	}
	resp, err := g.client.Do(req)
	if err != nil {
		return &GatewayError{Code: "OPEN_API_UNAVAILABLE", Message: "The Workforce API is temporarily unavailable.", Retryable: true, HTTPStatus: http.StatusServiceUnavailable}
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return decodeGatewayError(resp, operation)
	}
	limited := io.LimitReader(resp.Body, maxGatewayResponseBytes+1)
	raw, err := io.ReadAll(limited)
	if err != nil || int64(len(raw)) > maxGatewayResponseBytes {
		return &GatewayError{Code: "INVALID_OPEN_RESPONSE", Message: "The Workforce API returned an invalid response.", HTTPStatus: http.StatusBadGateway}
	}
	var envelope frameworkEnvelope
	if err := json.Unmarshal(raw, &envelope); err != nil || envelope.Code != 0 || len(envelope.Data) == 0 {
		return &GatewayError{Code: "INVALID_OPEN_RESPONSE", Message: "The Workforce API returned an invalid response.", HTTPStatus: http.StatusBadGateway}
	}
	if err := json.Unmarshal(envelope.Data, target); err != nil {
		return &GatewayError{Code: "INVALID_OPEN_RESPONSE", Message: "The Workforce API returned an invalid response.", HTTPStatus: http.StatusBadGateway}
	}
	return nil
}

func (g *HTTPGateway) newRequest(ctx context.Context, method, endpoint, authorization, idempotencyKey string, body any) (*http.Request, error) {
	var reader io.Reader
	if body != nil {
		raw, err := json.Marshal(body)
		if err != nil {
			return nil, fmt.Errorf("encode Open API request: %w", err)
		}
		reader = bytes.NewReader(raw)
	}
	ref, err := url.Parse(endpoint)
	if err != nil {
		return nil, fmt.Errorf("parse Open API endpoint: %w", err)
	}
	resolved := g.baseURL.ResolveReference(ref)
	req, err := http.NewRequestWithContext(ctx, method, resolved.String(), reader)
	if err != nil {
		return nil, fmt.Errorf("create Open API request: %w", err)
	}
	req.Header.Set("Authorization", authorization)
	req.Header.Set("Accept", "application/json")
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if idempotencyKey != "" {
		req.Header.Set("Idempotency-Key", idempotencyKey)
	}
	return req, nil
}

func decodeGatewayError(resp *http.Response, operation gatewayOperation) error {
	raw, _ := io.ReadAll(io.LimitReader(resp.Body, 64<<10))
	var envelope struct {
		Code int `json:"code"`
		Data struct {
			ReasonCode string `json:"reasonCode"`
		} `json:"data"`
	}
	_ = json.Unmarshal(raw, &envelope)
	code := allowedGatewayCode(operation, strings.TrimSpace(envelope.Data.ReasonCode), resp.StatusCode)
	message := publicGatewayMessage(code)
	retryable := code == "OPEN_API_UNAVAILABLE" || code == "OUTPUT_UNAVAILABLE" || code == "SELECTION_UNAVAILABLE"
	return &GatewayError{Code: code, Message: message, Retryable: retryable, HTTPStatus: resp.StatusCode}
}

func allowedGatewayCode(operation gatewayOperation, upstream string, status int) string {
	if status == http.StatusUnauthorized {
		if upstream == "APPLICATION_DISABLED" {
			return upstream
		}
		return "UNAUTHORIZED"
	}
	allowed := map[gatewayOperation]map[string]struct{}{
		operationModelOptions:   {"SELECTION_UNAVAILABLE": {}, "INVALID_REQUEST": {}, "PROJECT_NOT_FOUND": {}, "PROJECT_ARCHIVED": {}, "PROJECT_SOURCE_NOT_READY": {}, "PROJECT_CONFIGURATION_INVALID": {}},
		operationRuntimeOptions: {"SELECTION_UNAVAILABLE": {}, "TASK_RUNTIME_SELECTION_UNAVAILABLE": {}, "TASK_RUNTIME_SELECTION_INCOMPATIBLE": {}, "TASK_RUNTIME_SELECTION_REQUIRED": {}, "INVALID_REQUEST": {}, "PROJECT_NOT_FOUND": {}, "PROJECT_ARCHIVED": {}, "PROJECT_SOURCE_NOT_READY": {}, "PROJECT_CONFIGURATION_INVALID": {}},
		operationCapabilities: {
			"UNAUTHORIZED": {}, "APPLICATION_DISABLED": {},
		},
		operationCreateTask: {
			"INVALID_REQUEST": {}, "SCENARIO_UNAVAILABLE": {}, "IDEMPOTENCY_KEY_REQUIRED": {}, "IDEMPOTENCY_CONFLICT": {},
			"PROJECT_NOT_FOUND": {}, "PROJECT_ARCHIVED": {}, "PROJECT_SOURCE_NOT_READY": {}, "PROJECT_CONFIGURATION_INVALID": {},
			"TASK_RUNTIME_SELECTION_UNAVAILABLE": {}, "TASK_RUNTIME_SELECTION_INCOMPATIBLE": {}, "TASK_RUNTIME_SELECTION_REQUIRED": {},
		},
		operationGetTask: {
			"TASK_NOT_FOUND": {},
		},
		operationListOutputs: {
			"OUTPUT_NOT_FOUND": {}, "OUTPUT_UNAVAILABLE": {},
		},
		operationOpenOutput: {
			"OUTPUT_NOT_FOUND": {}, "OUTPUT_UNAVAILABLE": {},
		},
	}
	if _, ok := allowed[operation][upstream]; ok {
		return upstream
	}
	if status >= 500 || status == http.StatusTooManyRequests || status == http.StatusRequestTimeout {
		return "OPEN_API_UNAVAILABLE"
	}
	if operation == operationCapabilities && status == http.StatusUnauthorized {
		return "UNAUTHORIZED"
	}
	return "INTERNAL_ERROR"
}

func publicGatewayMessage(code string) string {
	switch code {
	case "UNAUTHORIZED":
		return "The API key is invalid or expired."
	case "APPLICATION_DISABLED":
		return "The integration application is disabled."
	case "TASK_NOT_FOUND":
		return "The task was not found."
	case "OUTPUT_NOT_FOUND", "NOT_FOUND":
		return "The output was not found."
	case "OUTPUT_UNAVAILABLE":
		return "The output is temporarily unavailable."
	case "OPEN_API_UNAVAILABLE":
		return "The Workforce API is temporarily unavailable."
	case "IDEMPOTENCY_CONFLICT":
		return "The idempotency key was already used with a different request."
	case "PROJECT_NOT_FOUND":
		return "The project was not found."
	case "PROJECT_ARCHIVED":
		return "The project no longer accepts new tasks."
	case "PROJECT_SOURCE_NOT_READY":
		return "The project source is not ready."
	case "PROJECT_CONFIGURATION_INVALID":
		return "The project configuration must be updated before creating a task."
	case "SELECTION_UNAVAILABLE":
		return "Task selection queries are temporarily unavailable."
	case "TASK_RUNTIME_SELECTION_REQUIRED":
		return "A RuntimeRelease selection is required."
	case "TASK_RUNTIME_SELECTION_UNAVAILABLE":
		return "The selected model credential or RuntimeRelease is unavailable."
	case "TASK_RUNTIME_SELECTION_INCOMPATIBLE":
		return "The selected RuntimeRelease is incompatible with the task."
	case "SCENARIO_UNAVAILABLE":
		return "The scenario is temporarily unavailable."
	case "INVALID_REQUEST", "IDEMPOTENCY_KEY_REQUIRED":
		return "The request is invalid."
	default:
		return "The request could not be completed."
	}
}

func (g *HTTPGateway) ListModelOptions(ctx context.Context, key string, projectID int64) (ModelOptions, error) {
	var result ModelOptions
	err := g.doJSON(ctx, operationModelOptions, http.MethodGet, "open/workforce/v1/projects/"+strconv.FormatInt(projectID, 10)+"/model-options", key, "", nil, &result)
	if err == nil && (result.Schema != "seven.workforce.open-model-options/v1" || result.ProjectID != projectID) {
		err = &GatewayError{Code: "INVALID_OPEN_RESPONSE", Message: "The Workforce API returned an invalid response.", HTTPStatus: http.StatusBadGateway}
	}
	for _, item := range result.Items {
		if item.CredentialID <= 0 || item.Model == "" {
			return ModelOptions{}, &GatewayError{Code: "INVALID_OPEN_RESPONSE", Message: "The Workforce API returned an invalid response.", HTTPStatus: http.StatusBadGateway}
		}
	}
	if result.Items == nil {
		result.Items = make([]ModelOption, 0)
	}
	return result, err
}
func (g *HTTPGateway) ListRuntimeOptions(ctx context.Context, key string, projectID, credentialID int64) (RuntimeOptions, error) {
	var result RuntimeOptions
	err := g.doJSON(ctx, operationRuntimeOptions, http.MethodGet, "open/workforce/v1/projects/"+strconv.FormatInt(projectID, 10)+"/runtime-options?modelCredentialId="+strconv.FormatInt(credentialID, 10), key, "", nil, &result)
	if err == nil && (result.Schema != "seven.workforce.open-runtime-options/v1" || result.ProjectVersionID <= 0) {
		err = &GatewayError{Code: "INVALID_OPEN_RESPONSE", Message: "The Workforce API returned an invalid response.", HTTPStatus: http.StatusBadGateway}
	}
	for _, item := range result.Items {
		if item.RuntimeReleaseID <= 0 {
			return RuntimeOptions{}, &GatewayError{Code: "INVALID_OPEN_RESPONSE", Message: "The Workforce API returned an invalid response.", HTTPStatus: http.StatusBadGateway}
		}
	}
	if result.Items == nil {
		result.Items = make([]RuntimeOption, 0)
	}
	return result, err
}
