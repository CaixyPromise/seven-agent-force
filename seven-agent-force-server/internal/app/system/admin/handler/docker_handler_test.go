package handler

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/CaixyPromise/seven-agent/force/internal/infrastructure/docker"
	"github.com/cloudwego/hertz/pkg/app/server"
	"github.com/cloudwego/hertz/pkg/common/ut"
)

func TestDockerContainerBasicActionsReturnBooleanSynchronously(t *testing.T) {
	tests := []struct {
		name         string
		method       string
		path         string
		register     func(*server.Hertz, *Handler)
		wantAction   string
		wantValidate string
	}{
		{
			name:         "start",
			method:       "POST",
			path:         "/containers/container-1/start",
			register:     func(engine *server.Hertz, h *Handler) { engine.POST("/containers/:id/start", h.StartDockerContainer) },
			wantAction:   "start:container-1",
			wantValidate: docker.OperationTypeContainerStart,
		},
		{
			name:         "stop",
			method:       "POST",
			path:         "/containers/container-1/stop",
			register:     func(engine *server.Hertz, h *Handler) { engine.POST("/containers/:id/stop", h.StopDockerContainer) },
			wantAction:   "stop:container-1",
			wantValidate: docker.OperationTypeContainerStop,
		},
		{
			name:   "restart",
			method: "POST",
			path:   "/containers/container-1/restart",
			register: func(engine *server.Hertz, h *Handler) {
				engine.POST("/containers/:id/restart", h.RestartDockerContainer)
			},
			wantAction:   "restart:container-1",
			wantValidate: docker.OperationTypeContainerRestart,
		},
		{
			name:         "delete",
			method:       "DELETE",
			path:         "/containers/container-1",
			register:     func(engine *server.Hertz, h *Handler) { engine.DELETE("/containers/:id", h.DeleteDockerContainer) },
			wantAction:   "delete:container-1",
			wantValidate: docker.OperationTypeContainerDelete,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fakeDocker := &fakeDockerSyncService{}
			handler := NewHandler(nil, nil, nil, nil, fakeDocker)
			engine := server.Default()
			tt.register(engine, handler)

			resp := ut.PerformRequest(engine.Engine, tt.method, tt.path, nil)
			if resp.Code != 200 {
				t.Fatalf("unexpected status: %d body=%s", resp.Code, resp.Body.String())
			}
			var body struct {
				Code int  `json:"code"`
				Data bool `json:"data"`
			}
			if err := json.Unmarshal(resp.Body.Bytes(), &body); err != nil {
				t.Fatalf("response data should be boolean: %v body=%s", err, resp.Body.String())
			}
			if !body.Data {
				t.Fatalf("expected data=true, got body=%s", resp.Body.String())
			}
			if fakeDocker.submitted {
				t.Fatalf("basic container action should not submit an async operation")
			}
			if got := fakeDocker.action; got != tt.wantAction {
				t.Fatalf("unexpected sync action: got %q want %q", got, tt.wantAction)
			}
			if got := fakeDocker.validatedOperation; got != tt.wantValidate {
				t.Fatalf("unexpected validation operation: got %q want %q", got, tt.wantValidate)
			}
		})
	}
}

func TestDockerEnabledReflectsDockerServiceAvailability(t *testing.T) {
	if NewHandler(nil, nil, nil, nil, nil).DockerEnabled() {
		t.Fatal("nil docker service should be disabled")
	}
	if NewHandler(nil, nil, nil, nil, fakeDockerAvailabilityService{enabled: false}).DockerEnabled() {
		t.Fatal("disabled docker service should be disabled")
	}
	if !NewHandler(nil, nil, nil, nil, fakeDockerAvailabilityService{enabled: true}).DockerEnabled() {
		t.Fatal("enabled docker service should be enabled")
	}
}

type fakeDockerAvailabilityService struct {
	docker.Service
	enabled bool
}

func (f fakeDockerAvailabilityService) Enabled() bool {
	return f.enabled
}

type fakeDockerSyncService struct {
	docker.Service

	action             string
	validatedOperation string
	submitted          bool
}

func (f *fakeDockerSyncService) ValidateContainerOperation(_ context.Context, id, operationType string) error {
	f.validatedOperation = operationType
	return nil
}

func (f *fakeDockerSyncService) StartContainer(ctx context.Context, id string) (bool, error) {
	if err := f.ValidateContainerOperation(ctx, id, docker.OperationTypeContainerStart); err != nil {
		return false, err
	}
	f.action = "start:" + id
	return true, nil
}

func (f *fakeDockerSyncService) StopContainer(ctx context.Context, id string) (bool, error) {
	if err := f.ValidateContainerOperation(ctx, id, docker.OperationTypeContainerStop); err != nil {
		return false, err
	}
	f.action = "stop:" + id
	return true, nil
}

func (f *fakeDockerSyncService) RestartContainer(ctx context.Context, id string) (bool, error) {
	if err := f.ValidateContainerOperation(ctx, id, docker.OperationTypeContainerRestart); err != nil {
		return false, err
	}
	f.action = "restart:" + id
	return true, nil
}

func (f *fakeDockerSyncService) DeleteContainer(ctx context.Context, id string) (bool, error) {
	if err := f.ValidateContainerOperation(ctx, id, docker.OperationTypeContainerDelete); err != nil {
		return false, err
	}
	f.action = "delete:" + id
	return true, nil
}

func (f *fakeDockerSyncService) SubmitOperation(context.Context, docker.OperationSubmitCommand) (*docker.OperationAcceptedVO, error) {
	f.submitted = true
	return &docker.OperationAcceptedVO{
		OperationID:   123,
		OperationType: docker.OperationTypeContainerStart,
		TargetType:    "container",
		TargetID:      "container-1",
		Status:        docker.OperationStatusPending,
	}, nil
}
