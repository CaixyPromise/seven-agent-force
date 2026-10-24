package docker

import (
	"context"
	"database/sql"
	"fmt"
	"io"
	"sort"
	"strings"
	"sync"
	"time"

	apperrors "github.com/CaixyPromise/seven-agent/force/internal/common/errors"
	"github.com/CaixyPromise/seven-agent/force/internal/infrastructure/datasource/store"
	secretvalueinfra "github.com/CaixyPromise/seven-agent/force/internal/infrastructure/security/crypto/secretvalue"
	"github.com/CaixyPromise/seven-agent/force/internal/shared/config"
	"github.com/CaixyPromise/seven-agent/force/internal/shared/xid"
	dockercontainer "github.com/docker/docker/api/types/container"
	dockerimage "github.com/docker/docker/api/types/image"
	dockernetwork "github.com/docker/docker/api/types/network"
	"github.com/docker/docker/api/types/strslice"
	dockerclient "github.com/docker/docker/client"
)

type service struct {
	cfg            config.DockerConfig
	originPatterns []string
	idGen          *xid.Generator
	client         *dockerclient.Client
	repo           *RegistryRepository
	operations     *OperationRepository
	projects       *ComposeProjectRepository
	secret         secretvalueinfra.Service
	registry       *registryHTTPClient
	compose        *composeRunner
	available      bool
	workerSem      chan struct{}
	queueSem       chan struct{}
	cancelMu       sync.Mutex
	cancels        map[int64]context.CancelFunc
}

func New(cfg config.DockerConfig, originPatterns []string, idGen *xid.Generator, provider store.Provider, secret secretvalueinfra.Service) (Service, error) {
	maxConcurrent := maxInt(cfg.Operation.MaxConcurrent, 4)
	maxQueued := cfg.Operation.MaxQueued
	if maxQueued <= 0 {
		maxQueued = maxConcurrent * 16
	}
	if maxQueued < maxConcurrent {
		maxQueued = maxConcurrent
	}
	svc := &service{
		cfg:            cfg,
		originPatterns: originPatterns,
		idGen:          idGen,
		secret:         secret,
		registry:       newRegistryHTTPClient(cfg.Registry.Timeout),
		compose:        newComposeRunner(cfg.Compose, cfg.Security),
		available:      cfg.Enabled,
		workerSem:      make(chan struct{}, maxConcurrent),
		queueSem:       make(chan struct{}, maxQueued),
		cancels:        map[int64]context.CancelFunc{},
	}
	if provider != nil && provider.SQLX() != nil {
		repo, err := NewRegistryRepository(provider)
		if err != nil {
			return nil, err
		}
		svc.repo = repo
		operations, err := NewOperationRepository(provider)
		if err != nil {
			return nil, err
		}
		svc.operations = operations
		projects, err := NewComposeProjectRepository(provider)
		if err != nil {
			return nil, err
		}
		svc.projects = projects
	}
	if !cfg.Enabled {
		return svc, nil
	}
	opts := []dockerclient.Opt{dockerclient.FromEnv}
	if cfg.Engine.Host != "" {
		opts = append(opts, dockerclient.WithHost(cfg.Engine.Host))
	}
	if cfg.Engine.APIVersion != "" {
		opts = append(opts, dockerclient.WithVersion(cfg.Engine.APIVersion))
	}
	if cfg.Engine.APIVersionNegotiation {
		opts = append(opts, dockerclient.WithAPIVersionNegotiation())
	}
	cli, err := dockerclient.NewClientWithOpts(opts...)
	if err != nil {
		return nil, fmt.Errorf("build docker client: %w", err)
	}
	svc.client = cli
	return svc, nil
}

func (s *service) requireClient() (*dockerclient.Client, error) {
	if s == nil || !s.available || s.client == nil {
		return nil, apperrors.Operation("Docker starter 未启用或 Docker daemon 不可用")
	}
	return s.client, nil
}

func (s *service) Enabled() bool {
	return s != nil && s.available
}

func (s *service) Ping(ctx context.Context) error {
	cli, err := s.requireClient()
	if err != nil {
		return err
	}
	runCtx, cancel := s.withTimeout(ctx)
	defer cancel()
	if _, err := cli.Ping(runCtx); err != nil {
		return fmt.Errorf("ping Docker daemon: %w", err)
	}
	return nil
}

func (s *service) Close() error {
	if s == nil {
		return nil
	}
	s.cancelMu.Lock()
	for _, cancel := range s.cancels {
		cancel()
	}
	s.cancels = map[int64]context.CancelFunc{}
	s.cancelMu.Unlock()
	if s.registry != nil {
		s.registry.CloseIdleConnections()
	}
	if s.client != nil {
		return s.client.Close()
	}
	return nil
}

func nullString(value string) sql.NullString {
	value = strings.TrimSpace(value)
	return sql.NullString{String: value, Valid: value != ""}
}

func nullInt64(value int64) sql.NullInt64 {
	return sql.NullInt64{Int64: value, Valid: value != 0}
}

func nullTimeValue(value time.Time) sql.NullTime {
	return sql.NullTime{Time: value, Valid: !value.IsZero()}
}

func (s *service) withTimeout(ctx context.Context) (context.Context, context.CancelFunc) {
	timeout := s.cfg.Engine.Timeout
	if timeout <= 0 {
		timeout = 30 * time.Second
	}
	return context.WithTimeout(ctx, timeout)
}

func (s *service) ListContainers(ctx context.Context, current, size int64, keyword, state string) (*PageResult[ContainerView], error) {
	cli, err := s.requireClient()
	if err != nil {
		return nil, err
	}
	runCtx, cancel := s.withTimeout(ctx)
	defer cancel()
	containers, err := cli.ContainerList(runCtx, dockercontainer.ListOptions{All: true})
	if err != nil {
		return nil, apperrors.Operation("获取 Docker 容器列表失败：" + err.Error())
	}
	views := make([]ContainerView, 0, len(containers))
	for _, item := range containers {
		view := s.toContainerView(item, nil)
		if matchContainer(view, keyword, state) {
			views = append(views, view)
		}
	}
	sort.SliceStable(views, func(i, j int) bool { return views[i].Created > views[j].Created })
	return paginate(views, current, size), nil
}

func (s *service) GetContainer(ctx context.Context, id string) (*ContainerDetailView, error) {
	cli, resolved, summary, err := s.resolveContainer(ctx, id)
	if err != nil {
		return nil, err
	}
	runCtx, cancel := s.withTimeout(ctx)
	defer cancel()
	inspect, err := cli.ContainerInspect(runCtx, resolved)
	if err != nil {
		return nil, apperrors.Operation("获取 Docker 容器详情失败：" + err.Error())
	}
	return &ContainerDetailView{Container: s.toContainerView(summary, &inspect), Inspect: normalizeInspect(inspect)}, nil
}

func (s *service) GetContainerLogs(ctx context.Context, id string, tail int) (string, error) {
	return s.GetContainerLogsQuery(ctx, id, ContainerLogQuery{Tail: tail})
}

func (s *service) StartContainer(ctx context.Context, id string) (bool, error) {
	cli, resolved, _, err := s.resolveContainer(ctx, id)
	if err != nil {
		return false, err
	}
	if err := s.ValidateContainerOperation(ctx, resolved, OperationTypeContainerStart); err != nil {
		return false, err
	}
	runCtx, cancel := s.withTimeout(ctx)
	defer cancel()
	if err := cli.ContainerStart(runCtx, resolved, dockercontainer.StartOptions{}); err != nil {
		return false, apperrors.Operation("启动容器失败：" + err.Error())
	}
	return true, nil
}

func (s *service) StopContainer(ctx context.Context, id string) (bool, error) {
	cli, resolved, _, err := s.resolveContainer(ctx, id)
	if err != nil {
		return false, err
	}
	if err := s.ValidateContainerOperation(ctx, resolved, OperationTypeContainerStop); err != nil {
		return false, err
	}
	runCtx, cancel := s.withTimeout(ctx)
	defer cancel()
	if err := cli.ContainerStop(runCtx, resolved, dockercontainer.StopOptions{}); err != nil {
		return false, apperrors.Operation("停止容器失败：" + err.Error())
	}
	return true, nil
}

func (s *service) RestartContainer(ctx context.Context, id string) (bool, error) {
	cli, resolved, _, err := s.resolveContainer(ctx, id)
	if err != nil {
		return false, err
	}
	if err := s.ValidateContainerOperation(ctx, resolved, OperationTypeContainerRestart); err != nil {
		return false, err
	}
	runCtx, cancel := s.withTimeout(ctx)
	defer cancel()
	if err := cli.ContainerRestart(runCtx, resolved, dockercontainer.StopOptions{}); err != nil {
		return false, apperrors.Operation("重启容器失败：" + err.Error())
	}
	return true, nil
}

func (s *service) DeleteContainer(ctx context.Context, id string) (bool, error) {
	cli, resolved, _, err := s.resolveContainer(ctx, id)
	if err != nil {
		return false, err
	}
	if err := s.ValidateContainerOperation(ctx, resolved, OperationTypeContainerDelete); err != nil {
		return false, err
	}
	runCtx, cancel := s.withTimeout(ctx)
	defer cancel()
	if err := cli.ContainerRemove(runCtx, resolved, dockercontainer.RemoveOptions{Force: true}); err != nil {
		return false, apperrors.Operation("删除容器失败：" + err.Error())
	}
	return true, nil
}

func (s *service) ListImages(ctx context.Context, current, size int64, keyword string) (*PageResult[ImageView], error) {
	cli, err := s.requireClient()
	if err != nil {
		return nil, err
	}
	runCtx, cancel := s.withTimeout(ctx)
	defer cancel()
	containers, _ := cli.ContainerList(runCtx, dockercontainer.ListOptions{All: true})
	usage := buildImageUsage(containers)
	images, err := cli.ImageList(runCtx, dockerimage.ListOptions{All: true})
	if err != nil {
		return nil, apperrors.Operation("获取 Docker 镜像列表失败：" + err.Error())
	}
	views := make([]ImageView, 0, len(images))
	for _, item := range images {
		view := ImageView{
			ImageID:              stripSHA(item.ID),
			RepoTags:             safeStrings(item.RepoTags),
			RepoDigests:          safeStrings(item.RepoDigests),
			Size:                 item.Size,
			Created:              item.Created,
			Labels:               safeLabels(item.Labels),
			UsedByContainerCount: usage[stripSHA(item.ID)],
		}
		if matchImage(view, keyword) {
			views = append(views, view)
		}
	}
	sort.SliceStable(views, func(i, j int) bool { return views[i].Created > views[j].Created })
	return paginate(views, current, size), nil
}

func (s *service) GetImage(ctx context.Context, id string) (*ImageDetailView, error) {
	cli, resolved, summary, err := s.resolveImage(ctx, id)
	if err != nil {
		return nil, err
	}
	runCtx, cancel := s.withTimeout(ctx)
	defer cancel()
	inspect, err := cli.ImageInspect(runCtx, resolved)
	if err != nil {
		return nil, apperrors.Operation("获取 Docker 镜像详情失败：" + err.Error())
	}
	containers, _ := cli.ContainerList(runCtx, dockercontainer.ListOptions{All: true})
	usage := buildImageUsage(containers)
	view := ImageView{
		ImageID:              stripSHA(summary.ID),
		RepoTags:             safeStrings(summary.RepoTags),
		RepoDigests:          safeStrings(summary.RepoDigests),
		Size:                 summary.Size,
		Created:              summary.Created,
		Labels:               safeLabels(summary.Labels),
		UsedByContainerCount: usage[stripSHA(summary.ID)],
	}
	return &ImageDetailView{
		ImageID:              view.ImageID,
		RepoTags:             view.RepoTags,
		RepoDigests:          view.RepoDigests,
		Size:                 view.Size,
		Created:              view.Created,
		Labels:               view.Labels,
		UsedByContainerCount: view.UsedByContainerCount,
		Inspect:              normalizeInspect(inspect),
	}, nil
}

func (s *service) ListImageContainers(ctx context.Context, imageID string) ([]ContainerUsageView, error) {
	cli, err := s.requireClient()
	if err != nil {
		return nil, err
	}
	target := stripSHA(imageID)
	runCtx, cancel := s.withTimeout(ctx)
	defer cancel()
	containers, err := cli.ContainerList(runCtx, dockercontainer.ListOptions{All: true})
	if err != nil {
		return nil, apperrors.Operation("获取 Docker 容器列表失败：" + err.Error())
	}
	result := make([]ContainerUsageView, 0)
	for _, item := range containers {
		if stripSHA(item.ImageID) == target {
			result = append(result, ContainerUsageView{ID: stripSHA(item.ID), Name: firstContainerName(item), Image: item.Image, State: string(item.State), Status: item.Status})
		}
	}
	return result, nil
}

func (s *service) PullImage(ctx context.Context, command ImagePullCommand) (bool, error) {
	repository := strings.TrimSpace(command.Repository)
	if repository == "" {
		return false, apperrors.Params("repository 不能为空")
	}
	auth := ""
	if command.RegistryID > 0 {
		rt, err := s.registryRuntime(ctx, command.RegistryID)
		if err != nil {
			return false, err
		}
		repository = qualifyRegistryRepository(rt, repository)
		auth, err = registryAuthPayload(rt)
		if err != nil {
			return false, err
		}
	}
	ref := repository
	if command.Tag != "" {
		ref += ":" + strings.TrimSpace(command.Tag)
	}
	cli, err := s.requireClient()
	if err != nil {
		return false, err
	}
	runCtx, cancel := s.withTimeout(ctx)
	defer cancel()
	reader, err := cli.ImagePull(runCtx, ref, dockerimage.PullOptions{RegistryAuth: auth})
	if err != nil {
		return false, apperrors.Operation("拉取镜像失败：" + err.Error())
	}
	defer reader.Close()
	_, _ = io.Copy(io.Discard, reader)
	return true, nil
}

func (s *service) TagImage(ctx context.Context, command ImageTagCommand) (bool, error) {
	source := strings.TrimSpace(command.SourceImage)
	repo := strings.TrimSpace(command.TargetRepository)
	tag := strings.TrimSpace(command.TargetTag)
	if source == "" || repo == "" || tag == "" {
		return false, apperrors.Params("镜像打标签参数不完整")
	}
	cli, err := s.requireClient()
	if err != nil {
		return false, err
	}
	runCtx, cancel := s.withTimeout(ctx)
	defer cancel()
	if err := cli.ImageTag(runCtx, source, repo+":"+tag); err != nil {
		return false, apperrors.Operation("镜像打标签失败：" + err.Error())
	}
	return true, nil
}

func (s *service) PushImage(ctx context.Context, command ImagePushCommand) (bool, error) {
	source := strings.TrimSpace(command.SourceImage)
	if source == "" {
		return false, apperrors.Params("sourceImage 不能为空")
	}
	ref := source
	if strings.TrimSpace(command.TargetRepository) != "" {
		tag := strings.TrimSpace(command.TargetTag)
		if tag == "" {
			return false, apperrors.Params("targetTag 不能为空")
		}
		targetRepository := strings.TrimSpace(command.TargetRepository)
		if command.RegistryID > 0 {
			rt, err := s.registryRuntime(ctx, command.RegistryID)
			if err != nil {
				return false, err
			}
			targetRepository = qualifyRegistryRepository(rt, targetRepository)
		}
		ref = targetRepository + ":" + tag
		if _, err := s.TagImage(ctx, ImageTagCommand{SourceImage: source, TargetRepository: targetRepository, TargetTag: tag}); err != nil {
			return false, err
		}
	}
	auth, err := s.registryAuth(ctx, command.RegistryID)
	if err != nil {
		return false, err
	}
	cli, err := s.requireClient()
	if err != nil {
		return false, err
	}
	runCtx, cancel := s.withTimeout(ctx)
	defer cancel()
	reader, err := cli.ImagePush(runCtx, ref, dockerimage.PushOptions{RegistryAuth: auth})
	if err != nil {
		return false, apperrors.Operation("推送镜像失败：" + err.Error())
	}
	defer reader.Close()
	_, _ = io.Copy(io.Discard, reader)
	return true, nil
}

func (s *service) DeleteImage(ctx context.Context, id string) (bool, error) {
	cli, resolved, _, err := s.resolveImage(ctx, id)
	if err != nil {
		return false, err
	}
	runCtx, cancel := s.withTimeout(ctx)
	defer cancel()
	if _, err := cli.ImageRemove(runCtx, resolved, dockerimage.RemoveOptions{Force: false}); err != nil {
		return false, apperrors.Operation("删除镜像失败：" + err.Error())
	}
	return true, nil
}

func (s *service) PullRemoteImage(ctx context.Context, command RemoteImagePullRequest) (bool, error) {
	if command.RegistryID <= 0 || strings.TrimSpace(command.Repository) == "" || strings.TrimSpace(command.Tag) == "" {
		return false, apperrors.Params("远程镜像拉取参数不完整")
	}
	return s.PullImage(ctx, ImagePullCommand{Repository: command.Repository, Tag: command.Tag, RegistryID: command.RegistryID})
}

func (s *service) StartupPreview(ctx context.Context, imageID string) (*ImageStartupPreview, error) {
	_, _, summary, err := s.resolveImage(ctx, imageID)
	if err != nil {
		return nil, err
	}
	cli, err := s.requireClient()
	if err != nil {
		return nil, err
	}
	runCtx, cancel := s.withTimeout(ctx)
	defer cancel()
	inspect, err := cli.ImageInspect(runCtx, summary.ID)
	if err != nil {
		return nil, apperrors.Operation("获取镜像启动预览失败：" + err.Error())
	}
	imageRef := firstNonBlank(firstString(inspect.RepoTags), firstString(summary.RepoTags), stripSHA(inspect.ID))
	serviceName := suggestServiceName(imageRef)
	env := envCommands(inspect.Config.Env)
	labels := labelCommands(inspect.Config.Labels)
	ports := exposedPortsFromConfig(inspect.Config.ExposedPorts)
	volumes := volumesFromConfig(inspect.Config.Volumes)
	entrypoint := []string(inspect.Config.Entrypoint)
	command := []string(inspect.Config.Cmd)
	preview := &ImageStartupPreview{
		ImageID:              stripSHA(inspect.ID),
		ImageReference:       imageRef,
		DefaultContainerName: serviceName,
		DefaultServiceName:   serviceName,
		DefaultProjectName:   fmt.Sprintf("%s-%d", serviceName, time.Now().Unix()),
		OS:                   inspect.Os,
		Architecture:         inspect.Architecture,
		WorkingDir:           inspect.Config.WorkingDir,
		User:                 inspect.Config.User,
		Entrypoint:           entrypoint,
		Command:              command,
		Environment:          env,
		PortBindings:         ports,
		VolumeBindings:       volumes,
		Labels:               labels,
		TTY:                  false,
		StdinOpen:            false,
		PublishAllPorts:      false,
	}
	preview.SuggestedComposeYaml = buildComposeYaml(serviceName, imageRef, env, ports, volumes, labels, preview.WorkingDir, preview.User, entrypoint, command, preview.TTY, preview.StdinOpen, "", false, "always", false, nil, nil, "")
	return preview, nil
}

func (s *service) CreateContainerFromImage(ctx context.Context, command ContainerCreateRequest) (string, error) {
	imageRef, err := s.resolveImageReference(ctx, command.ImageID, command.ImageReference)
	if err != nil {
		return "", err
	}
	if err := s.validateContainerCreate(command); err != nil {
		return "", err
	}
	cfg := &dockercontainer.Config{
		Image:        imageRef,
		Env:          envStrings(command.Environment),
		Labels:       keyValueMap(command.Labels),
		Tty:          command.TTY,
		OpenStdin:    command.StdinOpen,
		WorkingDir:   strings.TrimSpace(command.WorkingDir),
		User:         strings.TrimSpace(command.User),
		Entrypoint:   strslice.StrSlice(trimValues(command.Entrypoint)),
		Cmd:          strslice.StrSlice(trimValues(command.Command)),
		ExposedPorts: portSet(command.PortBindings),
	}
	hostCfg := &dockercontainer.HostConfig{
		Binds:           bindStrings(command.VolumeBindings),
		NetworkMode:     dockercontainer.NetworkMode(strings.TrimSpace(command.NetworkMode)),
		PortBindings:    portMap(command.PortBindings),
		Privileged:      command.Privileged,
		AutoRemove:      command.AutoRemove,
		PublishAllPorts: command.PublishAllPorts,
		CapAdd:          strslice.StrSlice(trimValues(command.CapAdd)),
		CapDrop:         strslice.StrSlice(trimValues(command.CapDrop)),
		RestartPolicy:   restartPolicy(command.RestartPolicy, command.RestartMaxRetryCount),
	}
	applyLimits(hostCfg, command.ResourceLimits)
	cli, err := s.requireClient()
	if err != nil {
		return "", err
	}
	runCtx, cancel := s.withTimeout(ctx)
	defer cancel()
	resp, err := cli.ContainerCreate(runCtx, cfg, hostCfg, &dockernetwork.NetworkingConfig{}, nil, strings.TrimSpace(command.ContainerName))
	if err != nil {
		return "", apperrors.Operation("创建容器失败：" + err.Error())
	}
	if err := cli.ContainerStart(runCtx, resp.ID, dockercontainer.StartOptions{}); err != nil {
		return "", apperrors.Operation("启动容器失败：" + err.Error())
	}
	return stripSHA(resp.ID), nil
}

func (s *service) validateContainerCreate(command ContainerCreateRequest) error {
	if !s.cfg.Security.StrictContainerCreate {
		return nil
	}
	networkMode := strings.ToLower(strings.TrimSpace(command.NetworkMode))
	if command.Privileged || networkMode == "host" {
		return apperrors.Forbidden("当前 Docker 安全策略不允许 privileged 或 host network")
	}
	for _, item := range command.VolumeBindings {
		if strings.TrimSpace(item.Source) != "" && (strings.HasPrefix(strings.TrimSpace(item.Source), "/") || strings.EqualFold(item.Type, "bind")) {
			return apperrors.Forbidden("当前 Docker 安全策略不允许 bind mount")
		}
	}
	return nil
}

func (s *service) ValidateCompose(ctx context.Context, command ComposeUpRequest) (*ComposeValidationView, error) {
	return s.compose.Validate(ctx, command)
}

func (s *service) UpCompose(ctx context.Context, command ComposeUpRequest) (bool, error) {
	return s.compose.Up(ctx, command)
}

func (s *service) ExportCompose(ctx context.Context, containerID string) (*ComposeExportView, error) {
	cli, resolved, _, err := s.resolveContainer(ctx, containerID)
	if err != nil {
		return nil, err
	}
	runCtx, cancel := s.withTimeout(ctx)
	defer cancel()
	inspect, err := cli.ContainerInspect(runCtx, resolved)
	if err != nil {
		return nil, apperrors.Operation("导出 compose 失败：" + err.Error())
	}
	labels := map[string]string{}
	if inspect.Config != nil && inspect.Config.Labels != nil {
		labels = inspect.Config.Labels
	}
	source := "reconstructed"
	if labels["com.docker.compose.project.config_files"] != "" {
		source = "compose_labels_reconstructed"
	}
	yaml := buildComposeFromInspect(inspect)
	return &ComposeExportView{
		ContainerID:    stripSHA(inspect.ID),
		ComposeManaged: labels["com.docker.compose.project"] != "",
		ComposeProject: firstNonBlank(labels["com.docker.compose.project"], labels["com.docker.compose.project.working_dir"]),
		ComposeService: firstNonBlank(labels["com.docker.compose.service"], strings.TrimPrefix(inspect.Name, "/")),
		ComposeYaml:    yaml,
		Source:         source,
	}, nil
}
