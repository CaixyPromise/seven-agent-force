package docker

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"time"

	apperrors "github.com/CaixyPromise/seven-agent/force/internal/common/errors"
	dockersystem "github.com/docker/docker/api/types/system"
)

const dockerDaemonConfigPath = "/etc/docker/daemon.json"

var daemonConfigEditableKeys = []string{
	"registry-mirrors",
	"insecure-registries",
	"log-driver",
	"log-opts",
	"live-restore",
	"ipv6",
	"iptables",
	"bip",
}

var daemonConfigEditableSet = func() map[string]struct{} {
	result := make(map[string]struct{}, len(daemonConfigEditableKeys))
	for _, key := range daemonConfigEditableKeys {
		result[key] = struct{}{}
	}
	return result
}()

func (s *service) GetDaemonConfig(ctx context.Context) (*DockerDaemonConfigView, error) {
	support, err := s.daemonConfigSupport(ctx)
	if err != nil {
		return nil, err
	}
	if !support.supported {
		return unsupportedDaemonConfigView(support), nil
	}
	raw, err := readDaemonConfigFile(support.configPath)
	if err != nil {
		return nil, err
	}
	return daemonConfigViewFromRaw(support, raw), nil
}

func (s *service) ValidateDaemonConfig(ctx context.Context, request DockerDaemonConfigUpdateRequest) (*DockerDaemonConfigValidateView, error) {
	support, err := s.daemonConfigSupport(ctx)
	if err != nil {
		return nil, err
	}
	if !support.supported {
		return &DockerDaemonConfigValidateView{Valid: false, Message: support.reason}, nil
	}
	if _, err := validateDaemonEditable(request.Editable); err != nil {
		return &DockerDaemonConfigValidateView{Valid: false, Message: err.Error()}, nil
	}
	return &DockerDaemonConfigValidateView{Valid: true, Keys: append([]string{}, daemonConfigEditableKeys...)}, nil
}

func (s *service) SaveDaemonConfig(ctx context.Context, request DockerDaemonConfigUpdateRequest) (*DockerDaemonConfigView, error) {
	support, err := s.daemonConfigSupport(ctx)
	if err != nil {
		return nil, err
	}
	if !support.supported {
		return nil, apperrors.Operation(support.reason)
	}
	editable, err := validateDaemonEditable(request.Editable)
	if err != nil {
		return nil, err
	}
	raw, err := readDaemonConfigFile(support.configPath)
	if err != nil {
		return nil, err
	}
	merged := mergeDaemonConfig(raw, editable)
	if err := backupAndWriteDaemonConfig(support.configPath, merged); err != nil {
		return nil, err
	}
	return daemonConfigViewFromRaw(support, merged), nil
}

func (s *service) RestartDaemon(ctx context.Context) (bool, error) {
	support, err := s.daemonConfigSupport(ctx)
	if err != nil {
		return false, err
	}
	if !support.supported {
		return false, apperrors.Operation(support.reason)
	}
	command := exec.CommandContext(ctx, "systemctl", "restart", "docker")
	output, err := command.CombinedOutput()
	if err != nil {
		message := strings.TrimSpace(string(output))
		if message == "" {
			message = err.Error()
		}
		return false, apperrors.Operation("重启 Docker daemon 失败：" + message)
	}
	return true, nil
}

type daemonConfigSupport struct {
	supported  bool
	reason     string
	platform   string
	rootless   bool
	configPath string
}

func (s *service) daemonConfigSupport(ctx context.Context) (daemonConfigSupport, error) {
	support := daemonConfigSupport{platform: runtime.GOOS, configPath: dockerDaemonConfigPath}
	if runtime.GOOS != "linux" {
		support.reason = "Docker daemon 配置编辑仅支持 rootful Linux"
		return support, nil
	}
	if isRemoteDockerHost(s.cfg.Engine.Host) {
		support.reason = "远程 Docker Host 不支持编辑本机 daemon.json"
		return support, nil
	}
	cli, err := s.requireClient()
	if err != nil {
		return support, err
	}
	runCtx, cancel := s.withTimeout(ctx)
	defer cancel()
	info, err := cli.Info(runCtx)
	if err != nil {
		return support, apperrors.Operation("获取 Docker daemon 信息失败：" + err.Error())
	}
	support.rootless = dockerInfoRootless(info)
	if support.rootless {
		support.reason = "无法可靠识别 rootless Docker daemon 配置路径"
		return support, nil
	}
	support.supported = true
	return support, nil
}

func isRemoteDockerHost(host string) bool {
	host = strings.TrimSpace(host)
	if host == "" {
		host = strings.TrimSpace(os.Getenv("DOCKER_HOST"))
	}
	if host == "" {
		return false
	}
	if strings.HasPrefix(host, "unix://") || strings.HasPrefix(host, "npipe://") {
		return false
	}
	parsed, err := url.Parse(host)
	if err == nil && parsed.Scheme != "" {
		return parsed.Scheme != "unix" && parsed.Scheme != "npipe"
	}
	return true
}

func dockerInfoRootless(info dockersystem.Info) bool {
	for _, option := range info.SecurityOptions {
		option = strings.ToLower(strings.TrimSpace(option))
		if option == "name=rootless" || option == "rootless" || strings.Contains(option, "rootless") {
			return true
		}
	}
	root := strings.TrimSpace(info.DockerRootDir)
	return root != "" && strings.Contains(root, "/.local/share/docker")
}

func unsupportedDaemonConfigView(support daemonConfigSupport) *DockerDaemonConfigView {
	return &DockerDaemonConfigView{
		Supported:     false,
		SupportReason: support.reason,
		Platform:      support.platform,
		Rootless:      support.rootless,
		ConfigPath:    support.configPath,
		EditableKeys:  append([]string{}, daemonConfigEditableKeys...),
		Editable:      map[string]any{},
		Readonly:      map[string]any{},
		Raw:           map[string]any{},
	}
}

func daemonConfigViewFromRaw(support daemonConfigSupport, raw map[string]any) *DockerDaemonConfigView {
	editable := map[string]any{}
	readonly := map[string]any{}
	for key, value := range raw {
		if _, ok := daemonConfigEditableSet[key]; ok {
			editable[key] = value
			continue
		}
		readonly[key] = value
	}
	return &DockerDaemonConfigView{
		Supported:       support.supported,
		SupportReason:   support.reason,
		Platform:        support.platform,
		Rootless:        support.rootless,
		ConfigPath:      support.configPath,
		Editable:        editable,
		Readonly:        readonly,
		Raw:             cloneMap(raw),
		EditableKeys:    append([]string{}, daemonConfigEditableKeys...),
		RequiresRestart: true,
	}
}

func readDaemonConfigFile(path string) (map[string]any, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return map[string]any{}, nil
		}
		return nil, apperrors.Operation("读取 Docker daemon.json 失败：" + err.Error())
	}
	data = bytes.TrimSpace(data)
	if len(data) == 0 {
		return map[string]any{}, nil
	}
	var raw map[string]any
	if err := json.Unmarshal(data, &raw); err != nil {
		return nil, apperrors.Params("daemon.json 不是有效 JSON：" + err.Error())
	}
	if raw == nil {
		raw = map[string]any{}
	}
	return raw, nil
}

func validateDaemonEditable(editable map[string]any) (map[string]any, error) {
	result := map[string]any{}
	for key, value := range editable {
		key = strings.TrimSpace(key)
		if _, ok := daemonConfigEditableSet[key]; !ok {
			return nil, apperrors.Params("daemon 配置字段不允许编辑：" + key)
		}
		normalized, err := normalizeDaemonEditableValue(key, value)
		if err != nil {
			return nil, err
		}
		result[key] = normalized
	}
	return result, nil
}

func normalizeDaemonEditableValue(key string, value any) (any, error) {
	switch key {
	case "registry-mirrors", "insecure-registries":
		return stringSliceValue(key, value)
	case "log-driver", "bip":
		text, ok := value.(string)
		if !ok {
			return nil, apperrors.Params(key + " 必须是字符串")
		}
		return strings.TrimSpace(text), nil
	case "log-opts":
		if value == nil {
			return map[string]any{}, nil
		}
		if opts, ok := value.(map[string]any); ok {
			return cloneMap(opts), nil
		}
		return nil, apperrors.Params("log-opts 必须是对象")
	case "live-restore", "ipv6", "iptables":
		boolean, ok := value.(bool)
		if !ok {
			return nil, apperrors.Params(key + " 必须是布尔值")
		}
		return boolean, nil
	default:
		return nil, apperrors.Params("daemon 配置字段不允许编辑：" + key)
	}
}

func stringSliceValue(key string, value any) ([]string, error) {
	items, ok := value.([]any)
	if !ok {
		if typed, typedOK := value.([]string); typedOK {
			return safeStrings(typed), nil
		}
		return nil, apperrors.Params(key + " 必须是字符串数组")
	}
	result := make([]string, 0, len(items))
	for _, item := range items {
		text, ok := item.(string)
		if !ok {
			return nil, apperrors.Params(key + " 必须是字符串数组")
		}
		if text = strings.TrimSpace(text); text != "" {
			result = append(result, text)
		}
	}
	return result, nil
}

func mergeDaemonConfig(raw, editable map[string]any) map[string]any {
	merged := cloneMap(raw)
	for key, value := range editable {
		merged[key] = value
	}
	return merged
}

func backupAndWriteDaemonConfig(path string, raw map[string]any) error {
	existing, err := os.ReadFile(path)
	if err != nil && !os.IsNotExist(err) {
		return apperrors.Operation("读取 Docker daemon.json 失败：" + err.Error())
	}
	if err == nil {
		backupPath := fmt.Sprintf("%s.bak.%s", path, time.Now().Format("20060102150405"))
		if err := os.WriteFile(backupPath, existing, 0600); err != nil {
			return apperrors.Operation("备份 Docker daemon.json 失败：" + err.Error())
		}
	}
	data, err := json.MarshalIndent(raw, "", "  ")
	if err != nil {
		return apperrors.Params("序列化 Docker daemon.json 失败：" + err.Error())
	}
	data = append(data, '\n')
	dir := filepath.Dir(path)
	tmp, err := os.CreateTemp(dir, ".daemon.json.*.tmp")
	if err != nil {
		return apperrors.Operation("创建 Docker daemon.json 临时文件失败：" + err.Error())
	}
	tmpPath := tmp.Name()
	defer os.Remove(tmpPath)
	if _, err := tmp.Write(data); err != nil {
		_ = tmp.Close()
		return apperrors.Operation("写入 Docker daemon.json 临时文件失败：" + err.Error())
	}
	if err := tmp.Chmod(0600); err != nil {
		_ = tmp.Close()
		return apperrors.Operation("设置 Docker daemon.json 权限失败：" + err.Error())
	}
	if err := tmp.Close(); err != nil {
		return apperrors.Operation("关闭 Docker daemon.json 临时文件失败：" + err.Error())
	}
	if err := os.Rename(tmpPath, path); err != nil {
		return apperrors.Operation("替换 Docker daemon.json 失败：" + err.Error())
	}
	return nil
}

func cloneMap(input map[string]any) map[string]any {
	result := make(map[string]any, len(input))
	for key, value := range input {
		result[key] = value
	}
	return result
}

func sortedMapKeys(input map[string]any) []string {
	keys := make([]string, 0, len(input))
	for key := range input {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}
