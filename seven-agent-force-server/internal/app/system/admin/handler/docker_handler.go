package handler

import (
	"context"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"

	apperrors "github.com/CaixyPromise/seven-agent/force/internal/common/errors"
	"github.com/CaixyPromise/seven-agent/force/internal/common/httpx"
	"github.com/CaixyPromise/seven-agent/force/internal/common/response"
	dockerinfra "github.com/CaixyPromise/seven-agent/force/internal/infrastructure/docker"
	"github.com/CaixyPromise/seven-agent/force/internal/shared/securitycontext"
	"github.com/cloudwego/hertz/pkg/app"
	"github.com/cloudwego/hertz/pkg/common/adaptor"
)

func (c *Handler) GetDockerContainers(ctx context.Context, reqCtx *app.RequestContext) {
	page, err := c.docker.ListContainers(
		ctx,
		parseQueryInt64WithDefault(reqCtx, "current", 1),
		parseQueryInt64WithDefault(reqCtx, "size", 10),
		strings.TrimSpace(string(reqCtx.Query("keyword"))),
		strings.TrimSpace(string(reqCtx.Query("state"))),
	)
	writeDockerResult(reqCtx, page, err)
}

func (c *Handler) DockerEnabled() bool {
	return c != nil && c.docker != nil && c.docker.Enabled()
}

func (c *Handler) GetDockerContainer(ctx context.Context, reqCtx *app.RequestContext) {
	result, err := c.docker.GetContainer(ctx, pathString(reqCtx, "id"))
	writeDockerResult(reqCtx, result, err)
}

func (c *Handler) GetDockerContainerLogs(ctx context.Context, reqCtx *app.RequestContext) {
	query, err := dockerLogQuery(reqCtx)
	if err != nil {
		response.Error(reqCtx, err)
		return
	}
	result, err := c.docker.GetContainerLogsQuery(ctx, pathString(reqCtx, "id"), query)
	writeDockerResult(reqCtx, result, err)
}

func (c *Handler) StreamDockerContainerLogs(ctx context.Context, reqCtx *app.RequestContext) {
	query, err := dockerLogQuery(reqCtx)
	if err != nil {
		response.Error(reqCtx, err)
		return
	}
	if strings.TrimSpace(string(reqCtx.Query("follow"))) == "" {
		query.Follow = true
	}
	stream, err := c.docker.StreamContainerLogs(ctx, pathString(reqCtx, "id"), query)
	if err != nil {
		response.Error(reqCtx, err)
		return
	}
	writeSSEStream(ctx, reqCtx, stream)
}

func (c *Handler) GetDockerContainerStats(ctx context.Context, reqCtx *app.RequestContext) {
	result, err := c.docker.GetContainerStats(ctx, pathString(reqCtx, "id"))
	writeDockerResult(reqCtx, result, err)
}

func (c *Handler) StreamDockerContainerStats(ctx context.Context, reqCtx *app.RequestContext) {
	stream, err := c.docker.StreamContainerStats(ctx, pathString(reqCtx, "id"))
	if err != nil {
		response.Error(reqCtx, err)
		return
	}
	writeSSEStream(ctx, reqCtx, stream)
}

func (c *Handler) OpenDockerContainerTerminal(ctx context.Context, reqCtx *app.RequestContext) {
	command := dockerinfra.ContainerTerminalRequest{
		Shell: strings.TrimSpace(string(reqCtx.Query("shell"))),
		Rows:  parseQueryUintWithDefault(reqCtx, "rows", 0),
		Cols:  parseQueryUintWithDefault(reqCtx, "cols", 0),
	}
	adaptor.HertzHandler(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if err := c.docker.ServeContainerTerminal(request.Context(), writer, request, pathString(reqCtx, "id"), command); err != nil {
			http.Error(writer, err.Error(), http.StatusBadRequest)
		}
	}))(ctx, reqCtx)
}

func (c *Handler) StartDockerContainer(ctx context.Context, reqCtx *app.RequestContext) {
	result, err := c.docker.StartContainer(ctx, pathString(reqCtx, "id"))
	writeDockerResult(reqCtx, result, err)
}

func (c *Handler) StopDockerContainer(ctx context.Context, reqCtx *app.RequestContext) {
	result, err := c.docker.StopContainer(ctx, pathString(reqCtx, "id"))
	writeDockerResult(reqCtx, result, err)
}

func (c *Handler) RestartDockerContainer(ctx context.Context, reqCtx *app.RequestContext) {
	result, err := c.docker.RestartContainer(ctx, pathString(reqCtx, "id"))
	writeDockerResult(reqCtx, result, err)
}

func (c *Handler) DeleteDockerContainer(ctx context.Context, reqCtx *app.RequestContext) {
	result, err := c.docker.DeleteContainer(ctx, pathString(reqCtx, "id"))
	writeDockerResult(reqCtx, result, err)
}

func (c *Handler) ExportDockerContainerCompose(ctx context.Context, reqCtx *app.RequestContext) {
	result, err := c.submitDockerIDOperation(ctx, reqCtx, dockerinfra.OperationTypeComposeExport, "container", pathString(reqCtx, "id"))
	writeDockerResult(reqCtx, result, err)
}

func (c *Handler) GetLocalDockerImages(ctx context.Context, reqCtx *app.RequestContext) {
	page, err := c.docker.ListImages(
		ctx,
		parseQueryInt64WithDefault(reqCtx, "current", 1),
		parseQueryInt64WithDefault(reqCtx, "size", 10),
		strings.TrimSpace(string(reqCtx.Query("keyword"))),
	)
	writeDockerResult(reqCtx, page, err)
}

func (c *Handler) GetLocalDockerImage(ctx context.Context, reqCtx *app.RequestContext) {
	result, err := c.docker.GetImage(ctx, pathString(reqCtx, "id"))
	writeDockerResult(reqCtx, result, err)
}

func (c *Handler) GetLocalDockerImageContainers(ctx context.Context, reqCtx *app.RequestContext) {
	result, err := c.docker.ListImageContainers(ctx, pathString(reqCtx, "id"))
	writeDockerResult(reqCtx, result, err)
}

func (c *Handler) PullDockerImage(ctx context.Context, reqCtx *app.RequestContext) {
	var request dockerinfra.ImagePullCommand
	if err := httpx.Bind(reqCtx, &request); err != nil {
		response.Error(reqCtx, err)
		return
	}
	result, err := c.docker.SubmitOperation(ctx, dockerinfra.OperationSubmitCommand{
		OperationType: dockerinfra.OperationTypeImagePull,
		TargetType:    "image",
		TargetName:    strings.TrimSpace(request.Repository),
		Payload:       request,
		Actor:         dockerActor(reqCtx),
	})
	writeDockerResult(reqCtx, result, err)
}

func (c *Handler) TagDockerImage(ctx context.Context, reqCtx *app.RequestContext) {
	var request dockerinfra.ImageTagCommand
	if err := httpx.Bind(reqCtx, &request); err != nil {
		response.Error(reqCtx, err)
		return
	}
	result, err := c.docker.SubmitOperation(ctx, dockerinfra.OperationSubmitCommand{
		OperationType: dockerinfra.OperationTypeImageTag,
		TargetType:    "image",
		TargetName:    strings.TrimSpace(request.TargetRepository) + ":" + strings.TrimSpace(request.TargetTag),
		Payload:       request,
		Actor:         dockerActor(reqCtx),
	})
	writeDockerResult(reqCtx, result, err)
}

func (c *Handler) PushDockerImage(ctx context.Context, reqCtx *app.RequestContext) {
	var request dockerinfra.ImagePushCommand
	if err := httpx.Bind(reqCtx, &request); err != nil {
		response.Error(reqCtx, err)
		return
	}
	result, err := c.docker.SubmitOperation(ctx, dockerinfra.OperationSubmitCommand{
		OperationType: dockerinfra.OperationTypeImagePush,
		TargetType:    "image",
		TargetName:    strings.TrimSpace(request.SourceImage),
		Payload:       request,
		Actor:         dockerActor(reqCtx),
	})
	writeDockerResult(reqCtx, result, err)
}

func (c *Handler) DeleteLocalDockerImage(ctx context.Context, reqCtx *app.RequestContext) {
	result, err := c.submitDockerIDOperation(ctx, reqCtx, dockerinfra.OperationTypeImageDelete, "image", pathString(reqCtx, "id"))
	writeDockerResult(reqCtx, result, err)
}

func (c *Handler) ExportLocalDockerImage(ctx context.Context, reqCtx *app.RequestContext) {
	result, err := c.submitDockerIDOperation(ctx, reqCtx, dockerinfra.OperationTypeImageExport, "image", pathString(reqCtx, "id"))
	writeDockerResult(reqCtx, result, err)
}

func (c *Handler) PullRemoteDockerImage(ctx context.Context, reqCtx *app.RequestContext) {
	var request dockerinfra.RemoteImagePullRequest
	if err := httpx.Bind(reqCtx, &request); err != nil {
		response.Error(reqCtx, err)
		return
	}
	result, err := c.docker.SubmitOperation(ctx, dockerinfra.OperationSubmitCommand{
		OperationType: dockerinfra.OperationTypeImageRemotePull,
		TargetType:    "image",
		TargetName:    strings.TrimSpace(request.Repository) + ":" + strings.TrimSpace(request.Tag),
		Payload:       request,
		Actor:         dockerActor(reqCtx),
	})
	writeDockerResult(reqCtx, result, err)
}

func (c *Handler) GetDockerImageStartupPreview(ctx context.Context, reqCtx *app.RequestContext) {
	result, err := c.docker.StartupPreview(ctx, pathString(reqCtx, "id"))
	writeDockerResult(reqCtx, result, err)
}

func (c *Handler) CreateDockerContainerFromImage(ctx context.Context, reqCtx *app.RequestContext) {
	var request dockerinfra.ContainerCreateRequest
	if err := httpx.Bind(reqCtx, &request); err != nil {
		response.Error(reqCtx, err)
		return
	}
	result, err := c.docker.SubmitOperation(ctx, dockerinfra.OperationSubmitCommand{
		OperationType: dockerinfra.OperationTypeContainerCreate,
		TargetType:    "container",
		TargetName:    strings.TrimSpace(request.ContainerName),
		Payload:       request,
		Actor:         dockerActor(reqCtx),
	})
	writeDockerResult(reqCtx, result, err)
}

func (c *Handler) ValidateDockerCompose(ctx context.Context, reqCtx *app.RequestContext) {
	var request dockerinfra.ComposeUpRequest
	if err := httpx.Bind(reqCtx, &request); err != nil {
		response.Error(reqCtx, err)
		return
	}
	result, err := c.docker.PreviewCompose(ctx, dockerActor(reqCtx), request)
	writeDockerResult(reqCtx, result, err)
}

func (c *Handler) UpDockerCompose(ctx context.Context, reqCtx *app.RequestContext) {
	var request dockerinfra.ComposeUpRequest
	if err := httpx.Bind(reqCtx, &request); err != nil {
		response.Error(reqCtx, err)
		return
	}
	result, err := c.docker.SubmitOperation(ctx, dockerinfra.OperationSubmitCommand{
		OperationType: dockerinfra.OperationTypeComposeUp,
		TargetType:    "compose",
		TargetName:    strings.TrimSpace(request.ProjectName),
		Payload:       request,
		Actor:         dockerActor(reqCtx),
	})
	writeDockerResult(reqCtx, result, err)
}

func (c *Handler) PreviewDockerCompose(ctx context.Context, reqCtx *app.RequestContext) {
	var request dockerinfra.ComposeUpRequest
	if err := httpx.Bind(reqCtx, &request); err != nil {
		response.Error(reqCtx, err)
		return
	}
	result, err := c.docker.PreviewCompose(ctx, dockerActor(reqCtx), request)
	writeDockerResult(reqCtx, result, err)
}

func (c *Handler) CheckDockerComposeWorkspace(ctx context.Context, reqCtx *app.RequestContext) {
	var request dockerinfra.ComposeWorkspaceCheckRequest
	if err := httpx.Bind(reqCtx, &request); err != nil {
		response.Error(reqCtx, err)
		return
	}
	result, err := c.docker.CheckComposeWorkspace(ctx, request)
	writeDockerResult(reqCtx, result, err)
}

func (c *Handler) ValidateDockerComposeYaml(ctx context.Context, reqCtx *app.RequestContext) {
	var request dockerinfra.ComposeYamlValidateRequest
	if err := httpx.Bind(reqCtx, &request); err != nil {
		response.Error(reqCtx, err)
		return
	}
	result, err := c.docker.ValidateComposeYaml(ctx, request)
	writeDockerResult(reqCtx, result, err)
}

func (c *Handler) GetDockerComposeBuilderMetadata(ctx context.Context, reqCtx *app.RequestContext) {
	result, err := c.docker.GetComposeBuilderMetadata(ctx)
	writeDockerResult(reqCtx, result, err)
}

func (c *Handler) PreviewDockerfileBuild(ctx context.Context, reqCtx *app.RequestContext) {
	var request dockerinfra.DockerfileBuildPreviewRequest
	if err := httpx.Bind(reqCtx, &request); err != nil {
		response.Error(reqCtx, err)
		return
	}
	result, err := c.docker.PreviewDockerfileBuild(ctx, request)
	writeDockerResult(reqCtx, result, err)
}

func (c *Handler) PreviewDockerComposeWithFiles(ctx context.Context, reqCtx *app.RequestContext) {
	var request dockerinfra.ComposePreviewWithFilesRequest
	if err := httpx.Bind(reqCtx, &request); err != nil {
		response.Error(reqCtx, err)
		return
	}
	result, err := c.docker.PreviewComposeWithFiles(ctx, dockerActor(reqCtx), request)
	writeDockerResult(reqCtx, result, err)
}

func (c *Handler) GetDockerComposeProjects(ctx context.Context, reqCtx *app.RequestContext) {
	result, err := c.docker.ListComposeProjects(
		ctx,
		parseQueryInt64WithDefault(reqCtx, "current", 1),
		parseQueryInt64WithDefault(reqCtx, "size", 10),
		strings.TrimSpace(string(reqCtx.Query("keyword"))),
		strings.TrimSpace(string(reqCtx.Query("status"))),
	)
	writeDockerResult(reqCtx, result, err)
}

func (c *Handler) GetDockerComposeProject(ctx context.Context, reqCtx *app.RequestContext) {
	result, err := c.docker.GetComposeProject(ctx, pathString(reqCtx, "projectId"))
	writeDockerResult(reqCtx, result, err)
}

func (c *Handler) ImportDiscoveredDockerComposeProject(ctx context.Context, reqCtx *app.RequestContext) {
	var request dockerinfra.DockerComposeImportDiscoveredRequest
	if err := httpx.Bind(reqCtx, &request); err != nil {
		response.Error(reqCtx, err)
		return
	}
	result, err := c.docker.ImportDiscoveredComposeProject(ctx, dockerActor(reqCtx), request)
	writeDockerResult(reqCtx, result, err)
}

func (c *Handler) CreateDockerComposeProject(ctx context.Context, reqCtx *app.RequestContext) {
	var request dockerinfra.ComposeProjectCreateRequest
	if err := httpx.Bind(reqCtx, &request); err != nil {
		response.Error(reqCtx, err)
		return
	}
	result, err := c.docker.CreateComposeProject(ctx, dockerActor(reqCtx), request)
	writeDockerResult(reqCtx, result, err)
}

func (c *Handler) UpdateDockerComposeProject(ctx context.Context, reqCtx *app.RequestContext) {
	var request dockerinfra.ComposeProjectUpdateRequest
	if err := httpx.Bind(reqCtx, &request); err != nil {
		response.Error(reqCtx, err)
		return
	}
	result, err := c.docker.UpdateComposeProjectCompose(ctx, dockerActor(reqCtx), pathString(reqCtx, "projectId"), request)
	writeDockerResult(reqCtx, result, err)
}

func (c *Handler) PreviewDockerComposeProject(ctx context.Context, reqCtx *app.RequestContext) {
	result, err := c.docker.PreviewComposeProject(ctx, dockerActor(reqCtx), pathString(reqCtx, "projectId"))
	writeDockerResult(reqCtx, result, err)
}

func (c *Handler) ValidateDockerComposeProject(ctx context.Context, reqCtx *app.RequestContext) {
	result, err := c.docker.ValidateComposeProject(ctx, dockerActor(reqCtx), pathString(reqCtx, "projectId"))
	writeDockerResult(reqCtx, result, err)
}

func (c *Handler) UpDockerComposeProject(ctx context.Context, reqCtx *app.RequestContext) {
	result, err := c.docker.SubmitComposeProjectOperation(ctx, dockerActor(reqCtx), pathString(reqCtx, "projectId"), dockerinfra.OperationTypeComposeUp, 0)
	writeDockerResult(reqCtx, result, err)
}

func (c *Handler) DownDockerComposeProject(ctx context.Context, reqCtx *app.RequestContext) {
	result, err := c.docker.SubmitComposeProjectOperation(ctx, dockerActor(reqCtx), pathString(reqCtx, "projectId"), dockerinfra.OperationTypeComposeDown, 0)
	writeDockerResult(reqCtx, result, err)
}

func (c *Handler) RestartDockerComposeProject(ctx context.Context, reqCtx *app.RequestContext) {
	result, err := c.docker.SubmitComposeProjectOperation(ctx, dockerActor(reqCtx), pathString(reqCtx, "projectId"), dockerinfra.OperationTypeComposeRestart, 0)
	writeDockerResult(reqCtx, result, err)
}

func (c *Handler) DockerComposeProjectPS(ctx context.Context, reqCtx *app.RequestContext) {
	result, err := c.docker.ComposeProjectPS(ctx, pathString(reqCtx, "projectId"))
	writeDockerResult(reqCtx, result, err)
}

func (c *Handler) DockerComposeProjectLogs(ctx context.Context, reqCtx *app.RequestContext) {
	result, err := c.docker.SubmitComposeProjectOperation(ctx, dockerActor(reqCtx), pathString(reqCtx, "projectId"), dockerinfra.OperationTypeComposeLogs, int(parseQueryInt64WithDefault(reqCtx, "tail", 200)))
	writeDockerResult(reqCtx, result, err)
}

func (c *Handler) DownDockerCompose(ctx context.Context, reqCtx *app.RequestContext) {
	var request dockerinfra.ComposeUpRequest
	if err := httpx.Bind(reqCtx, &request); err != nil {
		response.Error(reqCtx, err)
		return
	}
	result, err := c.docker.SubmitOperation(ctx, dockerinfra.OperationSubmitCommand{OperationType: dockerinfra.OperationTypeComposeDown, TargetType: "compose", TargetName: request.ProjectName, Payload: request, Actor: dockerActor(reqCtx)})
	writeDockerResult(reqCtx, result, err)
}

func (c *Handler) RestartDockerCompose(ctx context.Context, reqCtx *app.RequestContext) {
	var request dockerinfra.ComposeUpRequest
	if err := httpx.Bind(reqCtx, &request); err != nil {
		response.Error(reqCtx, err)
		return
	}
	result, err := c.docker.SubmitOperation(ctx, dockerinfra.OperationSubmitCommand{OperationType: dockerinfra.OperationTypeComposeRestart, TargetType: "compose", TargetName: request.ProjectName, Payload: request, Actor: dockerActor(reqCtx)})
	writeDockerResult(reqCtx, result, err)
}

func (c *Handler) DockerComposePS(ctx context.Context, reqCtx *app.RequestContext) {
	var request dockerinfra.ComposeUpRequest
	if err := httpx.Bind(reqCtx, &request); err != nil {
		response.Error(reqCtx, err)
		return
	}
	result, err := c.docker.ComposePS(ctx, request)
	writeDockerResult(reqCtx, result, err)
}

func (c *Handler) DockerComposeLogs(ctx context.Context, reqCtx *app.RequestContext) {
	var request dockerinfra.ComposeUpRequest
	if err := httpx.Bind(reqCtx, &request); err != nil {
		response.Error(reqCtx, err)
		return
	}
	payload := map[string]any{"projectName": request.ProjectName, "composeYaml": request.ComposeYaml, "tail": int(parseQueryInt64WithDefault(reqCtx, "tail", 200))}
	result, err := c.docker.SubmitOperation(ctx, dockerinfra.OperationSubmitCommand{OperationType: dockerinfra.OperationTypeComposeLogs, TargetType: "compose", TargetName: request.ProjectName, Payload: payload, Actor: dockerActor(reqCtx)})
	writeDockerResult(reqCtx, result, err)
}

func (c *Handler) GetDockerRegistries(ctx context.Context, reqCtx *app.RequestContext) {
	result, err := c.docker.ListRegistries(ctx)
	writeDockerResult(reqCtx, result, err)
}

func (c *Handler) GetDockerRegistry(ctx context.Context, reqCtx *app.RequestContext) {
	id, err := parsePathInt64(reqCtx, "id")
	if err != nil {
		response.Error(reqCtx, err)
		return
	}
	result, err := c.docker.GetRegistry(ctx, id)
	writeDockerResult(reqCtx, result, err)
}

func (c *Handler) CreateDockerRegistry(ctx context.Context, reqCtx *app.RequestContext) {
	var request dockerinfra.RemoteRegistryCommand
	if err := httpx.Bind(reqCtx, &request); err != nil {
		response.Error(reqCtx, err)
		return
	}
	result, err := c.docker.CreateRegistry(ctx, request)
	writeDockerResult(reqCtx, result, err)
}

func (c *Handler) UpdateDockerRegistry(ctx context.Context, reqCtx *app.RequestContext) {
	id, err := parsePathInt64(reqCtx, "id")
	if err != nil {
		response.Error(reqCtx, err)
		return
	}
	var request dockerinfra.RemoteRegistryCommand
	if err := httpx.Bind(reqCtx, &request); err != nil {
		response.Error(reqCtx, err)
		return
	}
	result, err := c.docker.UpdateRegistry(ctx, id, request)
	writeDockerResult(reqCtx, result, err)
}

func (c *Handler) DeleteDockerRegistry(ctx context.Context, reqCtx *app.RequestContext) {
	id, err := parsePathInt64(reqCtx, "id")
	if err != nil {
		response.Error(reqCtx, err)
		return
	}
	result, err := c.docker.DeleteRegistry(ctx, id)
	writeDockerResult(reqCtx, result, err)
}

func (c *Handler) TestDockerRegistry(ctx context.Context, reqCtx *app.RequestContext) {
	id, err := parsePathInt64(reqCtx, "id")
	if err != nil {
		response.Error(reqCtx, err)
		return
	}
	result, err := c.docker.TestRegistry(ctx, id)
	writeDockerResult(reqCtx, result, err)
}

func (c *Handler) GetDockerDaemonConfig(ctx context.Context, reqCtx *app.RequestContext) {
	result, err := c.docker.GetDaemonConfig(ctx)
	writeDockerResult(reqCtx, result, err)
}

func (c *Handler) ValidateDockerDaemonConfig(ctx context.Context, reqCtx *app.RequestContext) {
	var request dockerinfra.DockerDaemonConfigUpdateRequest
	if err := httpx.Bind(reqCtx, &request); err != nil {
		response.Error(reqCtx, err)
		return
	}
	result, err := c.docker.ValidateDaemonConfig(ctx, request)
	writeDockerResult(reqCtx, result, err)
}

func (c *Handler) SaveDockerDaemonConfig(ctx context.Context, reqCtx *app.RequestContext) {
	var request dockerinfra.DockerDaemonConfigUpdateRequest
	if err := httpx.Bind(reqCtx, &request); err != nil {
		response.Error(reqCtx, err)
		return
	}
	result, err := c.docker.SaveDaemonConfig(ctx, request)
	writeDockerResult(reqCtx, result, err)
}

func (c *Handler) RestartDockerDaemon(ctx context.Context, reqCtx *app.RequestContext) {
	result, err := c.docker.SubmitOperation(ctx, dockerinfra.OperationSubmitCommand{
		OperationType: dockerinfra.OperationTypeDaemonRestart,
		TargetType:    "daemon",
		TargetName:    "docker",
		Payload:       map[string]any{},
		Actor:         dockerActor(reqCtx),
	})
	writeDockerResult(reqCtx, result, err)
}

func (c *Handler) GetDockerRepositories(ctx context.Context, reqCtx *app.RequestContext) {
	id, err := parsePathInt64(reqCtx, "id")
	if err != nil {
		response.Error(reqCtx, err)
		return
	}
	result, err := c.docker.ListRepositories(
		ctx,
		id,
		parseQueryInt64WithDefault(reqCtx, "current", 1),
		parseQueryInt64WithDefault(reqCtx, "size", 20),
		strings.TrimSpace(string(reqCtx.Query("keyword"))),
	)
	writeDockerResult(reqCtx, result, err)
}

func (c *Handler) GetDockerRepositoryTags(ctx context.Context, reqCtx *app.RequestContext) {
	id, err := parsePathInt64(reqCtx, "id")
	if err != nil {
		response.Error(reqCtx, err)
		return
	}
	repository := repositoryPath(reqCtx)
	result, err := c.docker.ListTags(ctx, id, repository)
	writeDockerResult(reqCtx, result, err)
}

func (c *Handler) GetDockerRepositoryManifest(ctx context.Context, reqCtx *app.RequestContext) {
	id, err := parsePathInt64(reqCtx, "id")
	if err != nil {
		response.Error(reqCtx, err)
		return
	}
	repository := repositoryPath(reqCtx)
	reference, _ := url.PathUnescape(pathString(reqCtx, "reference"))
	result, err := c.docker.GetManifest(ctx, id, repository, reference)
	writeDockerResult(reqCtx, result, err)
}

func (c *Handler) ResolveDockerRepositoryDigest(ctx context.Context, reqCtx *app.RequestContext) {
	id, err := parsePathInt64(reqCtx, "id")
	if err != nil {
		response.Error(reqCtx, err)
		return
	}
	repository := repositoryPath(reqCtx)
	reference, _ := url.PathUnescape(pathString(reqCtx, "reference"))
	result, err := c.docker.ResolveDigest(ctx, id, repository, reference)
	writeDockerResult(reqCtx, map[string]string{"digest": result}, err)
}

func (c *Handler) SyncDockerRegistry(ctx context.Context, reqCtx *app.RequestContext) {
	id, err := parsePathInt64(reqCtx, "id")
	if err != nil {
		response.Error(reqCtx, err)
		return
	}
	result, err := c.docker.SubmitOperation(ctx, dockerinfra.OperationSubmitCommand{
		OperationType: dockerinfra.OperationTypeRegistrySync,
		TargetType:    "registry",
		TargetName:    pathString(reqCtx, "id"),
		Payload:       map[string]any{"registryId": id},
		Actor:         dockerActor(reqCtx),
	})
	writeDockerResult(reqCtx, result, err)
}

func (c *Handler) GetDockerVolumes(ctx context.Context, reqCtx *app.RequestContext) {
	result, err := c.docker.ListVolumes(ctx, parseQueryInt64WithDefault(reqCtx, "current", 1), parseQueryInt64WithDefault(reqCtx, "size", 10), strings.TrimSpace(string(reqCtx.Query("keyword"))))
	writeDockerResult(reqCtx, result, err)
}

func (c *Handler) GetDockerVolume(ctx context.Context, reqCtx *app.RequestContext) {
	result, err := c.docker.GetVolume(ctx, pathString(reqCtx, "name"))
	writeDockerResult(reqCtx, result, err)
}

func (c *Handler) CreateDockerVolume(ctx context.Context, reqCtx *app.RequestContext) {
	var request dockerinfra.DockerVolumeCreateRequest
	if err := httpx.Bind(reqCtx, &request); err != nil {
		response.Error(reqCtx, err)
		return
	}
	result, err := c.docker.CreateVolume(ctx, request)
	writeDockerResult(reqCtx, result, err)
}

func (c *Handler) DeleteDockerVolume(ctx context.Context, reqCtx *app.RequestContext) {
	var request dockerinfra.DockerVolumeDeleteRequest
	_ = httpx.Bind(reqCtx, &request)
	if queryBool(reqCtx, "force") {
		request.Force = true
	}
	result, err := c.docker.DeleteVolume(ctx, pathString(reqCtx, "name"), request)
	writeDockerResult(reqCtx, result, err)
}

func (c *Handler) PreviewDockerVolumePrune(ctx context.Context, reqCtx *app.RequestContext) {
	var request dockerinfra.CleanupPreviewRequest
	_ = httpx.Bind(reqCtx, &request)
	result, err := c.docker.PreviewVolumePrune(ctx, request)
	writeDockerResult(reqCtx, result, err)
}

func (c *Handler) ApplyDockerVolumePrune(ctx context.Context, reqCtx *app.RequestContext) {
	var request dockerinfra.CleanupApplyRequest
	_ = httpx.Bind(reqCtx, &request)
	result, err := c.docker.SubmitOperation(ctx, dockerinfra.OperationSubmitCommand{
		OperationType: dockerinfra.OperationTypeVolumePrune,
		TargetType:    "volume",
		TargetName:    "prune",
		Payload:       request,
		Actor:         dockerActor(reqCtx),
	})
	writeDockerResult(reqCtx, result, err)
}

func (c *Handler) GetDockerNetworks(ctx context.Context, reqCtx *app.RequestContext) {
	result, err := c.docker.ListNetworks(ctx, parseQueryInt64WithDefault(reqCtx, "current", 1), parseQueryInt64WithDefault(reqCtx, "size", 10), strings.TrimSpace(string(reqCtx.Query("keyword"))))
	writeDockerResult(reqCtx, result, err)
}

func (c *Handler) GetDockerNetwork(ctx context.Context, reqCtx *app.RequestContext) {
	result, err := c.docker.GetNetwork(ctx, pathString(reqCtx, "id"))
	writeDockerResult(reqCtx, result, err)
}

func (c *Handler) CreateDockerNetwork(ctx context.Context, reqCtx *app.RequestContext) {
	var request dockerinfra.DockerNetworkCreateRequest
	if err := httpx.Bind(reqCtx, &request); err != nil {
		response.Error(reqCtx, err)
		return
	}
	result, err := c.docker.CreateNetwork(ctx, request)
	writeDockerResult(reqCtx, result, err)
}

func (c *Handler) DeleteDockerNetwork(ctx context.Context, reqCtx *app.RequestContext) {
	result, err := c.docker.DeleteNetwork(ctx, pathString(reqCtx, "id"))
	writeDockerResult(reqCtx, result, err)
}

func (c *Handler) ConnectDockerNetwork(ctx context.Context, reqCtx *app.RequestContext) {
	var request dockerinfra.DockerNetworkConnectRequest
	if err := httpx.Bind(reqCtx, &request); err != nil {
		response.Error(reqCtx, err)
		return
	}
	result, err := c.docker.ConnectNetwork(ctx, pathString(reqCtx, "id"), request)
	writeDockerResult(reqCtx, result, err)
}

func (c *Handler) DisconnectDockerNetwork(ctx context.Context, reqCtx *app.RequestContext) {
	var request dockerinfra.DockerNetworkDisconnectRequest
	if err := httpx.Bind(reqCtx, &request); err != nil {
		response.Error(reqCtx, err)
		return
	}
	if queryBool(reqCtx, "force") {
		request.Force = true
	}
	result, err := c.docker.DisconnectNetwork(ctx, pathString(reqCtx, "id"), request)
	writeDockerResult(reqCtx, result, err)
}

func (c *Handler) PreviewDockerNetworkPrune(ctx context.Context, reqCtx *app.RequestContext) {
	var request dockerinfra.CleanupPreviewRequest
	_ = httpx.Bind(reqCtx, &request)
	result, err := c.docker.PreviewNetworkPrune(ctx, request)
	writeDockerResult(reqCtx, result, err)
}

func (c *Handler) ApplyDockerNetworkPrune(ctx context.Context, reqCtx *app.RequestContext) {
	var request dockerinfra.CleanupApplyRequest
	_ = httpx.Bind(reqCtx, &request)
	result, err := c.docker.SubmitOperation(ctx, dockerinfra.OperationSubmitCommand{
		OperationType: dockerinfra.OperationTypeNetworkPrune,
		TargetType:    "network",
		TargetName:    "prune",
		Payload:       request,
		Actor:         dockerActor(reqCtx),
	})
	writeDockerResult(reqCtx, result, err)
}

func (c *Handler) PreviewDockerImageCleanup(ctx context.Context, reqCtx *app.RequestContext) {
	var request dockerinfra.CleanupPreviewRequest
	_ = httpx.Bind(reqCtx, &request)
	result, err := c.docker.PreviewImageCleanup(ctx, request)
	writeDockerResult(reqCtx, result, err)
}

func (c *Handler) ApplyDockerImageCleanup(ctx context.Context, reqCtx *app.RequestContext) {
	var request dockerinfra.CleanupApplyRequest
	_ = httpx.Bind(reqCtx, &request)
	result, err := c.docker.SubmitOperation(ctx, dockerinfra.OperationSubmitCommand{OperationType: dockerinfra.OperationTypeImageCleanup, TargetType: "image", TargetName: "dangling", Payload: request, Actor: dockerActor(reqCtx)})
	writeDockerResult(reqCtx, result, err)
}

func (c *Handler) PreviewDockerContainerCleanup(ctx context.Context, reqCtx *app.RequestContext) {
	var request dockerinfra.CleanupPreviewRequest
	_ = httpx.Bind(reqCtx, &request)
	result, err := c.docker.PreviewContainerCleanup(ctx, request)
	writeDockerResult(reqCtx, result, err)
}

func (c *Handler) ApplyDockerContainerCleanup(ctx context.Context, reqCtx *app.RequestContext) {
	var request dockerinfra.CleanupApplyRequest
	_ = httpx.Bind(reqCtx, &request)
	result, err := c.docker.SubmitOperation(ctx, dockerinfra.OperationSubmitCommand{OperationType: dockerinfra.OperationTypeContainerCleanup, TargetType: "container", TargetName: "stopped", Payload: request, Actor: dockerActor(reqCtx)})
	writeDockerResult(reqCtx, result, err)
}

func (c *Handler) GetDockerOperations(ctx context.Context, reqCtx *app.RequestContext) {
	result, err := c.docker.ListOperations(ctx, parseQueryInt64WithDefault(reqCtx, "current", 1), parseQueryInt64WithDefault(reqCtx, "size", 20), strings.TrimSpace(string(reqCtx.Query("status"))), strings.TrimSpace(string(reqCtx.Query("operationType"))))
	writeDockerResult(reqCtx, result, err)
}

func (c *Handler) GetDockerOperation(ctx context.Context, reqCtx *app.RequestContext) {
	id, err := parsePathInt64(reqCtx, "operationId")
	if err != nil {
		response.Error(reqCtx, err)
		return
	}
	result, err := c.docker.GetOperation(ctx, id)
	writeDockerResult(reqCtx, result, err)
}

func (c *Handler) GetDockerOperationEvents(ctx context.Context, reqCtx *app.RequestContext) {
	id, err := parsePathInt64(reqCtx, "operationId")
	if err != nil {
		response.Error(reqCtx, err)
		return
	}
	result, err := c.docker.ListOperationEvents(ctx, id, parseQueryInt64WithDefault(reqCtx, "afterSequence", 0), int(parseQueryInt64WithDefault(reqCtx, "limit", 200)))
	writeDockerResult(reqCtx, result, err)
}

func (c *Handler) DiagnoseDockerOperationEventOrphans(ctx context.Context, reqCtx *app.RequestContext) {
	result, err := c.docker.DiagnoseOperationEventOrphans(
		ctx,
		parseQueryInt64WithDefault(reqCtx, "afterEventId", 0),
		int(parseQueryInt64WithDefault(reqCtx, "limit", 100)),
		dockerActor(reqCtx),
	)
	writeDockerResult(reqCtx, result, err)
}

func (c *Handler) CleanupDockerOperationEventOrphan(ctx context.Context, reqCtx *app.RequestContext) {
	eventID, err := parsePathInt64(reqCtx, "eventId")
	if err != nil {
		response.Error(reqCtx, err)
		return
	}
	var request dockerinfra.OperationEventOrphanCleanupRequest
	if err := httpx.Bind(reqCtx, &request); err != nil {
		response.Error(reqCtx, err)
		return
	}
	if request.EventID != eventID {
		response.Error(reqCtx, apperrors.Params("eventId 必须与路径中的精确事件一致"))
		return
	}
	result, err := c.docker.CleanupOperationEventOrphan(ctx, request, dockerActor(reqCtx))
	writeDockerResult(reqCtx, result, err)
}

func (c *Handler) GetLatestDockerOperation(ctx context.Context, reqCtx *app.RequestContext) {
	result, err := c.docker.LatestOperation(ctx, dockerinfra.LatestOperationQuery{
		TargetType:    strings.TrimSpace(string(reqCtx.Query("targetType"))),
		TargetName:    strings.TrimSpace(string(reqCtx.Query("targetName"))),
		TargetID:      strings.TrimSpace(string(reqCtx.Query("targetId"))),
		OperationType: strings.TrimSpace(string(reqCtx.Query("operationType"))),
	})
	writeDockerResult(reqCtx, result, err)
}

func (c *Handler) StreamDockerOperation(ctx context.Context, reqCtx *app.RequestContext) {
	id, err := parsePathInt64(reqCtx, "operationId")
	if err != nil {
		response.Error(reqCtx, err)
		return
	}
	stream, err := c.docker.StreamOperation(ctx, id, parseQueryInt64WithDefault(reqCtx, "afterSequence", 0))
	if err != nil {
		response.Error(reqCtx, err)
		return
	}
	writeSSEStream(ctx, reqCtx, stream)
}

func (c *Handler) CancelDockerOperation(ctx context.Context, reqCtx *app.RequestContext) {
	id, err := parsePathInt64(reqCtx, "operationId")
	if err != nil {
		response.Error(reqCtx, err)
		return
	}
	result, err := c.docker.CancelOperation(ctx, id, dockerActor(reqCtx))
	writeDockerResult(reqCtx, result, err)
}

func (c *Handler) RetryDockerOperation(ctx context.Context, reqCtx *app.RequestContext) {
	id, err := parsePathInt64(reqCtx, "operationId")
	if err != nil {
		response.Error(reqCtx, err)
		return
	}
	result, err := c.docker.RetryOperation(ctx, id, dockerActor(reqCtx))
	writeDockerResult(reqCtx, result, err)
}

func (c *Handler) DispatchDockerRepositoryResource(ctx context.Context, reqCtx *app.RequestContext) {
	id, err := parsePathInt64(reqCtx, "id")
	if err != nil {
		response.Error(reqCtx, err)
		return
	}
	raw := strings.TrimPrefix(pathString(reqCtx, "repository"), "/")
	if strings.HasSuffix(raw, "/tags") {
		repository := decodeDockerPath(strings.TrimSuffix(raw, "/tags"))
		result, err := c.docker.ListTags(ctx, id, repository)
		writeDockerResult(reqCtx, result, err)
		return
	}
	segments := strings.Split(raw, "/manifests/")
	if len(segments) == 2 {
		repository := decodeDockerPath(segments[0])
		reference := decodeDockerPath(segments[1])
		result, err := c.docker.GetManifest(ctx, id, repository, reference)
		writeDockerResult(reqCtx, result, err)
		return
	}
	response.Error(reqCtx, apperrors.NotFound("Docker registry repository resource 不存在"))
}

func writeDockerResult(reqCtx *app.RequestContext, value any, err error) {
	if err != nil {
		response.Error(reqCtx, err)
		return
	}
	response.Success(reqCtx, value)
}

func pathString(reqCtx *app.RequestContext, key string) string {
	if reqCtx == nil {
		return ""
	}
	return strings.TrimSpace(string(reqCtx.Param(key)))
}

func repositoryPath(reqCtx *app.RequestContext) string {
	raw := strings.TrimPrefix(pathString(reqCtx, "repository"), "/")
	return decodeDockerPath(raw)
}

func decodeDockerPath(raw string) string {
	decoded, err := url.PathUnescape(raw)
	if err != nil {
		return strings.TrimSpace(raw)
	}
	return strings.TrimSpace(decoded)
}

func queryBool(reqCtx *app.RequestContext, key string) bool {
	value := strings.TrimSpace(string(reqCtx.Query(key)))
	return value == "1" || strings.EqualFold(value, "true") || strings.EqualFold(value, "yes")
}

func (c *Handler) submitDockerIDOperation(ctx context.Context, reqCtx *app.RequestContext, operationType, targetType, id string) (*dockerinfra.OperationAcceptedVO, error) {
	if strings.EqualFold(targetType, "container") {
		if err := c.docker.ValidateContainerOperation(ctx, id, operationType); err != nil {
			return nil, err
		}
	}
	return c.docker.SubmitOperation(ctx, dockerinfra.OperationSubmitCommand{
		OperationType: operationType,
		TargetType:    targetType,
		TargetName:    id,
		Payload:       map[string]any{"id": id},
		Actor:         dockerActor(reqCtx),
	})
}

func dockerActor(reqCtx *app.RequestContext) dockerinfra.OperationActor {
	user := securitycontext.Require(reqCtx)
	return dockerinfra.OperationActor{
		UserID:      user.UserID,
		Username:    user.Username,
		IsAdmin:     user.IsAdmin,
		Permissions: append([]string(nil), user.Permissions...),
	}
}

func dockerLogQuery(reqCtx *app.RequestContext) (dockerinfra.ContainerLogQuery, error) {
	follow := false
	if value := strings.TrimSpace(string(reqCtx.Query("follow"))); value != "" {
		follow = value == "1" || strings.EqualFold(value, "true")
	}
	return dockerinfra.NormalizeContainerLogQuery(dockerinfra.ContainerLogQuery{
		Tail:       int(parseQueryInt64WithDefault(reqCtx, "tail", 200)),
		Since:      strings.TrimSpace(string(reqCtx.Query("since"))),
		Until:      strings.TrimSpace(string(reqCtx.Query("until"))),
		Timestamps: queryBool(reqCtx, "timestamps"),
		Grep:       strings.TrimSpace(string(reqCtx.Query("grep"))),
		Follow:     follow,
	})
}

func parseQueryUintWithDefault(reqCtx *app.RequestContext, key string, fallback uint) uint {
	value := strings.TrimSpace(string(reqCtx.Query(key)))
	if value == "" {
		return fallback
	}
	parsed, err := strconv.ParseUint(value, 10, 32)
	if err != nil {
		return fallback
	}
	return uint(parsed)
}

func writeSSEStream(ctx context.Context, reqCtx *app.RequestContext, stream io.ReadCloser) {
	adaptor.HertzHandler(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		var closeOnce sync.Once
		closeStream := func() {
			closeOnce.Do(func() {
				_ = stream.Close()
			})
		}
		defer closeStream()
		stopWatch := make(chan struct{})
		defer close(stopWatch)
		go func() {
			var closeNotify <-chan bool
			if notifier, ok := writer.(http.CloseNotifier); ok {
				closeNotify = notifier.CloseNotify()
			}
			select {
			case <-request.Context().Done():
				closeStream()
			case <-closeNotify:
				closeStream()
			case <-stopWatch:
			}
		}()
		flusher, ok := writer.(http.Flusher)
		if !ok {
			http.Error(writer, "streaming unsupported", http.StatusInternalServerError)
			return
		}
		writer.Header().Set("Content-Type", "text/event-stream")
		writer.Header().Set("Cache-Control", "no-cache")
		writer.Header().Set("Connection", "keep-alive")
		writer.WriteHeader(http.StatusOK)
		flusher.Flush()
		buffer := make([]byte, 4096)
		for {
			n, readErr := stream.Read(buffer)
			if n > 0 {
				if _, writeErr := writer.Write(buffer[:n]); writeErr != nil {
					return
				}
				flusher.Flush()
			}
			if readErr != nil {
				return
			}
		}
	}))(ctx, reqCtx)
}
