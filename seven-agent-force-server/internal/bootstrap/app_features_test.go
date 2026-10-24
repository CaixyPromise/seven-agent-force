package bootstrap

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"

	apperrors "github.com/CaixyPromise/seven-agent/force/internal/common/errors"
	"github.com/CaixyPromise/seven-agent/force/internal/common/response"
	"github.com/CaixyPromise/seven-agent/force/internal/infrastructure/datasource/store"
	dockerinfra "github.com/CaixyPromise/seven-agent/force/internal/infrastructure/docker"
	secretvalueinfra "github.com/CaixyPromise/seven-agent/force/internal/infrastructure/security/crypto/secretvalue"
	"github.com/CaixyPromise/seven-agent/force/internal/shared/config"
	"github.com/CaixyPromise/seven-agent/force/internal/shared/features"
	"github.com/CaixyPromise/seven-agent/force/internal/shared/xid"
	"github.com/cloudwego/hertz/pkg/common/ut"
	"go.uber.org/zap"
)

func TestActivateDockerFeatureSkipsFactoryWhenCapabilityDisabled(t *testing.T) {
	called := false
	effective, service, err := activateDockerFeature(context.Background(), features.Set{}, config.DockerConfig{}, nil, nil, nil, nil, zap.NewNop(), func(config.DockerConfig, []string, *xid.Generator, store.Provider, secretvalueinfra.Service) (dockerinfra.Service, error) {
		called = true
		return nil, nil
	})
	if err != nil || service != nil || called || effective.Enabled(features.DockerAdmin) {
		t.Fatalf("features=%v service=%v err=%v factoryCalled=%v", effective, service, err, called)
	}
}

func TestActivateDockerFeatureKeepsAvailableCapability(t *testing.T) {
	fake := &fakeDockerService{}
	effective, service, err := activateDockerFeature(context.Background(), features.Set{features.DockerAdmin: {}}, config.DockerConfig{Enabled: true}, nil, nil, nil, nil, zap.NewNop(), func(config.DockerConfig, []string, *xid.Generator, store.Provider, secretvalueinfra.Service) (dockerinfra.Service, error) {
		return fake, nil
	})
	if err != nil || service != fake || !effective.Enabled(features.DockerAdmin) || fake.closed {
		t.Fatalf("features=%v service=%v err=%v closed=%v", effective, service, err, fake.closed)
	}
}

func TestActivateDockerFeaturePassesGlobalOriginPatterns(t *testing.T) {
	fake := &fakeDockerService{}
	want := []string{"https://console.example.com", "https://admin.example.com"}
	var got []string
	effective, service, err := activateDockerFeature(context.Background(), features.Set{features.DockerAdmin: {}}, config.DockerConfig{Enabled: true}, want, nil, nil, nil, zap.NewNop(), func(_ config.DockerConfig, originPatterns []string, _ *xid.Generator, _ store.Provider, _ secretvalueinfra.Service) (dockerinfra.Service, error) {
		got = append([]string(nil), originPatterns...)
		return fake, nil
	})
	if err != nil || service != fake || !effective.Enabled(features.DockerAdmin) {
		t.Fatalf("features=%v service=%v err=%v", effective, service, err)
	}
	if strings.Join(got, ",") != strings.Join(want, ",") {
		t.Fatalf("origin patterns = %v, want %v", got, want)
	}
}

func TestActivateDockerFeatureDisablesUnavailableOptionalCapability(t *testing.T) {
	fake := &fakeDockerService{pingErr: errors.New("daemon offline")}
	effective, service, err := activateDockerFeature(context.Background(), features.Set{
		features.PlatformControl: {},
		features.DockerAdmin:     {},
	}, config.DockerConfig{Enabled: true}, nil, nil, nil, nil, zap.NewNop(), func(config.DockerConfig, []string, *xid.Generator, store.Provider, secretvalueinfra.Service) (dockerinfra.Service, error) {
		return fake, nil
	})
	if err != nil || service != nil || effective.Enabled(features.DockerAdmin) || !effective.Enabled(features.PlatformControl) || !fake.closed {
		t.Fatalf("features=%v service=%v err=%v closed=%v", effective, service, err, fake.closed)
	}
}

func TestActivateDockerFeatureFailsWhenUnavailableAndFailFast(t *testing.T) {
	fake := &fakeDockerService{pingErr: errors.New("daemon offline")}
	_, service, err := activateDockerFeature(context.Background(), features.Set{features.DockerAdmin: {}}, config.DockerConfig{Enabled: true, FailFast: true}, nil, nil, nil, nil, zap.NewNop(), func(config.DockerConfig, []string, *xid.Generator, store.Provider, secretvalueinfra.Service) (dockerinfra.Service, error) {
		return fake, nil
	})
	if err == nil || service != nil || !fake.closed || !strings.Contains(err.Error(), "docker.admin") || !strings.Contains(err.Error(), "docker.enabled=false") {
		t.Fatalf("service=%v err=%v closed=%v", service, err, fake.closed)
	}
}

func TestActivateDockerFeatureRejectsNilService(t *testing.T) {
	_, _, err := activateDockerFeature(context.Background(), features.Set{features.DockerAdmin: {}}, config.DockerConfig{Enabled: true}, nil, nil, nil, nil, zap.NewNop(), func(config.DockerConfig, []string, *xid.Generator, store.Provider, secretvalueinfra.Service) (dockerinfra.Service, error) {
		return nil, nil
	})
	if err == nil || !strings.Contains(err.Error(), "nil service") {
		t.Fatalf("err=%v", err)
	}
}

func TestAppNewRemovesUnavailableDockerFromRuntimeSurface(t *testing.T) {
	dir := t.TempDir()
	missingSocket := fmt.Sprintf("/tmp/seven-docker-%d-soft.sock", os.Getpid())
	writeConfig(t, filepath.Join(dir, "application.yaml"), `
seven:
  name: unavailable-docker-feature-test
server:
  host: 127.0.0.1
  port: 9999
  contextPath: /api
datasource:
  driver: mysql
cache:
  enabled: true
  redis:
    enabled: false
logging:
  level: debug
  format: json
id:
  node: 9
setup:
  enabled: false
platform:
  mode: local
docker:
  enabled: true
  failFast: false
  engine:
    host: unix://`+missingSocket+`
`)

	app, err := New(dir)
	if err != nil {
		t.Fatalf("bootstrap app: %v", err)
	}
	if app.features.Enabled(features.DockerAdmin) || app.docker != nil {
		t.Fatalf("effective features=%v docker=%v", app.features, app.docker)
	}

	runtimeResp := ut.PerformRequest(app.Engine(), http.MethodGet, "/api/system/features/runtime", nil)
	var runtimeResult struct {
		Code int `json:"code"`
		Data struct {
			Features struct {
				Enabled []string `json:"enabled"`
			} `json:"features"`
		} `json:"data"`
	}
	if err := json.Unmarshal(runtimeResp.Body.Bytes(), &runtimeResult); err != nil {
		t.Fatalf("decode runtime features: %v body=%s", err, runtimeResp.Body.String())
	}
	if runtimeResult.Code != 0 || containsString(runtimeResult.Data.Features.Enabled, string(features.DockerAdmin)) {
		t.Fatalf("runtime response=%s", runtimeResp.Body.String())
	}

	dockerResp := ut.PerformRequest(app.Engine(), http.MethodGet, "/api/admin/docker/operations", nil)
	var dockerResult response.Result
	if err := json.Unmarshal(dockerResp.Body.Bytes(), &dockerResult); err != nil {
		t.Fatalf("decode Docker response: %v body=%s", err, dockerResp.Body.String())
	}
	if dockerResult.Code != apperrors.CodeNotFound || dockerResult.Message != "请求路径不存在" {
		t.Fatalf("Docker route must not be mounted: status=%d body=%s", dockerResp.Code, dockerResp.Body.String())
	}
}

func TestAppNewFailsWhenRequiredDockerIsUnavailable(t *testing.T) {
	dir := t.TempDir()
	missingSocket := fmt.Sprintf("/tmp/seven-docker-%d-required.sock", os.Getpid())
	writeConfig(t, filepath.Join(dir, "application.yaml"), `
server:
  host: 127.0.0.1
  port: 9999
datasource:
  driver: mysql
cache:
  redis:
    enabled: false
logging:
  level: debug
id:
  node: 9
setup:
  enabled: false
docker:
  enabled: true
  failFast: true
  engine:
    host: unix://`+missingSocket+`
`)

	_, err := New(dir)
	if err == nil || !strings.Contains(err.Error(), "required feature docker.admin is unavailable") {
		t.Fatalf("err=%v", err)
	}
}

func containsString(values []string, target string) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}

type fakeDockerService struct {
	dockerinfra.Service
	pingErr error
	closed  bool
}

func (f *fakeDockerService) Enabled() bool { return true }

func (f *fakeDockerService) Ping(context.Context) error { return f.pingErr }

func (f *fakeDockerService) Close() error {
	f.closed = true
	return nil
}

func TestAppNewKeepsExternalLoginProviderAdminRouteInEveryPlatformMode(t *testing.T) {
	for _, mode := range []config.PlatformMode{
		config.PlatformModeLocal,
		config.PlatformModeHub,
		config.PlatformModeNode,
	} {
		t.Run(string(mode), func(t *testing.T) {
			dir := t.TempDir()
			nodeConfig := ""
			if mode == config.PlatformModeNode {
				nodeConfig = "\n  node:\n    code: oauth-provider-route-node\n    managementBearer: node-bearer\n"
			}
			writeConfig(t, filepath.Join(dir, "application.yaml"), `
seven:
  name: external-login-provider-route-test
server:
  host: 127.0.0.1
  port: 9999
  contextPath: /api
datasource:
  driver: mysql
cache:
  enabled: true
  redis:
    enabled: false
logging:
  level: debug
  format: json
id:
  node: 9
setup:
  enabled: false
platform:
  mode: `+string(mode)+nodeConfig)

			app, err := New(dir)
			if err != nil {
				t.Fatalf("bootstrap %s app: %v", mode, err)
			}
			resp := ut.PerformRequest(app.Engine(), http.MethodGet, "/api/external-login/admin/providers", nil)
			var result response.Result
			if err := json.Unmarshal(resp.Body.Bytes(), &result); err != nil {
				t.Fatalf("decode %s provider response: %v body=%s", mode, err, resp.Body.String())
			}
			if resp.Code != http.StatusOK || result.Code != apperrors.CodeNotLogin {
				t.Fatalf("%s provider route must exist: status=%d result=%+v", mode, resp.Code, result)
			}
		})
	}
}
