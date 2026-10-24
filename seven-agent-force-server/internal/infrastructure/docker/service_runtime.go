package docker

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"sort"
	"strings"
	"time"

	apperrors "github.com/CaixyPromise/seven-agent/force/internal/common/errors"
	"github.com/CaixyPromise/seven-agent/force/internal/shared/config"
	dockercontainer "github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/filters"
	dockerimage "github.com/docker/docker/api/types/image"
	dockernetwork "github.com/docker/docker/api/types/network"
	dockervolume "github.com/docker/docker/api/types/volume"
)

func (s *service) GetContainerStats(ctx context.Context, id string) (map[string]any, error) {
	cli, resolved, _, err := s.resolveContainer(ctx, id)
	if err != nil {
		return nil, err
	}
	runCtx, cancel := s.withTimeout(ctx)
	defer cancel()
	result, err := cli.ContainerStatsOneShot(runCtx, resolved)
	if err != nil {
		return nil, apperrors.Operation("读取 Docker 容器资源统计失败：" + err.Error())
	}
	defer result.Body.Close()
	var payload map[string]any
	if err := json.NewDecoder(io.LimitReader(result.Body, 2<<20)).Decode(&payload); err != nil {
		return nil, apperrors.Operation("解析 Docker 容器资源统计失败：" + err.Error())
	}
	return sanitizeMap(payload, s.cfg.Security), nil
}

func (s *service) ExportImage(ctx context.Context, id string) (map[string]any, error) {
	cli, resolved, summary, err := s.resolveImage(ctx, id)
	if err != nil {
		return nil, err
	}
	runCtx, cancel := s.withTimeout(ctx)
	defer cancel()
	reader, err := cli.ImageSave(runCtx, []string{resolved})
	if err != nil {
		return nil, apperrors.Operation("导出 Docker 镜像失败：" + err.Error())
	}
	defer reader.Close()
	written, err := io.Copy(io.Discard, io.LimitReader(reader, 16<<30))
	if err != nil {
		return nil, apperrors.Operation("读取 Docker 镜像导出流失败：" + err.Error())
	}
	return map[string]any{
		"imageId":     stripSHA(summary.ID),
		"repoTags":    safeStrings(summary.RepoTags),
		"repoDigests": safeStrings(summary.RepoDigests),
		"tarBytes":    written,
		"note":        "image export stream consumed by operation runtime; use future artifact storage for binary download",
	}, nil
}

func (s *service) PreviewCompose(ctx context.Context, actor OperationActor, command ComposeUpRequest) (*ComposePreviewView, error) {
	decision := evaluatePolicyWithMode(s.cfg.Security, actor, OperationTypeComposeUp, command, true)
	spec, _ := parseComposePolicySpec(command.ComposeYaml)
	preview := PreviewVO{
		Safe:              len(decision.Violations) == 0,
		Warnings:          decision.Warnings,
		Violations:        decision.Violations,
		AffectedResources: composeServiceNames(spec),
		NormalizedSpec:    composeNormalizedSpec(spec),
	}
	validation, err := s.ValidateCompose(ctx, command)
	if err != nil {
		return nil, err
	}
	preview.NormalizedSpec = validation.NormalizedYaml
	return &ComposePreviewView{
		Preview:        preview,
		Validation:     *validation,
		Services:       composeServiceNames(spec),
		NormalizedYaml: validation.NormalizedYaml,
	}, nil
}

func (s *service) DownCompose(ctx context.Context, command ComposeUpRequest) (bool, error) {
	return s.compose.Down(ctx, command)
}

func (s *service) RestartCompose(ctx context.Context, command ComposeUpRequest) (bool, error) {
	return s.compose.Restart(ctx, command)
}

func (s *service) ComposePS(ctx context.Context, command ComposeUpRequest) (*ComposePSView, error) {
	return s.compose.PS(ctx, command)
}

func (s *service) ComposeLogs(ctx context.Context, command ComposeUpRequest, tail int) (string, error) {
	return s.compose.Logs(ctx, command, tail)
}

func (s *service) ResolveDigest(ctx context.Context, id int64, repository, reference string) (string, error) {
	manifest, err := s.GetManifest(ctx, id, repository, reference)
	if err != nil {
		return "", err
	}
	if manifest == nil || strings.TrimSpace(manifest.Digest) == "" {
		return "", apperrors.Operation("远程镜像 digest 不可用")
	}
	return manifest.Digest, nil
}

func (s *service) ListVolumes(ctx context.Context, current, size int64, keyword string) (*PageResult[DockerResourceView], error) {
	cli, err := s.requireClient()
	if err != nil {
		return nil, err
	}
	runCtx, cancel := s.withTimeout(ctx)
	defer cancel()
	resp, err := cli.VolumeList(runCtx, dockervolume.ListOptions{})
	if err != nil {
		return nil, apperrors.Operation("查询 Docker volume 失败：" + err.Error())
	}
	items := make([]DockerResourceView, 0, len(resp.Volumes))
	for _, volume := range resp.Volumes {
		view := DockerResourceView{ID: volume.Name, Name: volume.Name, Driver: volume.Driver, Scope: volume.Scope, Labels: safeLabels(volume.Labels)}
		if volume.CreatedAt != "" {
			view.CreatedAt = volume.CreatedAt
		}
		if matchResource(view, keyword) {
			items = append(items, view)
		}
	}
	sort.SliceStable(items, func(i, j int) bool { return items[i].Name < items[j].Name })
	return paginate(items, current, size), nil
}

func (s *service) ListNetworks(ctx context.Context, current, size int64, keyword string) (*PageResult[DockerResourceView], error) {
	cli, err := s.requireClient()
	if err != nil {
		return nil, err
	}
	runCtx, cancel := s.withTimeout(ctx)
	defer cancel()
	resp, err := cli.NetworkList(runCtx, dockernetwork.ListOptions{})
	if err != nil {
		return nil, apperrors.Operation("查询 Docker network 失败：" + err.Error())
	}
	items := make([]DockerResourceView, 0, len(resp))
	for _, network := range resp {
		view := networkResourceView(network)
		if matchResource(view, keyword) {
			items = append(items, view)
		}
	}
	sort.SliceStable(items, func(i, j int) bool { return items[i].Name < items[j].Name })
	return paginate(items, current, size), nil
}

func (s *service) GetNetwork(ctx context.Context, id string) (*DockerNetworkDetailView, error) {
	id, err := normalizeDockerResourceID(id, "network")
	if err != nil {
		return nil, err
	}
	cli, err := s.requireClient()
	if err != nil {
		return nil, err
	}
	runCtx, cancel := s.withTimeout(ctx)
	defer cancel()
	inspect, err := cli.NetworkInspect(runCtx, id, dockernetwork.InspectOptions{})
	if err != nil {
		return nil, apperrors.Operation("获取 Docker network 详情失败：" + err.Error())
	}
	return &DockerNetworkDetailView{
		Resource:   networkResourceView(inspect),
		Inspect:    normalizeInspect(inspect),
		Containers: networkEndpointMap(inspect.Containers),
		Options:    inspect.Options,
	}, nil
}

func (s *service) CreateNetwork(ctx context.Context, request DockerNetworkCreateRequest) (*DockerResourceView, error) {
	request, err := request.Normalize()
	if err != nil {
		return nil, err
	}
	cli, err := s.requireClient()
	if err != nil {
		return nil, err
	}
	options := dockernetwork.CreateOptions{
		Driver:     request.Driver,
		Scope:      request.Scope,
		EnableIPv4: request.EnableIPv4,
		EnableIPv6: request.EnableIPv6,
		Internal:   request.Internal,
		Attachable: request.Attachable,
		Ingress:    request.Ingress,
		Options:    request.Options,
		Labels:     request.Labels,
	}
	if request.IPAMDriver != "" || len(request.IPAMOptions) > 0 || len(request.IPAMConfigs) > 0 {
		ipam := &dockernetwork.IPAM{Driver: request.IPAMDriver, Options: request.IPAMOptions}
		for _, item := range request.IPAMConfigs {
			ipam.Config = append(ipam.Config, dockernetwork.IPAMConfig{
				Subnet:     item.Subnet,
				IPRange:    item.IPRange,
				Gateway:    item.Gateway,
				AuxAddress: item.AuxAddress,
			})
		}
		options.IPAM = ipam
	}
	runCtx, cancel := s.withTimeout(ctx)
	defer cancel()
	created, err := cli.NetworkCreate(runCtx, request.Name, options)
	if err != nil {
		return nil, apperrors.Operation("创建 Docker network 失败：" + err.Error())
	}
	view := DockerResourceView{ID: created.ID, Name: request.Name, Driver: request.Driver, Scope: request.Scope, Labels: safeLabels(request.Labels)}
	if created.Warning != "" {
		view.Description = created.Warning
	}
	return &view, nil
}

func (s *service) DeleteNetwork(ctx context.Context, id string) (bool, error) {
	id, err := normalizeDockerResourceID(id, "network")
	if err != nil {
		return false, err
	}
	cli, err := s.requireClient()
	if err != nil {
		return false, err
	}
	runCtx, cancel := s.withTimeout(ctx)
	defer cancel()
	if err := cli.NetworkRemove(runCtx, id); err != nil {
		return false, apperrors.Operation("删除 Docker network 失败：" + err.Error())
	}
	return true, nil
}

func (s *service) ConnectNetwork(ctx context.Context, id string, request DockerNetworkConnectRequest) (bool, error) {
	request, id, err := request.Normalize(id)
	if err != nil {
		return false, err
	}
	cli, err := s.requireClient()
	if err != nil {
		return false, err
	}
	runCtx, cancel := s.withTimeout(ctx)
	defer cancel()
	if err := cli.NetworkConnect(runCtx, id, request.ContainerID, networkEndpointSettings(request)); err != nil {
		return false, apperrors.Operation("连接 Docker network 失败：" + err.Error())
	}
	return true, nil
}

func (s *service) DisconnectNetwork(ctx context.Context, id string, request DockerNetworkDisconnectRequest) (bool, error) {
	request, id, err := request.Normalize(id)
	if err != nil {
		return false, err
	}
	cli, err := s.requireClient()
	if err != nil {
		return false, err
	}
	runCtx, cancel := s.withTimeout(ctx)
	defer cancel()
	if err := cli.NetworkDisconnect(runCtx, id, request.ContainerID, request.Force); err != nil {
		return false, apperrors.Operation("断开 Docker network 失败：" + err.Error())
	}
	return true, nil
}

func (s *service) PreviewNetworkPrune(ctx context.Context, request CleanupPreviewRequest) (*DockerResourcePrunePreview, error) {
	_ = request
	resources, err := s.previewNetworkPruneResources(ctx)
	if err != nil {
		return nil, err
	}
	preview := newDockerResourcePrunePreview("network", resources, 0)
	return &preview, nil
}

func (s *service) previewNetworkPruneResources(ctx context.Context) ([]string, error) {
	cli, err := s.requireClient()
	if err != nil {
		return nil, err
	}
	runCtx, cancel := s.withTimeout(ctx)
	defer cancel()
	networks, err := cli.NetworkList(runCtx, dockernetwork.ListOptions{Filters: filters.NewArgs(filters.Arg("dangling", "true"))})
	if err != nil {
		return nil, apperrors.Operation("预览 Docker network prune 失败：" + err.Error())
	}
	resources := make([]string, 0, len(networks))
	for _, item := range networks {
		resources = append(resources, firstNonBlank(item.Name, item.ID))
	}
	sort.Strings(resources)
	return resources, nil
}

func (s *service) ApplyNetworkPrune(ctx context.Context, request CleanupApplyRequest) (map[string]any, error) {
	cli, err := s.requireClient()
	if err != nil {
		return nil, err
	}
	resources, err := s.previewNetworkPruneResources(ctx)
	if err != nil {
		return nil, err
	}
	if err := requireCleanupToken("network", resources, request.PreviewToken); err != nil {
		return nil, err
	}
	runCtx, cancel := s.withTimeout(ctx)
	defer cancel()
	report, err := cli.NetworksPrune(runCtx, filters.NewArgs())
	if err != nil {
		return nil, apperrors.Operation("执行 Docker network prune 失败：" + err.Error())
	}
	return map[string]any{"deleted": report.NetworksDeleted, "spaceReclaimed": int64(0)}, nil
}

func (s *service) GetVolume(ctx context.Context, name string) (*DockerVolumeDetailView, error) {
	name, err := normalizeDockerResourceID(name, "volume")
	if err != nil {
		return nil, err
	}
	cli, err := s.requireClient()
	if err != nil {
		return nil, err
	}
	runCtx, cancel := s.withTimeout(ctx)
	defer cancel()
	inspect, err := cli.VolumeInspect(runCtx, name)
	if err != nil {
		return nil, apperrors.Operation("获取 Docker volume 详情失败：" + err.Error())
	}
	return &DockerVolumeDetailView{
		Resource:   volumeResourceView(inspect),
		Inspect:    normalizeInspect(inspect),
		Mountpoint: inspect.Mountpoint,
		Options:    inspect.Options,
		Status:     inspect.Status,
	}, nil
}

func (s *service) CreateVolume(ctx context.Context, request DockerVolumeCreateRequest) (*DockerResourceView, error) {
	request, err := request.Normalize()
	if err != nil {
		return nil, err
	}
	cli, err := s.requireClient()
	if err != nil {
		return nil, err
	}
	runCtx, cancel := s.withTimeout(ctx)
	defer cancel()
	created, err := cli.VolumeCreate(runCtx, dockervolume.CreateOptions{
		Name:       request.Name,
		Driver:     request.Driver,
		DriverOpts: request.DriverOpts,
		Labels:     request.Labels,
	})
	if err != nil {
		return nil, apperrors.Operation("创建 Docker volume 失败：" + err.Error())
	}
	view := volumeResourceView(created)
	return &view, nil
}

func (s *service) DeleteVolume(ctx context.Context, name string, request DockerVolumeDeleteRequest) (bool, error) {
	name, err := normalizeDockerResourceID(name, "volume")
	if err != nil {
		return false, err
	}
	cli, err := s.requireClient()
	if err != nil {
		return false, err
	}
	runCtx, cancel := s.withTimeout(ctx)
	defer cancel()
	if err := cli.VolumeRemove(runCtx, name, request.Force); err != nil {
		return false, apperrors.Operation("删除 Docker volume 失败：" + err.Error())
	}
	return true, nil
}

func (s *service) PreviewVolumePrune(ctx context.Context, request CleanupPreviewRequest) (*DockerResourcePrunePreview, error) {
	_ = request
	resources, bytes, err := s.previewVolumePruneResources(ctx)
	if err != nil {
		return nil, err
	}
	preview := newDockerResourcePrunePreview("volume", resources, bytes)
	return &preview, nil
}

func (s *service) previewVolumePruneResources(ctx context.Context) ([]string, int64, error) {
	cli, err := s.requireClient()
	if err != nil {
		return nil, 0, err
	}
	runCtx, cancel := s.withTimeout(ctx)
	defer cancel()
	resp, err := cli.VolumeList(runCtx, dockervolume.ListOptions{Filters: filters.NewArgs(filters.Arg("dangling", "true"))})
	if err != nil {
		return nil, 0, apperrors.Operation("预览 Docker volume prune 失败：" + err.Error())
	}
	resources := make([]string, 0, len(resp.Volumes))
	var bytes int64
	for _, item := range resp.Volumes {
		resources = append(resources, item.Name)
		if item.UsageData != nil && item.UsageData.Size > 0 {
			bytes += item.UsageData.Size
		}
	}
	sort.Strings(resources)
	return resources, bytes, nil
}

func (s *service) ApplyVolumePrune(ctx context.Context, request CleanupApplyRequest) (map[string]any, error) {
	cli, err := s.requireClient()
	if err != nil {
		return nil, err
	}
	resources, _, err := s.previewVolumePruneResources(ctx)
	if err != nil {
		return nil, err
	}
	if err := requireCleanupToken("volume", resources, request.PreviewToken); err != nil {
		return nil, err
	}
	runCtx, cancel := s.withTimeout(ctx)
	defer cancel()
	report, err := cli.VolumesPrune(runCtx, filters.NewArgs())
	if err != nil {
		return nil, apperrors.Operation("执行 Docker volume prune 失败：" + err.Error())
	}
	return map[string]any{"deleted": report.VolumesDeleted, "spaceReclaimed": int64(report.SpaceReclaimed)}, nil
}

func (s *service) PreviewImageCleanup(ctx context.Context, request CleanupPreviewRequest) (*CleanupPreviewVO, error) {
	_ = request
	resources, bytes, err := s.previewImageCleanupResources(ctx)
	if err != nil {
		return nil, err
	}
	return &CleanupPreviewVO{PreviewToken: cleanupToken("image", resources), ResourceType: "image", AffectedResources: resources, EstimatedBytes: bytes}, nil
}

func (s *service) previewImageCleanupResources(ctx context.Context) ([]string, int64, error) {
	cli, err := s.requireClient()
	if err != nil {
		return nil, 0, err
	}
	runCtx, cancel := s.withTimeout(ctx)
	defer cancel()
	images, err := cli.ImageList(runCtx, dockerimage.ListOptions{All: true, Filters: filters.NewArgs(filters.Arg("dangling", "true"))})
	if err != nil {
		return nil, 0, apperrors.Operation("预览 Docker dangling image 失败：" + err.Error())
	}
	resources := make([]string, 0, len(images))
	var bytes int64
	for _, image := range images {
		resources = append(resources, stripSHA(image.ID))
		bytes += image.Size
	}
	sort.Strings(resources)
	return resources, bytes, nil
}

func (s *service) PreviewContainerCleanup(ctx context.Context, request CleanupPreviewRequest) (*CleanupPreviewVO, error) {
	_ = request
	resources, err := s.previewContainerCleanupResources(ctx)
	if err != nil {
		return nil, err
	}
	return &CleanupPreviewVO{PreviewToken: cleanupToken("container", resources), ResourceType: "container", AffectedResources: resources}, nil
}

func (s *service) previewContainerCleanupResources(ctx context.Context) ([]string, error) {
	cli, err := s.requireClient()
	if err != nil {
		return nil, err
	}
	runCtx, cancel := s.withTimeout(ctx)
	defer cancel()
	containers, err := cli.ContainerList(runCtx, dockercontainer.ListOptions{All: true, Filters: exitedContainerFilters()})
	if err != nil {
		return nil, apperrors.Operation("预览 Docker stopped container 失败：" + err.Error())
	}
	resources := make([]string, 0, len(containers))
	for _, container := range containers {
		resources = append(resources, stripSHA(container.ID))
	}
	sort.Strings(resources)
	return resources, nil
}

func (s *service) applyImageCleanup(ctx context.Context, request CleanupApplyRequest) (map[string]any, error) {
	cli, err := s.requireClient()
	if err != nil {
		return nil, err
	}
	resources, _, err := s.previewImageCleanupResources(ctx)
	if err != nil {
		return nil, err
	}
	if err := requireCleanupToken("image", resources, request.PreviewToken); err != nil {
		return nil, err
	}
	runCtx, cancel := s.withTimeout(ctx)
	defer cancel()
	report, err := cli.ImagesPrune(runCtx, filters.NewArgs(filters.Arg("dangling", "true")))
	if err != nil {
		return nil, apperrors.Operation("清理 Docker dangling image 失败：" + err.Error())
	}
	deleted := make([]string, 0, len(report.ImagesDeleted))
	for _, item := range report.ImagesDeleted {
		deleted = append(deleted, firstNonBlank(item.Deleted, item.Untagged))
	}
	return map[string]any{"deleted": deleted, "spaceReclaimed": report.SpaceReclaimed}, nil
}

func (s *service) applyContainerCleanup(ctx context.Context, request CleanupApplyRequest) (map[string]any, error) {
	cli, err := s.requireClient()
	if err != nil {
		return nil, err
	}
	resources, err := s.previewContainerCleanupResources(ctx)
	if err != nil {
		return nil, err
	}
	if err := requireCleanupToken("container", resources, request.PreviewToken); err != nil {
		return nil, err
	}
	runCtx, cancel := s.withTimeout(ctx)
	defer cancel()
	// Docker Engine's container prune API does not accept the list-only
	// "status" filter. The preview token above binds the current stopped
	// container set, and prune semantics already target stopped containers.
	report, err := cli.ContainersPrune(runCtx, filters.NewArgs())
	if err != nil {
		return nil, apperrors.Operation("清理 Docker stopped container 失败：" + err.Error())
	}
	return map[string]any{"deleted": report.ContainersDeleted, "spaceReclaimed": report.SpaceReclaimed}, nil
}

func (s *service) syncRegistry(ctx context.Context, registryID int64) (map[string]any, error) {
	repos, err := s.ListRepositories(ctx, registryID, 1, s.cfg.Registry.MaxPageSize, "")
	if err != nil {
		return nil, err
	}
	return map[string]any{"registryId": registryID, "repositoryCount": repos.Total, "sample": repos.Records}, nil
}

func (s *service) MetricsSnapshot(ctx context.Context) (*DockerMetricsSnapshot, error) {
	snapshot := &DockerMetricsSnapshot{Enabled: s.Enabled(), ContainerCountByState: map[string]int64{}}
	if !s.Enabled() {
		return snapshot, nil
	}
	cli, err := s.requireClient()
	if err != nil {
		return snapshot, nil
	}
	runCtx, cancel := s.withTimeout(ctx)
	defer cancel()
	if _, err := cli.Ping(runCtx); err == nil {
		snapshot.DaemonHealthy = true
	}
	containers, _ := cli.ContainerList(runCtx, dockercontainer.ListOptions{All: true})
	for _, container := range containers {
		snapshot.ContainerCountByState[strings.ToLower(string(container.State))]++
	}
	images, _ := cli.ImageList(runCtx, dockerimage.ListOptions{All: true})
	for _, image := range images {
		snapshot.ImageCount++
		snapshot.ImageSizeBytes += image.Size
	}
	if s.repo != nil {
		registries, _ := s.repo.List(ctx)
		snapshot.RegistryHealthy = len(registries) > 0
	}
	if s.operations != nil {
		total, succeeded, failed, policy, _ := s.operations.OperationStats(ctx)
		snapshot.OperationTotal = total
		snapshot.OperationSucceeded = succeeded
		snapshot.OperationFailed = failed
		snapshot.PolicyViolationTotal = policy
	}
	return snapshot, nil
}

func exitedContainerFilters() filters.Args {
	args := filters.NewArgs()
	args.Add("status", "exited")
	args.Add("status", "created")
	return args
}

func networkResourceView(network dockernetwork.Inspect) DockerResourceView {
	view := DockerResourceView{
		ID:         network.ID,
		Name:       network.Name,
		Driver:     network.Driver,
		Scope:      network.Scope,
		Labels:     safeLabels(network.Labels),
		Dangling:   len(network.Containers) == 0,
		CreatedAt:  network.Created.Format(time.RFC3339),
		Internal:   network.Internal,
		Attachable: network.Attachable,
		Ingress:    network.Ingress,
		IPv6:       network.EnableIPv6,
		Options:    network.Options,
		Containers: networkEndpointMap(network.Containers),
	}
	return view
}

func volumeResourceView(volume dockervolume.Volume) DockerResourceView {
	view := DockerResourceView{
		ID:         volume.Name,
		Name:       volume.Name,
		Driver:     volume.Driver,
		Scope:      volume.Scope,
		Labels:     safeLabels(volume.Labels),
		CreatedAt:  volume.CreatedAt,
		Mountpoint: volume.Mountpoint,
		Options:    volume.Options,
	}
	if volume.UsageData != nil {
		view.SizeBytes = volume.UsageData.Size
		view.Dangling = volume.UsageData.RefCount == 0
	}
	return view
}

func networkEndpointSettings(request DockerNetworkConnectRequest) *dockernetwork.EndpointSettings {
	settings := &dockernetwork.EndpointSettings{
		Aliases:    request.Aliases,
		Links:      request.Links,
		MacAddress: request.MacAddress,
		DriverOpts: request.DriverOpts,
	}
	if request.IPv4Address != "" || request.IPv6Address != "" || len(request.LinkLocalIPs) > 0 {
		settings.IPAMConfig = &dockernetwork.EndpointIPAMConfig{
			IPv4Address:  request.IPv4Address,
			IPv6Address:  request.IPv6Address,
			LinkLocalIPs: request.LinkLocalIPs,
		}
	}
	if settings.IPAMConfig == nil && len(settings.Aliases) == 0 && len(settings.Links) == 0 && settings.MacAddress == "" && len(settings.DriverOpts) == 0 {
		return nil
	}
	return settings
}

func networkEndpointMap(values map[string]dockernetwork.EndpointResource) map[string]any {
	if len(values) == 0 {
		return nil
	}
	result := make(map[string]any, len(values))
	for key, value := range values {
		result[key] = value
	}
	return result
}

func matchResource(item DockerResourceView, keyword string) bool {
	keyword = strings.ToLower(strings.TrimSpace(keyword))
	if keyword == "" {
		return true
	}
	return strings.Contains(strings.ToLower(item.ID), keyword) ||
		strings.Contains(strings.ToLower(item.Name), keyword) ||
		strings.Contains(strings.ToLower(item.Driver), keyword)
}

func sanitizeMap(value map[string]any, cfg config.DockerSecurityConfig) map[string]any {
	masked, ok := maskJSONValue(value, cfg).(map[string]any)
	if !ok {
		return value
	}
	return masked
}

func cleanupToken(prefix string, resources []string) string {
	copied := append([]string{}, resources...)
	sort.Strings(copied)
	sum := sha256.Sum256([]byte(strings.Join(copied, "\n")))
	return fmt.Sprintf("%s:%d:%s", prefix, len(copied), hex.EncodeToString(sum[:])[:24])
}

func requireCleanupToken(prefix string, resources []string, token string) error {
	expected := cleanupToken(prefix, resources)
	if strings.TrimSpace(token) == "" {
		return apperrors.Params("cleanup previewToken 不能为空，请先执行 preview")
	}
	if strings.TrimSpace(token) != expected {
		return apperrors.Params("cleanup previewToken 已失效，请重新 preview")
	}
	return nil
}

func extractComposeServices(yaml string) []string {
	lines := strings.Split(yaml, "\n")
	inServices := false
	result := []string{}
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed == "services:" {
			inServices = true
			continue
		}
		if inServices && strings.HasSuffix(trimmed, ":") && !strings.HasPrefix(line, "    ") {
			name := strings.TrimSuffix(trimmed, ":")
			if name != "" && name != "services" {
				result = append(result, name)
			}
		}
	}
	return result
}
