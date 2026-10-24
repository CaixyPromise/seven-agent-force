package controller

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"testing"

	"github.com/cloudwego/hertz/pkg/app"
)

func TestDiscoveryErrorEnvelopeCarriesSafeReasonCode(t *testing.T) {
	context := &app.RequestContext{}
	writeError(context, http.StatusBadGateway, "MODEL_DISCOVERY_UNAVAILABLE", "无法从上游获取模型目录")
	var envelope struct {
		Message string `json:"message"`
		Data    struct {
			ReasonCode string `json:"reasonCode"`
		} `json:"data"`
	}
	if err := json.Unmarshal(context.Response.Body(), &envelope); err != nil {
		t.Fatal(err)
	}
	if envelope.Data.ReasonCode != "MODEL_DISCOVERY_UNAVAILABLE" {
		t.Fatalf("unsafe or incomplete error envelope: %+v", envelope)
	}
}

func TestUnknownModelCredentialErrorDoesNotExposeInfrastructureDetail(t *testing.T) {
	context := &app.RequestContext{}
	writeServiceError(context, errors.New(`ERROR: relation "workforce_model_credentials" does not exist`))
	var envelope struct {
		Message string `json:"message"`
		Data    struct {
			ReasonCode string `json:"reasonCode"`
		} `json:"data"`
	}
	if err := json.Unmarshal(context.Response.Body(), &envelope); err != nil {
		t.Fatal(err)
	}
	if envelope.Data.ReasonCode != "MODEL_CREDENTIAL_INTERNAL_ERROR" || strings.Contains(strings.ToLower(envelope.Message), "relation") || strings.Contains(strings.ToLower(envelope.Message), "provider_profiles") {
		t.Fatalf("unknown model credential failure leaked or was misclassified: envelope=%+v", envelope)
	}
}
