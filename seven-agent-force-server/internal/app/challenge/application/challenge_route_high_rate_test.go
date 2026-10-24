package application

import (
	"bytes"
	"encoding/json"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	challengedomain "github.com/CaixyPromise/seven-agent/force/internal/app/challenge/domain"
	challengefacade "github.com/CaixyPromise/seven-agent/force/internal/app/challenge/facade"
	challengehandler "github.com/CaixyPromise/seven-agent/force/internal/app/challenge/handler"
	apperrors "github.com/CaixyPromise/seven-agent/force/internal/common/errors"
	"github.com/CaixyPromise/seven-agent/force/internal/common/response"
	emailinfra "github.com/CaixyPromise/seven-agent/force/internal/infrastructure/email"
	"github.com/CaixyPromise/seven-agent/force/internal/shared/config"
	"github.com/cloudwego/hertz/pkg/app/server"
	"github.com/cloudwego/hertz/pkg/common/ut"
)

func TestChallengeHTTPRoutesThrottleStartAndEmailSendBeforeThirdTrigger(t *testing.T) {
	var sent int32
	service, _ := newTestChallengeServiceWithStoreAndEmailSender(
		t,
		config.ChallengeConfig{
			SessionTTLSeconds:               300,
			ProofTokenTTLMinSeconds:         60,
			ProofTokenTTLMaxSeconds:         300,
			EmailMaxAttempts:                3,
			EmailCooldownSeconds:            1,
			TriggerMaxAttempts:              2,
			ThrottleWindowSeconds:           300,
			ThrottleLockSeconds:             900,
			WebAuthnChallengeTimeoutSeconds: 60,
		},
		&testCompletionStore{},
		countingEmailSender{count: &sent},
	)
	engine := newChallengeHTTPTestEngine(service)

	postJSON(t, engine, "/internal/challenges/start", privilegedEmailStartRequest("user:1001", "idem-http-email-1", "config:1|reveal"), 0)
	postJSON(t, engine, "/internal/challenges/start", privilegedEmailStartRequest("user:1002", "idem-http-email-2", "config:2|reveal"), 0)
	result := postJSON(t, engine, "/internal/challenges/start", privilegedEmailStartRequest("user:1003", "idem-http-email-3", "config:3|reveal"), apperrors.CodeRateLimited)

	assertRouteChallengeThrottled(t, result)
	if got := atomic.LoadInt32(&sent); got != 2 {
		t.Fatalf("expected throttled HTTP start to block third email send, sent=%d", got)
	}
}

func TestChallengeHTTPRoutesThrottleRefreshBeforeThirdEmailSend(t *testing.T) {
	var sent int32
	service, repo := newTestChallengeServiceWithStoreAndEmailSender(
		t,
		config.ChallengeConfig{
			SessionTTLSeconds:               300,
			ProofTokenTTLMinSeconds:         60,
			ProofTokenTTLMaxSeconds:         300,
			EmailMaxAttempts:                3,
			EmailCooldownSeconds:            1,
			TriggerMaxAttempts:              2,
			ThrottleWindowSeconds:           300,
			ThrottleLockSeconds:             900,
			WebAuthnChallengeTimeoutSeconds: 60,
		},
		&testCompletionStore{},
		countingEmailSender{count: &sent},
	)
	engine := newChallengeHTTPTestEngine(service)

	start := postJSON(t, engine, "/internal/challenges/start", privilegedEmailStartRequest("user:1001", "idem-http-refresh-1", "config:1|reveal"), 0)
	challengeID := dataString(t, start, "challengeIdentifier")
	stepID := firstRouteStepOfType(t, start, challengedomain.ChallengeTypeEmailOneTimePassword)
	stored := repo.mustSession(t, challengeID)
	stored.SessionContext[emailOTPTriggerContextKey(&stored.Steps[0])] = time.Now().UTC().Add(-2 * time.Second).Format(time.RFC3339Nano)
	postJSON(t, engine, "/v1/challenges/"+challengeID+"/refresh", challengefacade.RefreshChallengeRequest{StepIdentifier: stepID}, 0)
	result := postJSON(t, engine, "/v1/challenges/"+challengeID+"/refresh", challengefacade.RefreshChallengeRequest{StepIdentifier: stepID}, apperrors.CodeRateLimited)

	assertRouteChallengeThrottled(t, result)
	if got := atomic.LoadInt32(&sent); got != 2 {
		t.Fatalf("expected throttled HTTP refresh to block third email send, sent=%d", got)
	}
}

func TestChallengeHTTPRoutesThrottleRepeatedRespondFailures(t *testing.T) {
	service, _ := newTestChallengeService(t, config.ChallengeConfig{
		SessionTTLSeconds:               300,
		ProofTokenTTLMinSeconds:         60,
		ProofTokenTTLMaxSeconds:         300,
		ImageMaxAttempts:                5,
		ImageCooldownSeconds:            1,
		TriggerMaxAttempts:              10,
		ThrottleMaxFailures:             2,
		ThrottleWindowSeconds:           300,
		ThrottleLockSeconds:             900,
		WebAuthnChallengeTimeoutSeconds: 60,
	})
	engine := newChallengeHTTPTestEngine(service)

	first := postJSON(t, engine, "/internal/challenges/start", loginCaptchaStartRequest("alice", "idem-http-respond-1", "10.40.0.1", "device-a"), 0)
	firstStep := firstRouteStepOfType(t, first, challengedomain.ChallengeTypeImageCaptcha)
	firstRespond := postJSON(t, engine, "/v1/challenges/"+dataString(t, first, "challengeIdentifier")+"/respond", challengefacade.RespondChallengeRequest{
		StepIdentifier: firstStep,
		Payload:        map[string]any{"captchaCode": "wrong"},
	}, apperrors.CodeParamsError)
	assertRouteNoErrorSemantics(t, firstRespond)

	second := postJSON(t, engine, "/internal/challenges/start", loginCaptchaStartRequest("alice", "idem-http-respond-2", "10.40.0.2", "device-b"), 0)
	secondStep := firstRouteStepOfType(t, second, challengedomain.ChallengeTypeImageCaptcha)
	secondRespond := postJSON(t, engine, "/v1/challenges/"+dataString(t, second, "challengeIdentifier")+"/respond", challengefacade.RespondChallengeRequest{
		StepIdentifier: secondStep,
		Payload:        map[string]any{"captchaCode": "wrong"},
	}, apperrors.CodeRateLimited)
	assertRouteNoErrorSemantics(t, secondRespond)
}

func newChallengeHTTPTestEngine(service *ChallengeService) *server.Hertz {
	engine := server.Default()
	internalHandler := challengehandler.NewInternalHandler(service)
	clientHandler := challengehandler.NewClientHandler(service)
	engine.POST("/internal/challenges/start", internalHandler.Start)
	engine.POST("/v1/challenges/:challengeIdentifier/respond", clientHandler.Respond)
	engine.POST("/v1/challenges/:challengeIdentifier/refresh", clientHandler.Refresh)
	return engine
}

func postJSON(t *testing.T, engine *server.Hertz, path string, body any, expectedCode int) response.Result {
	t.Helper()
	payload, err := json.Marshal(body)
	if err != nil {
		t.Fatalf("marshal request body: %v", err)
	}
	recorder := ut.PerformRequest(engine.Engine, "POST", path, &ut.Body{
		Body: bytes.NewReader(payload),
		Len:  len(payload),
	}, ut.Header{Key: "Content-Type", Value: "application/json"})
	expectedHTTP := 200
	if expectedCode == apperrors.CodeRateLimited {
		expectedHTTP = 429
	}
	if recorder.Code != expectedHTTP {
		t.Fatalf("unexpected HTTP status for %s: %d body=%s", path, recorder.Code, recorder.Body.String())
	}
	var result response.Result
	if err := json.Unmarshal(recorder.Body.Bytes(), &result); err != nil {
		t.Fatalf("unmarshal response for %s: %v body=%s", path, err, recorder.Body.String())
	}
	if result.Code != expectedCode {
		t.Fatalf("unexpected result code for %s: got=%d want=%d body=%s", path, result.Code, expectedCode, recorder.Body.String())
	}
	return result
}

func assertRouteChallengeThrottled(t *testing.T, result response.Result) {
	t.Helper()
	if result.Code != apperrors.CodeRateLimited {
		t.Fatalf("expected rate-limited code for throttled route, got %+v", result)
	}
	assertRouteNoErrorSemantics(t, result)
	data, ok := result.Data.(map[string]any)
	if !ok {
		t.Fatalf("expected throttle detail map, got %+v", result.Data)
	}
	if _, exists := data["errorCode"]; exists {
		t.Fatalf("throttle detail must omit errorCode, got %+v", data)
	}
}

func assertRouteNoErrorSemantics(t *testing.T, result response.Result) {
	t.Helper()
	raw, err := json.Marshal(result)
	if err != nil {
		t.Fatalf("marshal result: %v", err)
	}
	var body map[string]any
	if err := json.Unmarshal(raw, &body); err != nil {
		t.Fatalf("unmarshal result: %v", err)
	}
	if _, exists := body["errorType"]; exists {
		t.Fatalf("response must omit errorType: %+v", body)
	}
	if _, exists := body["errorCode"]; exists {
		t.Fatalf("response must omit errorCode: %+v", body)
	}
}

func firstRouteStepOfType(t *testing.T, result response.Result, challengeType challengedomain.ChallengeType) string {
	t.Helper()
	data, ok := result.Data.(map[string]any)
	if !ok {
		t.Fatalf("expected response data map, got %+v", result.Data)
	}
	rawSteps, ok := data["steps"].([]any)
	if !ok {
		t.Fatalf("expected steps array, got %+v", data["steps"])
	}
	for _, raw := range rawSteps {
		step, ok := raw.(map[string]any)
		if !ok {
			continue
		}
		if step["challengeType"] == string(challengeType) {
			stepID, _ := step["stepIdentifier"].(string)
			if strings.TrimSpace(stepID) == "" {
				t.Fatalf("expected stepIdentifier in %+v", step)
			}
			return stepID
		}
	}
	t.Fatalf("expected step type %s in %+v", challengeType, rawSteps)
	return ""
}

func dataString(t *testing.T, result response.Result, key string) string {
	t.Helper()
	data, ok := result.Data.(map[string]any)
	if !ok {
		t.Fatalf("expected response data map, got %+v", result.Data)
	}
	value, _ := data[key].(string)
	if strings.TrimSpace(value) == "" {
		t.Fatalf("expected non-empty data.%s in %+v", key, data)
	}
	return value
}

var _ emailinfra.Sender = countingEmailSender{}
