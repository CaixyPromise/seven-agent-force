package handler

import (
	"context"
	"net/http"
	"strings"
	"testing"

	setupfacade "github.com/CaixyPromise/seven-agent/force/internal/app/setup/facade"
	ssofacade "github.com/CaixyPromise/seven-agent/force/internal/app/sso/facade"
	"github.com/CaixyPromise/seven-agent/force/internal/common/response"
	"github.com/CaixyPromise/seven-agent/force/internal/shared/config"
	"github.com/cloudwego/hertz/pkg/app/server"
	"github.com/cloudwego/hertz/pkg/common/test/assert"
	"github.com/cloudwego/hertz/pkg/common/ut"
)

func TestCreateOwnerRequiresJSONAndOrigin(t *testing.T) {
	engine := server.New(server.WithHostPorts("127.0.0.1:0"))
	handler := NewHandler(&fakeSetupService{}, config.SetupConfig{
		RequireOriginHeader:   true,
		AllowedOriginPatterns: []string{"http://127.0.0.1:*", "http://localhost:*"},
	})
	engine.POST("/setup/owner", handler.CreateOwner)

	resp := ut.PerformRequest(engine.Engine, http.MethodPost, "/setup/owner", nil)
	assert.DeepEqual(t, http.StatusOK, resp.Code)
	if !strings.Contains(resp.Body.String(), "初始化请求来源不可信") {
		t.Fatalf("expected origin rejection, got %s", resp.Body.String())
	}

	resp = ut.PerformRequest(engine.Engine, http.MethodPost, "/setup/owner", &ut.Body{Body: strings.NewReader("x"), Len: 1},
		ut.Header{Key: "Origin", Value: "http://127.0.0.1:5177"},
		ut.Header{Key: "Content-Type", Value: "text/plain"})
	assert.DeepEqual(t, http.StatusOK, resp.Code)
	if !strings.Contains(resp.Body.String(), "application/json") {
		t.Fatalf("expected content-type rejection, got %s", resp.Body.String())
	}
}

func TestCreateOwnerWritesBootstrapCookies(t *testing.T) {
	engine := server.New(server.WithHostPorts("127.0.0.1:0"))
	handler := NewHandler(&fakeSetupService{}, config.SetupConfig{
		RequireOriginHeader:   true,
		AllowedOriginPatterns: []string{"http://127.0.0.1:*"},
	})
	engine.POST("/setup/owner", handler.CreateOwner)

	resp := ut.PerformRequest(engine.Engine, http.MethodPost, "/setup/owner",
		&ut.Body{Body: strings.NewReader(`{"username":"Owner1","password":"Owner123","confirmPassword":"Owner123"}`), Len: 73},
		ut.Header{Key: "Origin", Value: "http://127.0.0.1:5177"},
		ut.Header{Key: "Content-Type", Value: "application/json"},
		ut.Header{Key: "X-Setup-Token", Value: "token"})
	assert.DeepEqual(t, http.StatusOK, resp.Code)
	if !strings.Contains(resp.Body.String(), `"accessToken":"access-token"`) {
		t.Fatalf("expected owner response, got %s", resp.Body.String())
	}
	cookies := string(resp.Header().Peek("Set-Cookie"))
	if !strings.Contains(cookies, "SEVEN_SSO_SESSION=s1") {
		t.Fatalf("expected session cookie, got %q", cookies)
	}
}

type fakeSetupService struct{}

func (f *fakeSetupService) GetSetupStatus(context.Context) (*setupfacade.SetupStatusDTO, error) {
	token := "setup-token"
	return &setupfacade.SetupStatusDTO{SetupToken: &token}, nil
}

func (f *fakeSetupService) CreateOwner(context.Context, setupfacade.SetupOwnerRequestDTO, string, *ssofacade.RequestContext) (*setupfacade.OwnerBootstrapResult, error) {
	return &setupfacade.OwnerBootstrapResult{
		Owner:                    &setupfacade.SetupOwnerResultDTO{ID: 1001, Username: "Owner1", AccessToken: "access-token", TokenType: "Bearer", AccessTTLSec: 1800},
		SessionCookieHeaderValue: "SEVEN_SSO_SESSION=s1",
		RefreshCookieHeaderValue: "seven_sso_rt=r1",
	}, nil
}

var _ = response.Result{}
