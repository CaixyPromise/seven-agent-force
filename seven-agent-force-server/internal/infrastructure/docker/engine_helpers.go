package docker

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"net/url"
	"strconv"
	"strings"

	apperrors "github.com/CaixyPromise/seven-agent/force/internal/common/errors"
	dockercontainer "github.com/docker/docker/api/types/container"
	dockerimage "github.com/docker/docker/api/types/image"
	dockerregistry "github.com/docker/docker/api/types/registry"
	dockerclient "github.com/docker/docker/client"
	"github.com/docker/go-connections/nat"
)

func (s *service) resolveContainer(ctx context.Context, id string) (*dockerclient.Client, string, dockercontainer.Summary, error) {
	cli, err := s.requireClient()
	if err != nil {
		return nil, "", dockercontainer.Summary{}, err
	}
	runCtx, cancel := s.withTimeout(ctx)
	defer cancel()
	items, err := cli.ContainerList(runCtx, dockercontainer.ListOptions{All: true})
	if err != nil {
		return nil, "", dockercontainer.Summary{}, apperrors.Operation("获取 Docker 容器列表失败：" + err.Error())
	}
	for _, item := range items {
		if id == item.ID || id == stripSHA(item.ID) {
			return cli, item.ID, item, nil
		}
	}
	return nil, "", dockercontainer.Summary{}, apperrors.NotFound("容器不存在")
}

func (s *service) resolveImage(ctx context.Context, id string) (*dockerclient.Client, string, dockerimage.Summary, error) {
	cli, err := s.requireClient()
	if err != nil {
		return nil, "", dockerimage.Summary{}, err
	}
	runCtx, cancel := s.withTimeout(ctx)
	defer cancel()
	items, err := cli.ImageList(runCtx, dockerimage.ListOptions{All: true})
	if err != nil {
		return nil, "", dockerimage.Summary{}, apperrors.Operation("获取 Docker 镜像列表失败：" + err.Error())
	}
	for _, item := range items {
		if id == item.ID || id == stripSHA(item.ID) || contains(item.RepoTags, id) || contains(item.RepoDigests, id) {
			return cli, item.ID, item, nil
		}
	}
	return nil, "", dockerimage.Summary{}, apperrors.NotFound("镜像不存在")
}

func (s *service) resolveImageReference(ctx context.Context, imageID, imageReference string) (string, error) {
	if strings.TrimSpace(imageReference) != "" {
		return strings.TrimSpace(imageReference), nil
	}
	_, _, summary, err := s.resolveImage(ctx, imageID)
	if err != nil {
		return "", err
	}
	if len(summary.RepoTags) == 0 || summary.RepoTags[0] == "<none>:<none>" {
		return "", apperrors.Params("镜像缺少可用 tag，请先给镜像打标签")
	}
	return summary.RepoTags[0], nil
}

func (s *service) toContainerView(item dockercontainer.Summary, inspect *dockercontainer.InspectResponse) ContainerView {
	labels := safeLabels(item.Labels)
	restartCount := 0
	if inspect != nil && inspect.ContainerJSONBase != nil {
		restartCount = inspect.RestartCount
		if inspect.Config != nil && inspect.Config.Labels != nil {
			labels = safeLabels(inspect.Config.Labels)
		}
	}
	return ContainerView{
		ID:                 stripSHA(item.ID),
		Name:               firstContainerName(item),
		Image:              item.Image,
		ImageID:            stripSHA(item.ImageID),
		State:              string(item.State),
		Status:             item.Status,
		Created:            item.Created,
		Ports:              toPorts(item.Ports),
		Labels:             labels,
		RestartCount:       restartCount,
		ComposeManaged:     labels["com.docker.compose.project"] != "",
		ComposeProject:     labels["com.docker.compose.project"],
		ComposeService:     labels["com.docker.compose.service"],
		ComposeConfigFiles: labels["com.docker.compose.project.config_files"],
		ComposeWorkingDir:  labels["com.docker.compose.project.working_dir"],
		AvailableActions:   containerAvailableActions(string(item.State)),
	}
}

func firstContainerName(item dockercontainer.Summary) string {
	if len(item.Names) == 0 {
		return stripSHA(item.ID)
	}
	return strings.TrimPrefix(item.Names[0], "/")
}

func toPorts(ports []dockercontainer.Port) []ContainerPortView {
	result := make([]ContainerPortView, 0, len(ports))
	for _, port := range ports {
		result = append(result, ContainerPortView{PrivatePort: port.PrivatePort, PublicPort: port.PublicPort, Type: port.Type, IP: port.IP})
	}
	return result
}

func matchContainer(item ContainerView, keyword, state string) bool {
	needle := strings.ToLower(strings.TrimSpace(keyword))
	if needle != "" && !strings.Contains(strings.ToLower(item.ID), needle) && !strings.Contains(strings.ToLower(item.Name), needle) && !strings.Contains(strings.ToLower(item.Image), needle) {
		return false
	}
	state = strings.TrimSpace(state)
	return state == "" || strings.EqualFold(item.State, state)
}

func matchImage(item ImageView, keyword string) bool {
	needle := strings.ToLower(strings.TrimSpace(keyword))
	if needle == "" {
		return true
	}
	if strings.Contains(strings.ToLower(item.ImageID), needle) {
		return true
	}
	for _, value := range append(item.RepoTags, item.RepoDigests...) {
		if strings.Contains(strings.ToLower(value), needle) {
			return true
		}
	}
	return false
}

func buildImageUsage(containers []dockercontainer.Summary) map[string]int {
	usage := map[string]int{}
	for _, item := range containers {
		usage[stripSHA(item.ImageID)]++
	}
	return usage
}

func (s *service) registryAuth(ctx context.Context, registryID int64) (string, error) {
	if registryID <= 0 {
		return "", nil
	}
	rt, err := s.registryRuntime(ctx, registryID)
	if err != nil {
		return "", err
	}
	return registryAuthPayload(rt)
}

func registryAuthPayload(rt registryRuntime) (string, error) {
	auth := dockerregistry.AuthConfig{
		ServerAddress: rt.Endpoint,
		Username:      rt.Username,
		Password:      rt.Password,
	}
	payload, err := json.Marshal(auth)
	if err != nil {
		return "", err
	}
	return base64.URLEncoding.EncodeToString(payload), nil
}

func qualifyRegistryRepository(rt registryRuntime, repository string) string {
	repository = strings.TrimSpace(repository)
	host := registryHost(rt.Endpoint)
	if repository == "" || host == "" || repositoryHasRegistryHost(repository) {
		return repository
	}
	return host + "/" + strings.TrimPrefix(repository, "/")
}

func registryHost(endpoint string) string {
	endpoint = strings.TrimSpace(endpoint)
	if endpoint == "" {
		return ""
	}
	if parsed, err := url.Parse(endpoint); err == nil && parsed.Host != "" {
		return parsed.Host
	}
	return strings.Trim(strings.TrimPrefix(strings.TrimPrefix(endpoint, "http://"), "https://"), "/")
}

func repositoryHasRegistryHost(repository string) bool {
	first, _, _ := strings.Cut(strings.TrimSpace(repository), "/")
	return strings.Contains(first, ".") || strings.Contains(first, ":") || first == "localhost"
}

func envCommands(values []string) []KeyValueCommand {
	result := make([]KeyValueCommand, 0, len(values))
	for _, value := range values {
		if strings.TrimSpace(value) == "" {
			continue
		}
		key, val, ok := strings.Cut(value, "=")
		if !ok {
			val = ""
		}
		result = append(result, KeyValueCommand{Key: key, Value: maskSensitiveValue(key, val)})
	}
	return result
}

func labelCommands(values map[string]string) []KeyValueCommand {
	result := make([]KeyValueCommand, 0, len(values))
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sortStrings(keys)
	for _, key := range keys {
		result = append(result, KeyValueCommand{Key: key, Value: maskSensitiveValue(key, values[key])})
	}
	return result
}

func exposedPortsFromConfig(ports map[string]struct{}) []PortBindingCommand {
	result := make([]PortBindingCommand, 0, len(ports))
	keys := make([]string, 0, len(ports))
	for port := range ports {
		keys = append(keys, string(port))
	}
	sortStrings(keys)
	for _, raw := range keys {
		port := nat.Port(raw)
		private, _ := nat.ParsePort(port.Port())
		result = append(result, PortBindingCommand{ContainerPort: uint16(private), Protocol: port.Proto()})
	}
	return result
}

func volumesFromConfig(volumes map[string]struct{}) []VolumeBindingCommand {
	result := make([]VolumeBindingCommand, 0, len(volumes))
	keys := make([]string, 0, len(volumes))
	for key := range volumes {
		keys = append(keys, key)
	}
	sortStrings(keys)
	for _, target := range keys {
		result = append(result, VolumeBindingCommand{Target: target, Type: "bind"})
	}
	return result
}

func envStrings(values []KeyValueCommand) []string {
	result := make([]string, 0, len(values))
	for _, item := range values {
		if strings.TrimSpace(item.Key) != "" {
			result = append(result, strings.TrimSpace(item.Key)+"="+item.Value)
		}
	}
	return result
}

func keyValueMap(values []KeyValueCommand) map[string]string {
	if len(values) == 0 {
		return nil
	}
	result := map[string]string{}
	for _, item := range values {
		if strings.TrimSpace(item.Key) != "" {
			result[strings.TrimSpace(item.Key)] = item.Value
		}
	}
	return result
}

func portSet(bindings []PortBindingCommand) nat.PortSet {
	result := nat.PortSet{}
	for _, binding := range bindings {
		if binding.ContainerPort == 0 {
			continue
		}
		proto := firstNonBlank(binding.Protocol, "tcp")
		port, err := nat.NewPort(proto, strconv.Itoa(int(binding.ContainerPort)))
		if err == nil {
			result[port] = struct{}{}
		}
	}
	return result
}

func portMap(bindings []PortBindingCommand) nat.PortMap {
	result := nat.PortMap{}
	for _, binding := range bindings {
		if binding.ContainerPort == 0 {
			continue
		}
		proto := firstNonBlank(binding.Protocol, "tcp")
		port, err := nat.NewPort(proto, strconv.Itoa(int(binding.ContainerPort)))
		if err != nil {
			continue
		}
		hostPort := ""
		if binding.HostPort > 0 {
			hostPort = strconv.Itoa(int(binding.HostPort))
		}
		result[port] = append(result[port], nat.PortBinding{HostIP: binding.HostIP, HostPort: hostPort})
	}
	return result
}

func bindStrings(bindings []VolumeBindingCommand) []string {
	result := make([]string, 0, len(bindings))
	for _, binding := range bindings {
		if strings.TrimSpace(binding.Source) == "" || strings.TrimSpace(binding.Target) == "" {
			continue
		}
		spec := strings.TrimSpace(binding.Source) + ":" + strings.TrimSpace(binding.Target)
		if binding.ReadOnly {
			spec += ":ro"
		}
		result = append(result, spec)
	}
	return result
}

func restartPolicy(policy string, retries int) dockercontainer.RestartPolicy {
	switch strings.ToLower(strings.TrimSpace(policy)) {
	case "no", "none":
		return dockercontainer.RestartPolicy{Name: dockercontainer.RestartPolicyDisabled}
	case "unless-stopped", "unless_stopped":
		return dockercontainer.RestartPolicy{Name: dockercontainer.RestartPolicyUnlessStopped}
	case "on-failure", "on_failure":
		if retries <= 0 {
			retries = 3
		}
		return dockercontainer.RestartPolicy{Name: dockercontainer.RestartPolicyOnFailure, MaximumRetryCount: retries}
	default:
		return dockercontainer.RestartPolicy{Name: dockercontainer.RestartPolicyAlways}
	}
}

func applyLimits(host *dockercontainer.HostConfig, limits *ResourceLimitCommand) {
	if host == nil || limits == nil {
		return
	}
	if limits.CPUs > 0 {
		host.NanoCPUs = int64(limits.CPUs * 1_000_000_000)
	}
	if limits.MemoryMB > 0 {
		host.Memory = limits.MemoryMB * 1024 * 1024
	}
	if limits.MemorySwapMB > 0 {
		host.MemorySwap = limits.MemorySwapMB * 1024 * 1024
	}
	if limits.PidsLimit > 0 {
		host.PidsLimit = &limits.PidsLimit
	}
}

func buildComposeFromInspect(inspect dockercontainer.InspectResponse) string {
	service := "service"
	if inspect.Name != "" {
		service = strings.TrimPrefix(inspect.Name, "/")
	}
	env, labels, ports, volumes := []KeyValueCommand{}, []KeyValueCommand{}, []PortBindingCommand{}, []VolumeBindingCommand{}
	entrypoint, cmd := []string{}, []string{}
	imageRef, workingDir, user := inspect.Image, "", ""
	tty, stdinOpen := false, false
	if inspect.Config != nil {
		imageRef = firstNonBlank(inspect.Config.Image, imageRef)
		env = envCommands(inspect.Config.Env)
		labels = labelCommands(inspect.Config.Labels)
		entrypoint = []string(inspect.Config.Entrypoint)
		cmd = []string(inspect.Config.Cmd)
		workingDir = inspect.Config.WorkingDir
		user = inspect.Config.User
		tty = inspect.Config.Tty
		stdinOpen = inspect.Config.OpenStdin
	}
	networkMode, restart := "", ""
	privileged, publishAll := false, false
	capAdd, capDrop := []string{}, []string{}
	if inspect.HostConfig != nil {
		networkMode = string(inspect.HostConfig.NetworkMode)
		restart = string(inspect.HostConfig.RestartPolicy.Name)
		privileged = inspect.HostConfig.Privileged
		publishAll = inspect.HostConfig.PublishAllPorts
		capAdd = []string(inspect.HostConfig.CapAdd)
		capDrop = []string(inspect.HostConfig.CapDrop)
		ports = portBindingsFromHost(inspect.HostConfig.PortBindings)
		volumes = volumeBindingsFromBinds(inspect.HostConfig.Binds)
	}
	return buildComposeYaml(service, imageRef, env, ports, volumes, labels, workingDir, user, entrypoint, cmd, tty, stdinOpen, networkMode, privileged, restart, publishAll, capAdd, capDrop, "")
}

func portBindingsFromHost(bindings nat.PortMap) []PortBindingCommand {
	result := []PortBindingCommand{}
	for port, values := range bindings {
		private, _ := nat.ParsePort(port.Port())
		if len(values) == 0 {
			result = append(result, PortBindingCommand{ContainerPort: uint16(private), Protocol: port.Proto()})
			continue
		}
		for _, binding := range values {
			hostPort, _ := strconv.Atoi(binding.HostPort)
			result = append(result, PortBindingCommand{ContainerPort: uint16(private), Protocol: port.Proto(), HostIP: binding.HostIP, HostPort: uint16(hostPort)})
		}
	}
	return result
}

func volumeBindingsFromBinds(binds []string) []VolumeBindingCommand {
	result := []VolumeBindingCommand{}
	for _, bind := range binds {
		parts := strings.Split(bind, ":")
		if len(parts) >= 2 {
			result = append(result, VolumeBindingCommand{Source: parts[0], Target: parts[1], Type: "bind", ReadOnly: len(parts) > 2 && strings.Contains(parts[2], "ro")})
		}
	}
	return result
}
