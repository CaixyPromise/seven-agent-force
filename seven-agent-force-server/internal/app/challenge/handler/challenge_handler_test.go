package handler

import (
	"bytes"
	"context"
	"encoding/json"
	"testing"
	"time"

	challengefacade "github.com/CaixyPromise/seven-agent/force/internal/app/challenge/facade"
	"github.com/CaixyPromise/seven-agent/force/internal/common/response"
	"github.com/cloudwego/hertz/pkg/app/server"
	"github.com/cloudwego/hertz/pkg/common/ut"
)

func TestChallengeHandlersExposeStartGetRespondRefreshRoutes(t *testing.T) {
	internal := &fakeInternalFacade{
		response: &challengefacade.StartChallengeResponse{
			ChallengeIdentifier: "challenge-1",
			ChallengeState:      "PENDING",
		},
	}
	client := &fakeClientFacade{
		getResponse: &challengefacade.StartChallengeResponse{
			ChallengeIdentifier: "challenge-1",
			ChallengeState:      "PENDING",
		},
		respondResponse: &challengefacade.RespondChallengeResponse{
			ChallengeState: "PASSED",
			ProofToken:     "proof-token-1",
		},
		refreshResponse: &challengefacade.StartChallengeResponse{
			ChallengeIdentifier: "challenge-1",
			ChallengeState:      "PENDING",
		},
	}

	engine := server.Default()
	internalHandler := NewInternalHandler(internal)
	clientHandler := NewClientHandler(client)
	engine.POST("/internal/challenges/start", internalHandler.Start)
	engine.GET("/v1/challenges/:challengeIdentifier", clientHandler.Get)
	engine.POST("/v1/challenges/:challengeIdentifier/respond", clientHandler.Respond)
	engine.POST("/v1/challenges/:challengeIdentifier/refresh", clientHandler.Refresh)

	startBody := marshalBody(t, challengefacade.StartChallengeRequest{
		IssuingServiceName:   "system-admin",
		AudienceServiceNames: []string{"system-admin"},
		BusinessAction:       "LOGIN",
		FlowNonce:            "flow-1",
		IdempotencyKey:       "idem-1",
	})
	startResp := ut.PerformRequest(engine.Engine, "POST", "/internal/challenges/start", &ut.Body{
		Body: bytes.NewReader(startBody),
		Len:  len(startBody),
	}, ut.Header{Key: "Content-Type", Value: "application/json"})
	assertResultCode(t, startResp, 200)
	if internal.request.IdempotencyKey != "idem-1" {
		t.Fatalf("unexpected start request captured: %+v", internal.request)
	}

	getResp := ut.PerformRequest(engine.Engine, "GET", "/v1/challenges/challenge-1", nil)
	assertResultCode(t, getResp, 200)
	if client.lastChallengeID != "challenge-1" {
		t.Fatalf("expected get to capture path param, got %s", client.lastChallengeID)
	}

	respondBody := marshalBody(t, challengefacade.RespondChallengeRequest{
		StepIdentifier: "step-1",
		Payload: map[string]any{
			"captchaCode": "1234",
		},
	})
	respondResp := ut.PerformRequest(engine.Engine, "POST", "/v1/challenges/challenge-1/respond", &ut.Body{
		Body: bytes.NewReader(respondBody),
		Len:  len(respondBody),
	}, ut.Header{Key: "Content-Type", Value: "application/json"})
	assertResultCode(t, respondResp, 200)
	if client.respondRequest.StepIdentifier != "step-1" {
		t.Fatalf("unexpected respond request: %+v", client.respondRequest)
	}

	refreshBody := marshalBody(t, challengefacade.RefreshChallengeRequest{
		StepIdentifier: "step-1",
	})
	refreshResp := ut.PerformRequest(engine.Engine, "POST", "/v1/challenges/challenge-1/refresh", &ut.Body{
		Body: bytes.NewReader(refreshBody),
		Len:  len(refreshBody),
	}, ut.Header{Key: "Content-Type", Value: "application/json"})
	assertResultCode(t, refreshResp, 200)
	if client.refreshRequest.StepIdentifier != "step-1" {
		t.Fatalf("unexpected refresh request: %+v", client.refreshRequest)
	}
}

type fakeInternalFacade struct {
	request  challengefacade.StartChallengeRequest
	response *challengefacade.StartChallengeResponse
}

func (f *fakeInternalFacade) StartChallenge(ctx context.Context, request challengefacade.StartChallengeRequest) (*challengefacade.StartChallengeResponse, error) {
	f.request = request
	return f.response, nil
}

type fakeClientFacade struct {
	lastChallengeID string
	getResponse     *challengefacade.StartChallengeResponse
	respondResponse *challengefacade.RespondChallengeResponse
	refreshResponse *challengefacade.StartChallengeResponse
	respondRequest  challengefacade.RespondChallengeRequest
	refreshRequest  challengefacade.RefreshChallengeRequest
}

func (f *fakeClientFacade) GetChallenge(ctx context.Context, challengeIdentifier string) (*challengefacade.StartChallengeResponse, error) {
	f.lastChallengeID = challengeIdentifier
	return f.getResponse, nil
}

func (f *fakeClientFacade) Respond(ctx context.Context, challengeIdentifier string, request challengefacade.RespondChallengeRequest) (*challengefacade.RespondChallengeResponse, error) {
	f.lastChallengeID = challengeIdentifier
	f.respondRequest = request
	return f.respondResponse, nil
}

func (f *fakeClientFacade) Refresh(ctx context.Context, challengeIdentifier string, request challengefacade.RefreshChallengeRequest) (*challengefacade.StartChallengeResponse, error) {
	f.lastChallengeID = challengeIdentifier
	f.refreshRequest = request
	return f.refreshResponse, nil
}

func marshalBody(t *testing.T, value any) []byte {
	t.Helper()
	body, err := json.Marshal(value)
	if err != nil {
		t.Fatalf("marshal body: %v", err)
	}
	return body
}

func assertResultCode(t *testing.T, recorder *ut.ResponseRecorder, expectedStatus int) {
	t.Helper()
	if recorder.Code != expectedStatus {
		t.Fatalf("unexpected status: %d body=%s", recorder.Code, recorder.Body.String())
	}
	var result response.Result
	if err := json.Unmarshal(recorder.Body.Bytes(), &result); err != nil {
		t.Fatalf("unmarshal result: %v", err)
	}
	if result.Code != 0 {
		t.Fatalf("unexpected business result: %+v", result)
	}
}

var _ challengefacade.ChallengeInternalFacade = (*fakeInternalFacade)(nil)
var _ challengefacade.ChallengeClientFacade = (*fakeClientFacade)(nil)

func init() {
	_ = time.UTC
}
