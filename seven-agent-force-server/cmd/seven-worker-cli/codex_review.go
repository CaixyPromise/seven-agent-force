package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"
)

// The Reviewer contract is deliberately separate from attemptSpec and the
// Agent Codex command. A ReviewRun is a machine-only consumer of a frozen
// result; it must not inherit Agent Attempt flags, session state, or verdict
// selectors.
const (
	codexReviewHarnessType        = "codex"
	codexReviewAdapter            = "codex-app-server-v1"
	codexReviewReportSchema       = "review-report/v1"
	codexReviewStructuredContract = "independent-reviewer/structured-output/v1"
	codexReviewProviderProtocol   = "OPENAI"
	codexReviewMaxInputBytes      = 256 << 10
	codexReviewMaxModelBytes      = 256
	codexReviewProviderIdentityID = "task.model-credential"
)

var (
	errCodexReviewArgs             = errors.New("CODEX_REVIEW_ARGS_INVALID")
	errCodexReviewInput            = errors.New("CODEX_REVIEW_INPUT_INVALID")
	errCodexReviewProviderIdentity = errors.New("CODEX_REVIEW_PROVIDER_IDENTITY_INVALID")
	errCodexReviewContract         = errors.New("CODEX_REVIEW_CONTRACT_INVALID")
)

// codexReviewProviderIdentity is intentionally a closed, non-secret shape.
// In particular, it has no API key, token, endpoint, or free-form auth field.
// The ReviewRun runtime owns the provider relay; this CLI receives only the
// frozen identity that the relay is authorized to serve.
type codexReviewProviderIdentity struct {
	CredentialID       string `json:"credentialId"`
	CredentialVersion  string `json:"credentialVersion"`
	Protocol           string `json:"protocol"`
	Model              string `json:"model"`
	StructuredContract string `json:"structuredContract"`
}

type codexReviewInput struct {
	SchemaVersion  int                          `json:"schemaVersion"`
	ReviewRunID    string                       `json:"reviewRunId"`
	TaskID         string                       `json:"taskId"`
	ResultCommit   string                       `json:"resultCommit"`
	ResultTree     string                       `json:"resultTree"`
	ReviewContract json.RawMessage              `json:"reviewContract"`
	Requirements   json.RawMessage              `json:"requirements"`
	Provider       *codexReviewProviderIdentity `json:"provider"`
}

// codexReviewContract is the immutable capability declaration consumed by
// scheduling/alignment code. It is mirrored in the image manifest, while the
// CLI keeps its own fail-closed copy so a malformed input cannot silently
// select another harness or report schema.
type codexReviewContract struct {
	HarnessType                 string
	Adapter                     string
	ReportSchema                string
	ProviderIdentitySource      string
	VerdictSource               string
	TaskVerdictSelectionAllowed bool
	EnvVerdictSelectionAllowed  bool
}

func frozenCodexReviewContract() codexReviewContract {
	return codexReviewContract{
		HarnessType:                 codexReviewHarnessType,
		Adapter:                     codexReviewAdapter,
		ReportSchema:                codexReviewReportSchema,
		ProviderIdentitySource:      codexReviewProviderIdentityID,
		VerdictSource:               "codex-structured-output",
		TaskVerdictSelectionAllowed: false,
		EnvVerdictSelectionAllowed:  false,
	}
}

// validateCodexReviewArgs rejects all command-line parameters. Review input
// is mounted by the ReviewRun runtime, and no caller may turn a task argument
// into a verdict, model, provider, or path override.
func validateCodexReviewArgs(args []string) error {
	if len(args) != 0 {
		return errCodexReviewArgs
	}
	return nil
}

func decodeCodexReviewInput(body []byte) (codexReviewInput, error) {
	if len(body) == 0 || len(body) > codexReviewMaxInputBytes {
		return codexReviewInput{}, errCodexReviewInput
	}
	decoder := json.NewDecoder(bytes.NewReader(body))
	decoder.DisallowUnknownFields()
	var input codexReviewInput
	if err := decoder.Decode(&input); err != nil {
		return codexReviewInput{}, errCodexReviewInput
	}
	var trailing any
	if err := decoder.Decode(&trailing); err != io.EOF {
		return codexReviewInput{}, errCodexReviewInput
	}
	if err := validateCodexReviewInput(input); err != nil {
		return codexReviewInput{}, err
	}
	return input, nil
}

func validateCodexReviewInput(input codexReviewInput) error {
	contract := frozenCodexReviewContract()
	if contract.HarnessType != codexReviewHarnessType || contract.Adapter != codexReviewAdapter || contract.ReportSchema != codexReviewReportSchema || contract.TaskVerdictSelectionAllowed || contract.EnvVerdictSelectionAllowed {
		return errCodexReviewContract
	}
	if input.SchemaVersion != 1 || !positiveDecimalID(input.ReviewRunID) || !positiveDecimalID(input.TaskID) || !validGitObjectID(input.ResultCommit) || !validGitObjectID(input.ResultTree) {
		return errCodexReviewInput
	}
	if !validJSONDocument(input.ReviewContract) || !validJSONDocument(input.Requirements) {
		return errCodexReviewInput
	}
	if input.Provider == nil {
		return errCodexReviewProviderIdentity
	}
	return validateCodexReviewProviderIdentity(*input.Provider)
}

func validateCodexReviewProviderIdentity(identity codexReviewProviderIdentity) error {
	if !positiveDecimalID(identity.CredentialID) || !positiveDecimalID(identity.CredentialVersion) || identity.Protocol != codexReviewProviderProtocol || identity.StructuredContract != codexReviewStructuredContract || !validReviewModel(identity.Model) {
		return errCodexReviewProviderIdentity
	}
	return nil
}

func validReviewModel(value string) bool {
	if value == "" || len(value) > codexReviewMaxModelBytes || strings.TrimSpace(value) != value {
		return false
	}
	for _, character := range value {
		if character < 0x20 || character == 0x7f || character == '"' || character == '\\' {
			return false
		}
	}
	return true
}

func validJSONDocument(raw json.RawMessage) bool {
	if len(raw) == 0 {
		return false
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	var value any
	if decoder.Decode(&value) != nil {
		return false
	}
	// Review contracts and requirements are structured documents. Keep the
	// image boundary fail-closed as well as the outer ReviewRun runtime so a
	// direct mounted-input invocation cannot turn an array or scalar into a
	// seemingly valid Reviewer input.
	if object, ok := value.(map[string]any); !ok || object == nil {
		return false
	}
	var trailing any
	return decoder.Decode(&trailing) == io.EOF
}

func positiveDecimalID(value string) bool {
	parsed, err := strconv.ParseInt(value, 10, 64)
	return err == nil && parsed > 0 && strconv.FormatInt(parsed, 10) == value
}

func validGitObjectID(value string) bool {
	return (len(value) == 40 || len(value) == 64) && validLowerHex(value, len(value))
}

func validLowerHex(value string, expectedLength int) bool {
	if len(value) != expectedLength {
		return false
	}
	for _, character := range value {
		if !((character >= '0' && character <= '9') || (character >= 'a' && character <= 'f')) {
			return false
		}
	}
	return true
}

// codexReviewOutputSchema describes the fixed report contract. The generic
// review adapter prepares the provider envelope below; the Server canonicalizes
// the resulting report independently. Neither schema grants result trust.
const codexReviewOutputSchema = `{"$schema":"http://json-schema.org/draft-07/schema#","type":"object","additionalProperties":false,"required":["schemaVersion","verdict","summary"],"properties":{"schemaVersion":{"const":"review-report/v1"},"verdict":{"enum":["PASS","REVISE","REJECT"]},"score":{"type":"integer","minimum":0,"maximum":100},"summary":{"type":"string","minLength":1,"maxLength":2048},"findings":{"type":"array","maxItems":32,"items":{"type":"object","additionalProperties":false,"required":["id","severity","category","title","message","evidence"],"properties":{"id":{"type":"string","minLength":1,"maxLength":128},"severity":{"enum":["INFO","WARNING","BLOCKING"]},"category":{"enum":["REQUIREMENT","CORRECTNESS","SECURITY","TESTING","MAINTAINABILITY"]},"title":{"type":"string","minLength":1,"maxLength":2048},"message":{"type":"string","minLength":1,"maxLength":2048},"evidence":{"type":"object","additionalProperties":false,"required":["path","line"],"properties":{"path":{"type":"string","minLength":1},"line":{"type":"integer","minimum":1}}},"suggestedDirection":{"type":"string","maxLength":2048}}}},"reviewTests":{"type":"array","maxItems":64,"items":{"type":"object","additionalProperties":false,"required":["command","exitCode","status","summary"],"properties":{"command":{"type":"string","minLength":1,"maxLength":512},"exitCode":{"type":"integer","minimum":-1,"maximum":255},"status":{"enum":["PASSED","FAILED","SKIPPED","UNABLE_TO_REPRODUCE"]},"summary":{"type":"string","minLength":1,"maxLength":2048}}}},"limitations":{"type":"array","maxItems":64,"items":{"type":"object","additionalProperties":false,"required":["code","summary"],"properties":{"code":{"enum":["UNABLE_TO_REPRODUCE","TIME_BOUNDED","NO_PROVIDER_RESPONSE"]},"summary":{"type":"string","minLength":1,"maxLength":2048}}}}}}`

// local-api follows the OpenAI Responses JSON-schema surface, but its
// structured-output gateway intentionally accepts only a bounded subset of
// JSON Schema keywords and nesting. Keep the full review-report contract
// above as the Server-side canonicalization contract, and send this small
// provider-compatible envelope to Codex. The Server still validates every
// optional finding, evidence path, enum, limit, and contradiction after the
// report is returned; this envelope is not a trust boundary or a verdict
// selector.
const codexReviewProviderOutputSchema = `{"type":"object","additionalProperties":false,"required":["schemaVersion","verdict","score","summary","findings","reviewTests","limitations"],"properties":{"schemaVersion":{"type":"string"},"verdict":{"type":"string"},"score":{"type":"integer"},"summary":{"type":"string"},"findings":{"type":"array","items":{"type":"object","additionalProperties":false,"required":["id","severity","category","title","message","evidence"],"properties":{"id":{"type":"string"},"severity":{"type":"string"},"category":{"type":"string"},"title":{"type":"string"},"message":{"type":"string"},"evidence":{"type":"object","additionalProperties":false,"required":["path","line"],"properties":{"path":{"type":"string"},"line":{"type":"integer"}}}}}},"reviewTests":{"type":"array","items":{"type":"object","additionalProperties":false,"required":["command","exitCode","status","summary"],"properties":{"command":{"type":"string"},"exitCode":{"type":"integer"},"status":{"type":"string"},"summary":{"type":"string"}}}},"limitations":{"type":"array","items":{"type":"object","additionalProperties":false,"required":["code","summary"],"properties":{"code":{"type":"string"},"summary":{"type":"string"}}}}}}`

// startCodexReviewGateway terminates the Reviewer's HTTP provider boundary at
// loopback. The Reviewer process can speak ordinary HTTP to Codex on 127.0.0.1,
// while the only egress from this container is the Unix socket mounted at
// /provider. The bridge injects only the short-lived, non-user provider
// contract expected by the paired relay; it never forwards arbitrary headers.
func startCodexReviewGateway(input codexReviewInput) (func(), error) {
	if input.Provider == nil {
		return nil, errCodexReviewProviderIdentity
	}
	authority, err := os.ReadFile("/provider/review-authority")
	if err != nil || !validLowerHex(strings.TrimSpace(string(authority)), 64) {
		return nil, errors.New("review provider authority is unavailable")
	}
	authorityValue := strings.TrimSpace(string(authority))
	listener, err := net.Listen("tcp", "127.0.0.1:8787")
	if err != nil {
		return nil, fmt.Errorf("start Review Gateway bridge: %w", err)
	}
	transport := &http.Transport{
		DialContext: func(ctx context.Context, _, _ string) (net.Conn, error) {
			return (&net.Dialer{Timeout: 5 * time.Second}).DialContext(ctx, "unix", "/provider/review-gateway.sock")
		},
		MaxResponseHeaderBytes: 1 << 20,
	}
	client := &http.Client{Transport: transport, Timeout: 0}
	handler := http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		if request.Method != http.MethodPost || request.URL.RawQuery != "" || (request.URL.Path != "/v1/responses" && request.URL.Path != "/v1/responses/compact") {
			writeCodexReviewGatewayError(response, http.StatusNotFound, "NOT_FOUND")
			return
		}
		body, readErr := io.ReadAll(io.LimitReader(request.Body, 128<<20+1))
		if readErr != nil || len(body) > 128<<20 {
			writeCodexReviewGatewayError(response, http.StatusRequestEntityTooLarge, "REQUEST_TOO_LARGE")
			return
		}
		upstream, requestErr := http.NewRequestWithContext(request.Context(), http.MethodPost, "http://review-gateway"+request.URL.RequestURI(), bytes.NewReader(body))
		if requestErr != nil {
			writeCodexReviewGatewayError(response, http.StatusBadGateway, "GATEWAY_REQUEST_FAILED")
			return
		}
		if contentType := request.Header.Get("Content-Type"); contentType != "" {
			upstream.Header.Set("Content-Type", contentType)
		}
		if accept := request.Header.Get("Accept"); accept != "" {
			upstream.Header.Set("Accept", accept)
		}
		upstream.Header.Set("X-Seven-Review-Authority", authorityValue)
		upstream.Header.Set("X-Seven-Review-Credential-Id", input.Provider.CredentialID)
		upstream.Header.Set("X-Seven-Review-Credential-Version", input.Provider.CredentialVersion)
		upstream.Header.Set("X-Seven-Review-Protocol", input.Provider.Protocol)
		upstream.Header.Set("X-Seven-Review-Contract", input.Provider.StructuredContract)
		upstreamResponse, doErr := client.Do(upstream)
		if doErr != nil {
			writeCodexReviewGatewayError(response, http.StatusBadGateway, "GATEWAY_UNAVAILABLE")
			return
		}
		defer upstreamResponse.Body.Close()
		for _, header := range []string{"Content-Type", "Cache-Control", "X-Request-ID", "Retry-After"} {
			if value := upstreamResponse.Header.Get(header); value != "" {
				response.Header().Set(header, value)
			}
		}
		response.WriteHeader(upstreamResponse.StatusCode)
		_, _ = io.Copy(response, io.LimitReader(upstreamResponse.Body, 128<<20))
	})
	server := &http.Server{Handler: handler, ReadHeaderTimeout: 5 * time.Second, MaxHeaderBytes: 1 << 20}
	go func() { _ = server.Serve(listener) }()
	return func() {
		shutdownContext, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = server.Shutdown(shutdownContext)
		transport.CloseIdleConnections()
		_ = listener.Close()
	}, nil
}

func writeCodexReviewGatewayError(response http.ResponseWriter, status int, code string) {
	response.Header().Set("Content-Type", "application/json")
	response.Header().Set("Cache-Control", "no-store")
	response.WriteHeader(status)
	_, _ = io.WriteString(response, `{"error":{"type":"seven_review_gateway_error","code":"`+code+`"}}`)
}
