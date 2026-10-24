package docker

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	apperrors "github.com/CaixyPromise/seven-agent/force/internal/common/errors"
	"github.com/CaixyPromise/seven-agent/force/internal/shared/config"
	"go.yaml.in/yaml/v3"
)

type composeWorkspaceManager struct {
	cfg config.DockerComposeConfig
}

type composeWriteResult struct {
	WorkingDir       string
	ComposeFilePath  string
	ConfigFilesJSON  string
	FileManifestJSON string
	WrittenFilePaths []string
}

type composeFileManifestEntry struct {
	Path        string `json:"path"`
	ServiceName string `json:"serviceName,omitempty"`
	Kind        string `json:"kind"`
	Size        int    `json:"size"`
}

func newComposeWorkspaceManager(cfg config.DockerComposeConfig) *composeWorkspaceManager {
	return &composeWorkspaceManager{cfg: cfg}
}

func (m *composeWorkspaceManager) Check(request ComposeWorkspaceCheckRequest) (*ComposeWorkspaceCheckView, error) {
	resolved, exists, err := m.resolveWorkspace(request.WorkingDir, false)
	if err != nil {
		return &ComposeWorkspaceCheckView{Valid: false, Message: err.Error()}, nil
	}
	view := &ComposeWorkspaceCheckView{
		Valid:        true,
		Exists:       exists,
		CanCreate:    !exists,
		AllowedRoot:  true,
		ResolvedPath: resolved,
	}
	if exists {
		info, err := os.Stat(resolved)
		if err != nil {
			view.Valid = false
			view.Message = err.Error()
			return view, nil
		}
		if !info.IsDir() {
			view.Valid = false
			view.Message = "workingDir 必须是目录"
			return view, nil
		}
		view.CanWrite = info.Mode().Perm()&0o200 != 0
		view.ComposeFileExists = m.composeFileExists(resolved)
		if view.ComposeFileExists && !request.OverwriteExistingCompose {
			view.Warnings = append(view.Warnings, "工作目录已存在 Compose 文件，保存时需要明确允许覆盖")
		}
		return view, nil
	}
	parent := existingParent(resolved)
	if parent == "" {
		view.Valid = false
		view.Message = "无法定位可创建的父目录"
		return view, nil
	}
	info, err := os.Stat(parent)
	if err != nil {
		view.Valid = false
		view.Message = err.Error()
		return view, nil
	}
	view.CanCreate = info.IsDir() && info.Mode().Perm()&0o200 != 0
	view.CanWrite = view.CanCreate
	if !request.CreateIfMissing {
		view.Warnings = append(view.Warnings, "工作目录不存在")
	}
	return view, nil
}

func (m *composeWorkspaceManager) WriteProjectFiles(projectName, workingDir, composeYaml string, buildFiles []ComposeBuildFileCommand, overwrite bool) (*composeWriteResult, error) {
	if strings.TrimSpace(composeYaml) == "" {
		return nil, apperrors.Params("composeYaml 不能为空")
	}
	if max := maxInt(m.cfg.MaxComposeBytes, 1024*1024); len([]byte(composeYaml)) > max {
		return nil, apperrors.Params("composeYaml 超过大小限制")
	}
	if strings.TrimSpace(workingDir) == "" {
		root, err := m.defaultWorkspaceRoot()
		if err != nil {
			return nil, err
		}
		workingDir = filepath.Join(root, safeProjectDirName(projectName))
	}
	resolved, _, err := m.resolveWorkspace(workingDir, true)
	if err != nil {
		return nil, err
	}
	if err := os.MkdirAll(resolved, os.FileMode(defaultUint32(m.cfg.DirMode, 0o750))); err != nil {
		return nil, apperrors.System("创建 Compose 工作目录失败：" + err.Error())
	}
	composeFile := filepath.Join(resolved, firstNonBlank(m.cfg.DefaultFileName, "docker-compose.yaml"))
	if !overwrite {
		if _, err := os.Stat(composeFile); err == nil {
			return nil, apperrors.Params("Compose 文件已存在，未允许覆盖")
		}
	}
	var manifest []composeFileManifestEntry
	if err := atomicWriteFile(composeFile, []byte(strings.TrimSpace(composeYaml)+"\n"), os.FileMode(defaultUint32(m.cfg.FileMode, 0o640))); err != nil {
		return nil, err
	}
	manifest = append(manifest, composeFileManifestEntry{Path: composeFile, Kind: "compose", Size: len([]byte(composeYaml))})
	for _, build := range buildFiles {
		entries, err := m.writeBuildFiles(resolved, build, overwrite)
		if err != nil {
			return nil, err
		}
		manifest = append(manifest, entries...)
	}
	manifestBytes, _ := json.Marshal(manifest)
	configFilesBytes, _ := json.Marshal([]string{composeFile})
	paths := make([]string, 0, len(manifest))
	for _, entry := range manifest {
		paths = append(paths, entry.Path)
	}
	return &composeWriteResult{
		WorkingDir:       resolved,
		ComposeFilePath:  composeFile,
		ConfigFilesJSON:  string(configFilesBytes),
		FileManifestJSON: string(manifestBytes),
		WrittenFilePaths: paths,
	}, nil
}

func (m *composeWorkspaceManager) PrepareSandbox(projectName, workingDir, composeYaml string, buildFiles []ComposeBuildFileCommand) (*composeWriteResult, func(), error) {
	baseDir := strings.TrimSpace(m.cfg.TempDir)
	if baseDir == "" {
		baseDir = os.TempDir()
	}
	dir, err := os.MkdirTemp(baseDir, "docker-compose-builder-")
	if err != nil {
		return nil, nil, apperrors.System("创建 Compose 临时目录失败：" + err.Error())
	}
	cleanup := func() { _ = os.RemoveAll(dir) }
	result, err := (&composeWorkspaceManager{cfg: config.DockerComposeConfig{
		DefaultFileName:            firstNonBlank(m.cfg.DefaultFileName, "docker-compose.yaml"),
		DirMode:                    defaultUint32(m.cfg.DirMode, 0o750),
		FileMode:                   defaultUint32(m.cfg.FileMode, 0o640),
		MaxComposeBytes:            maxInt(m.cfg.MaxComposeBytes, 1024*1024),
		MaxDockerfileBytes:         maxInt(m.cfg.MaxDockerfileBytes, 256*1024),
		MaxExtraFilesBytes:         maxInt(m.cfg.MaxExtraFilesBytes, 2*1024*1024),
		AllowedProjectFileSuffixes: m.cfg.AllowedProjectFileSuffixes,
		WorkspaceRoots:             []string{dir},
	}}).WriteProjectFiles(firstNonBlank(projectName, "preview"), dir, composeYaml, buildFiles, true)
	if err != nil {
		cleanup()
		return nil, nil, err
	}
	return result, cleanup, nil
}

func (m *composeWorkspaceManager) PreviewDockerfile(request DockerfileBuildPreviewRequest) (*DockerfileBuildPreviewView, error) {
	workingDir := request.WorkingDir
	if strings.TrimSpace(workingDir) == "" {
		root, err := m.defaultWorkspaceRoot()
		if err != nil {
			return nil, err
		}
		workingDir = filepath.Join(root, safeProjectDirName(firstNonBlank(request.ProjectName, "preview")))
	}
	resolved, _, err := m.resolveWorkspace(workingDir, true)
	if err != nil {
		return &DockerfileBuildPreviewView{Valid: false, Message: err.Error()}, nil
	}
	build := ComposeBuildFileCommand{
		ServiceName:       request.ServiceName,
		Context:           request.Context,
		DockerfilePath:    request.DockerfilePath,
		DockerfileContent: request.DockerfileContent,
		BuildArgs:         request.BuildArgs,
		ImageTag:          request.ImageTag,
	}
	ctxPath, dockerfilePath, warnings, violations, err := m.validateBuildFile(resolved, build)
	if err != nil {
		violations = append(violations, PolicyViolationVO{Code: "DOCKERFILE_INVALID", Severity: "HIGH", Action: PolicyActionDeny, Field: "dockerfile", Message: err.Error()})
	}
	valid := len(violations) == 0
	message := "Dockerfile 配置可用"
	if !valid {
		message = violations[0].Message
	}
	return &DockerfileBuildPreviewView{
		Valid:                  valid,
		Message:                message,
		ResolvedContext:        ctxPath,
		ResolvedDockerfilePath: dockerfilePath,
		ImageTag:               strings.TrimSpace(request.ImageTag),
		Warnings:               warnings,
		Violations:             violations,
	}, nil
}

func (m *composeWorkspaceManager) writeBuildFiles(workDir string, build ComposeBuildFileCommand, overwrite bool) ([]composeFileManifestEntry, error) {
	ctxPath, dockerfilePath, _, violations, err := m.validateBuildFile(workDir, build)
	if err != nil {
		return nil, err
	}
	if len(violations) > 0 {
		return nil, apperrors.Params(violations[0].Message)
	}
	if err := os.MkdirAll(ctxPath, os.FileMode(defaultUint32(m.cfg.DirMode, 0o750))); err != nil {
		return nil, apperrors.System("创建 Dockerfile context 失败：" + err.Error())
	}
	var manifest []composeFileManifestEntry
	if strings.TrimSpace(build.DockerfileContent) != "" {
		if !overwrite {
			if _, err := os.Stat(dockerfilePath); err == nil {
				return nil, apperrors.Params("Dockerfile 已存在，未允许覆盖")
			}
		}
		content := []byte(build.DockerfileContent)
		if err := atomicWriteFile(dockerfilePath, content, os.FileMode(defaultUint32(m.cfg.FileMode, 0o640))); err != nil {
			return nil, err
		}
		manifest = append(manifest, composeFileManifestEntry{Path: dockerfilePath, ServiceName: strings.TrimSpace(build.ServiceName), Kind: "dockerfile", Size: len(content)})
	}
	totalExtra := 0
	for _, extra := range build.ExtraFiles {
		rel := strings.TrimSpace(extra.Path)
		if err := m.validateProjectRelativeFile(rel); err != nil {
			return nil, err
		}
		content, err := decodeProjectFileContent(extra)
		if err != nil {
			return nil, err
		}
		totalExtra += len(content)
		if max := maxInt(m.cfg.MaxExtraFilesBytes, 2*1024*1024); totalExtra > max {
			return nil, apperrors.Params("extraFiles 总大小超过限制")
		}
		target, err := safeJoin(ctxPath, rel)
		if err != nil {
			return nil, err
		}
		if !overwrite && !extra.Overwrite {
			if _, err := os.Stat(target); err == nil {
				return nil, apperrors.Params("项目文件已存在，未允许覆盖：" + rel)
			}
		}
		if err := os.MkdirAll(filepath.Dir(target), os.FileMode(defaultUint32(m.cfg.DirMode, 0o750))); err != nil {
			return nil, apperrors.System("创建项目文件目录失败：" + err.Error())
		}
		if err := atomicWriteFile(target, content, os.FileMode(defaultUint32(m.cfg.FileMode, 0o640))); err != nil {
			return nil, err
		}
		manifest = append(manifest, composeFileManifestEntry{Path: target, ServiceName: strings.TrimSpace(build.ServiceName), Kind: "extra", Size: len(content)})
	}
	return manifest, nil
}

func (m *composeWorkspaceManager) validateBuildFile(workDir string, build ComposeBuildFileCommand) (string, string, []PolicyViolationVO, []PolicyViolationVO, error) {
	if strings.TrimSpace(build.ServiceName) == "" {
		return "", "", nil, nil, apperrors.Params("serviceName 不能为空")
	}
	contextRel := firstNonBlank(build.Context, ".")
	contextPath, err := safeJoin(workDir, contextRel)
	if err != nil {
		return "", "", nil, nil, err
	}
	if isDangerousWorkspacePath(contextPath) {
		return "", "", nil, nil, apperrors.Params("Dockerfile context 是高危路径")
	}
	dockerfileRel := firstNonBlank(build.DockerfilePath, "Dockerfile")
	dockerfilePath, err := safeJoin(contextPath, dockerfileRel)
	if err != nil {
		return "", "", nil, nil, err
	}
	if strings.TrimSpace(build.DockerfileContent) != "" {
		if max := maxInt(m.cfg.MaxDockerfileBytes, 256*1024); len([]byte(build.DockerfileContent)) > max {
			return "", "", nil, nil, apperrors.Params("Dockerfile 内容超过大小限制")
		}
	}
	var warnings []PolicyViolationVO
	for _, item := range inspectKeyValuePolicy(config.DockerSecurityConfig{}, "buildArgs", build.BuildArgs) {
		item.Action = PolicyActionWarn
		warnings = append(warnings, item)
	}
	if strings.Contains(strings.ToLower(build.DockerfileContent), "add http://") || strings.Contains(strings.ToLower(build.DockerfileContent), "add https://") {
		warnings = append(warnings, PolicyViolationVO{Code: "DOCKERFILE_REMOTE_ADD", Severity: "MEDIUM", Action: PolicyActionWarn, Field: "dockerfileContent", Message: "Dockerfile 使用远程 ADD，建议改为受控下载或固定校验"})
	}
	return contextPath, dockerfilePath, warnings, nil, nil
}

func (m *composeWorkspaceManager) validateProjectRelativeFile(path string) error {
	path = strings.TrimSpace(path)
	if path == "" {
		return apperrors.Params("项目文件路径不能为空")
	}
	if filepath.IsAbs(path) || strings.Contains(path, "..") {
		return apperrors.Params("项目文件路径必须是工作目录内相对路径")
	}
	base := filepath.Base(path)
	ext := filepath.Ext(path)
	allowed := m.cfg.AllowedProjectFileSuffixes
	if len(allowed) == 0 {
		allowed = []string{".env", ".conf", ".json", ".yaml", ".yml", ".sh", "Dockerfile"}
	}
	for _, rule := range allowed {
		rule = strings.TrimSpace(rule)
		if rule == "" {
			continue
		}
		if strings.EqualFold(rule, base) || strings.EqualFold(rule, ext) {
			return nil
		}
	}
	return apperrors.Params("项目文件类型不在白名单内：" + path)
}

func (m *composeWorkspaceManager) composeFileExists(dir string) bool {
	for _, name := range []string{firstNonBlank(m.cfg.DefaultFileName, "docker-compose.yaml"), "docker-compose.yaml", "docker-compose.yml", "compose.yaml"} {
		if _, err := os.Stat(filepath.Join(dir, name)); err == nil {
			return true
		}
	}
	return false
}

func (m *composeWorkspaceManager) resolveWorkspace(path string, allowMissing bool) (string, bool, error) {
	path = strings.TrimSpace(path)
	if path == "" {
		return "", false, apperrors.Params("workingDir 不能为空")
	}
	if strings.Contains(path, "..") {
		return "", false, apperrors.Params("workingDir 不允许包含 ..")
	}
	if !filepath.IsAbs(path) {
		abs, err := filepath.Abs(path)
		if err != nil {
			return "", false, apperrors.Params("workingDir 无法解析")
		}
		path = abs
	}
	path = filepath.Clean(path)
	if isDangerousWorkspacePath(path) {
		return "", false, apperrors.Forbidden("workingDir 是高危系统路径")
	}
	exists := true
	resolved, err := filepath.EvalSymlinks(path)
	if err != nil {
		if !allowMissing && !os.IsNotExist(err) {
			return "", false, apperrors.Params("workingDir 无法解析软链：" + err.Error())
		}
		exists = false
		parent := existingParent(path)
		if parent == "" {
			return "", false, apperrors.Params("workingDir 父目录不存在")
		}
		parentResolved, err := filepath.EvalSymlinks(parent)
		if err != nil {
			return "", false, apperrors.Params("workingDir 父目录无法解析软链：" + err.Error())
		}
		rel, _ := filepath.Rel(parent, path)
		resolved = filepath.Join(parentResolved, rel)
	}
	if !m.pathAllowed(resolved) {
		return "", exists, apperrors.Forbidden("workingDir 不在允许的 Docker Compose 工作目录内")
	}
	return resolved, exists, nil
}

func (m *composeWorkspaceManager) pathAllowed(path string) bool {
	roots := m.workspaceRoots()
	for _, root := range roots {
		rel, err := filepath.Rel(root, path)
		if err == nil && rel != ".." && !strings.HasPrefix(rel, "../") {
			return true
		}
	}
	return false
}

func (m *composeWorkspaceManager) workspaceRoots() []string {
	values := m.cfg.WorkspaceRoots
	if len(values) == 0 {
		values = []string{"data/docker-compose"}
	}
	roots := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		if !filepath.IsAbs(value) {
			if abs, err := filepath.Abs(value); err == nil {
				value = abs
			}
		}
		value = filepath.Clean(value)
		if resolved, err := filepath.EvalSymlinks(value); err == nil {
			value = resolved
		}
		roots = append(roots, value)
	}
	sort.Strings(roots)
	return roots
}

func (m *composeWorkspaceManager) defaultWorkspaceRoot() (string, error) {
	roots := m.workspaceRoots()
	if len(roots) == 0 {
		return "", apperrors.System("docker.compose.workspaceRoots 未配置")
	}
	return roots[0], nil
}

func (s *service) CheckComposeWorkspace(ctx context.Context, request ComposeWorkspaceCheckRequest) (*ComposeWorkspaceCheckView, error) {
	_ = ctx
	return newComposeWorkspaceManager(s.cfg.Compose).Check(request)
}

func (s *service) PreviewDockerfileBuild(ctx context.Context, request DockerfileBuildPreviewRequest) (*DockerfileBuildPreviewView, error) {
	_ = ctx
	return newComposeWorkspaceManager(s.cfg.Compose).PreviewDockerfile(request)
}

func (s *service) ValidateComposeYaml(ctx context.Context, request ComposeYamlValidateRequest) (*ComposeYamlValidateView, error) {
	composeYaml := strings.TrimSpace(request.ComposeYaml)
	if composeYaml == "" {
		return &ComposeYamlValidateView{Valid: false, Message: "composeYaml 不能为空"}, nil
	}
	if max := maxInt(s.cfg.Compose.MaxComposeBytes, 1024*1024); len([]byte(composeYaml)) > max {
		return &ComposeYamlValidateView{Valid: false, Message: "composeYaml 超过大小限制"}, nil
	}
	parsed, err := parseComposeYamlForBuilder(composeYaml)
	if err != nil {
		return &ComposeYamlValidateView{Valid: false, Message: err.Error()}, nil
	}
	validation, err := s.ValidateCompose(ctx, ComposeUpRequest{ProjectName: request.ProjectName, ComposeYaml: composeYaml})
	if err != nil {
		return nil, err
	}
	parsed.Valid = validation.Valid
	parsed.Message = firstNonBlank(validation.Message, parsed.Message)
	parsed.NormalizedYaml = validation.NormalizedYaml
	return parsed, nil
}

func (s *service) GetComposeBuilderMetadata(ctx context.Context) (*ComposeBuilderMetadataView, error) {
	_ = ctx
	roots := newComposeWorkspaceManager(s.cfg.Compose).workspaceRoots()
	defaultRoot := ""
	if len(roots) > 0 {
		defaultRoot = roots[0]
	}
	supported := []string{
		"image", "build", "container_name", "ports", "environment", "volumes", "networks", "depends_on",
		"restart", "command", "working_dir", "user", "healthcheck", "deploy.resources", "privileged",
		"network_mode", "pid", "ipc", "cap_add", "cap_drop", "labels", "devices", "extra_hosts", "dns",
	}
	return &ComposeBuilderMetadataView{
		WorkspaceRoots:             roots,
		DefaultWorkspaceRoot:       defaultRoot,
		DefaultFileName:            firstNonBlank(s.cfg.Compose.DefaultFileName, "docker-compose.yaml"),
		MaxComposeBytes:            maxInt(s.cfg.Compose.MaxComposeBytes, 1024*1024),
		MaxDockerfileBytes:         maxInt(s.cfg.Compose.MaxDockerfileBytes, 256*1024),
		MaxExtraFilesBytes:         maxInt(s.cfg.Compose.MaxExtraFilesBytes, 2*1024*1024),
		AllowedProjectFileSuffixes: append([]string{}, s.cfg.Compose.AllowedProjectFileSuffixes...),
		RestartPolicies:            []string{"no", "always", "unless-stopped", "on-failure"},
		NetworkModes:               []string{"bridge", "host", "none"},
		SupportedServiceFields:     supported,
		DefaultService: ComposeBuilderDefaultServiceView{
			Restart:     "unless-stopped",
			NetworkMode: "bridge",
		},
		HealthcheckDefaults: ComposeHealthcheckDefaultsView{
			Interval:    "30s",
			Timeout:     "5s",
			Retries:     3,
			StartPeriod: "10s",
		},
		ResourceLimitHints: ComposeResourceLimitHintsView{
			CPUExamples:    []string{"0.5", "1.0", "2.0"},
			MemoryExamples: []string{"256M", "512M", "1G"},
		},
	}, nil
}

func (s *service) PreviewComposeWithFiles(ctx context.Context, actor OperationActor, request ComposePreviewWithFilesRequest) (*ComposePreviewView, error) {
	manager := newComposeWorkspaceManager(s.cfg.Compose)
	prepared, cleanup, err := manager.PrepareSandbox(request.ProjectName, request.WorkingDir, request.ComposeYaml, request.BuildFiles)
	if err != nil {
		return nil, err
	}
	defer cleanup()
	return s.PreviewCompose(ctx, actor, ComposeUpRequest{
		ProjectName:     request.ProjectName,
		ComposeYaml:     request.ComposeYaml,
		WorkingDir:      prepared.WorkingDir,
		ComposeFilePath: prepared.ComposeFilePath,
	})
}

func parseComposeYamlForBuilder(raw string) (*ComposeYamlValidateView, error) {
	var root map[string]any
	if err := yaml.Unmarshal([]byte(raw), &root); err != nil {
		return nil, apperrors.Params("解析 compose YAML 失败：" + err.Error())
	}
	servicesNode, ok := root["services"].(map[string]any)
	if !ok || len(servicesNode) == 0 {
		return nil, apperrors.Params("compose YAML 必须包含 services")
	}
	view := &ComposeYamlValidateView{Valid: true, Message: "YAML 解析成功", VisualDraft: &ComposeVisualDraftView{Version: stringValue(root["version"])}}
	for key := range root {
		switch key {
		case "version", "name", "services", "networks", "volumes", "secrets", "configs":
		default:
			view.UnsupportedFields = append(view.UnsupportedFields, ComposeUnsupportedFieldVO{Path: key, Value: root[key], Reason: "顶层字段暂不支持可视化编辑"})
		}
	}
	for name, rawService := range servicesNode {
		serviceMap, _ := rawService.(map[string]any)
		service := ComposeServiceVO{
			ServiceName:    strings.TrimSpace(name),
			Image:          stringValue(serviceMap["image"]),
			ContainerCount: 0,
			Status:         ComposeProjectStatusUnknown,
			Ports:          parseComposePorts(serviceMap["ports"]),
		}
		view.Services = append(view.Services, service)
		visualService := ComposeVisualServiceView{
			ServiceName:   strings.TrimSpace(name),
			Image:         stringValue(serviceMap["image"]),
			ContainerName: stringValue(serviceMap["container_name"]),
			Ports:         parseVisualPorts(serviceMap["ports"]),
			Environment:   parseVisualEnvironment(serviceMap["environment"]),
			Volumes:       parseVisualVolumes(serviceMap["volumes"]),
			Networks:      parseStringSlice(serviceMap["networks"]),
			DependsOn:     parseStringSlice(serviceMap["depends_on"]),
			Restart:       stringValue(serviceMap["restart"]),
			Command:       normalizeScalarOrSlice(serviceMap["command"]),
			WorkingDir:    stringValue(serviceMap["working_dir"]),
			User:          stringValue(serviceMap["user"]),
		}
		if build := parseVisualBuild(serviceMap["build"]); build != nil {
			visualService.Build = build
		}
		if healthcheck := parseVisualHealthcheck(serviceMap["healthcheck"]); healthcheck != nil {
			visualService.Healthcheck = healthcheck
		}
		if resources := parseVisualResources(serviceMap); resources != nil {
			visualService.Resources = resources
		}
		if advanced := parseVisualAdvanced(serviceMap); advanced != nil {
			visualService.Advanced = advanced
		}
		supported := map[string]struct{}{
			"image": {}, "build": {}, "container_name": {}, "ports": {}, "environment": {}, "volumes": {}, "networks": {},
			"depends_on": {}, "restart": {}, "command": {}, "working_dir": {}, "user": {}, "healthcheck": {}, "deploy": {},
			"privileged": {}, "network_mode": {}, "pid": {}, "ipc": {}, "cap_add": {}, "cap_drop": {}, "labels": {},
			"devices": {}, "extra_hosts": {}, "dns": {}, "mem_limit": {}, "memswap_limit": {}, "cpus": {}, "pids_limit": {},
		}
		for field, value := range serviceMap {
			if _, ok := supported[field]; !ok {
				view.UnsupportedFields = append(view.UnsupportedFields, ComposeUnsupportedFieldVO{Path: "services." + name + "." + field, Value: value, Reason: "服务字段暂不支持可视化编辑"})
				visualService.UnsupportedFields = append(visualService.UnsupportedFields, ComposeUnsupportedFieldVO{Path: "services." + name + "." + field, Value: value, Reason: "服务字段暂不支持可视化编辑"})
			}
		}
		view.VisualDraft.Services = append(view.VisualDraft.Services, visualService)
	}
	view.Networks = topLevelNames(root["networks"])
	view.Volumes = topLevelNames(root["volumes"])
	view.VisualDraft.Networks = parseVisualNetworks(root["networks"])
	view.VisualDraft.Volumes = parseVisualVolumesTop(root["volumes"])
	sort.SliceStable(view.Services, func(i, j int) bool { return view.Services[i].ServiceName < view.Services[j].ServiceName })
	sort.SliceStable(view.VisualDraft.Services, func(i, j int) bool {
		return view.VisualDraft.Services[i].ServiceName < view.VisualDraft.Services[j].ServiceName
	})
	sort.Strings(view.Networks)
	sort.Strings(view.Volumes)
	return view, nil
}

func parseVisualPorts(raw any) []ComposeVisualPortView {
	items, ok := raw.([]any)
	if !ok {
		return nil
	}
	result := make([]ComposeVisualPortView, 0, len(items))
	for _, item := range items {
		switch value := item.(type) {
		case string:
			port := parseComposePortString(value)
			result = append(result, ComposeVisualPortView{
				HostIP:        port.IP,
				HostPort:      fmt.Sprint(port.PublicPort),
				ContainerPort: fmt.Sprint(port.PrivatePort),
				Protocol:      port.Type,
			})
		case map[string]any:
			result = append(result, ComposeVisualPortView{
				HostIP:        stringValue(value["host_ip"]),
				HostPort:      fmt.Sprint(intValue(value["published"])),
				ContainerPort: fmt.Sprint(intValue(value["target"])),
				Protocol:      firstNonBlank(stringValue(value["protocol"]), "tcp"),
			})
		}
	}
	return result
}

func parseVisualEnvironment(raw any) []KeyValueCommand {
	switch value := raw.(type) {
	case map[string]any:
		result := make([]KeyValueCommand, 0, len(value))
		for key, v := range value {
			result = append(result, KeyValueCommand{Key: key, Value: fmt.Sprint(v)})
		}
		sort.SliceStable(result, func(i, j int) bool { return result[i].Key < result[j].Key })
		return result
	case []any:
		result := make([]KeyValueCommand, 0, len(value))
		for _, item := range value {
			if s := stringValue(item); s != "" {
				key, val, ok := strings.Cut(s, "=")
				if ok {
					result = append(result, KeyValueCommand{Key: key, Value: val})
				} else {
					result = append(result, KeyValueCommand{Key: s})
				}
			}
		}
		return result
	default:
		return nil
	}
}

func parseVisualVolumes(raw any) []ComposeVisualVolumeMountView {
	items, ok := raw.([]any)
	if !ok {
		return nil
	}
	result := make([]ComposeVisualVolumeMountView, 0, len(items))
	for _, item := range items {
		switch value := item.(type) {
		case string:
			result = append(result, parseVisualVolumeString(value))
		case map[string]any:
			result = append(result, ComposeVisualVolumeMountView{
				Source:   stringValue(value["source"]),
				Target:   stringValue(value["target"]),
				Type:     stringValue(value["type"]),
				ReadOnly: boolValueAny(value["read_only"]),
			})
		}
	}
	return result
}

func parseVisualNetworks(raw any) []ComposeVisualNetworkView {
	switch value := raw.(type) {
	case map[string]any:
		result := make([]ComposeVisualNetworkView, 0, len(value))
		for key, rawItem := range value {
			item, _ := rawItem.(map[string]any)
			result = append(result, ComposeVisualNetworkView{
				Name:     key,
				Driver:   stringValue(item["driver"]),
				External: boolValueAny(item["external"]),
			})
		}
		sort.SliceStable(result, func(i, j int) bool { return result[i].Name < result[j].Name })
		return result
	case []any:
		result := make([]ComposeVisualNetworkView, 0, len(value))
		for _, item := range value {
			if s := stringValue(item); s != "" {
				result = append(result, ComposeVisualNetworkView{Name: s})
			}
		}
		return result
	default:
		return nil
	}
}

func parseVisualVolumesTop(raw any) []ComposeVisualVolumeView {
	switch value := raw.(type) {
	case map[string]any:
		result := make([]ComposeVisualVolumeView, 0, len(value))
		for key, rawItem := range value {
			item, _ := rawItem.(map[string]any)
			result = append(result, ComposeVisualVolumeView{
				Name:     key,
				Driver:   stringValue(item["driver"]),
				External: boolValueAny(item["external"]),
			})
		}
		sort.SliceStable(result, func(i, j int) bool { return result[i].Name < result[j].Name })
		return result
	case []any:
		result := make([]ComposeVisualVolumeView, 0, len(value))
		for _, item := range value {
			if s := stringValue(item); s != "" {
				result = append(result, ComposeVisualVolumeView{Name: s})
			}
		}
		return result
	default:
		return nil
	}
}

func parseVisualBuild(raw any) *ComposeVisualBuildView {
	switch value := raw.(type) {
	case string:
		return &ComposeVisualBuildView{Context: value}
	case map[string]any:
		args := map[string]string{}
		if rawArgs, ok := value["args"].(map[string]any); ok {
			for key, v := range rawArgs {
				args[key] = fmt.Sprint(v)
			}
		}
		view := &ComposeVisualBuildView{
			Context:    stringValue(value["context"]),
			Dockerfile: stringValue(value["dockerfile"]),
			Args:       args,
		}
		if len(view.Args) == 0 {
			view.Args = nil
		}
		return view
	default:
		return nil
	}
}

func parseVisualHealthcheck(raw any) *ComposeVisualHealthcheckView {
	serviceMap, ok := raw.(map[string]any)
	if !ok {
		return nil
	}
	return &ComposeVisualHealthcheckView{
		Test:        normalizeScalarOrSlice(serviceMap["test"]),
		Interval:    stringValue(serviceMap["interval"]),
		Timeout:     stringValue(serviceMap["timeout"]),
		Retries:     intValue(serviceMap["retries"]),
		StartPeriod: stringValue(serviceMap["start_period"]),
		Disable:     boolValueAny(serviceMap["disable"]),
	}
}

func parseVisualResources(serviceMap map[string]any) *ComposeVisualResourcesView {
	if serviceMap == nil {
		return nil
	}
	resources := &ComposeVisualResourcesView{
		CPUs:              firstNonBlank(stringValue(serviceMap["cpus"]), stringValue(deployResourceValue(serviceMap, "limits", "cpus"))),
		Memory:            firstNonBlank(stringValue(serviceMap["mem_limit"]), stringValue(deployResourceValue(serviceMap, "limits", "memory"))),
		MemoryReservation: firstNonBlank(stringValue(serviceMap["mem_reservation"]), stringValue(deployResourceValue(serviceMap, "reservations", "memory"))),
		PidsLimit:         int64(intValue(serviceMap["pids_limit"])),
	}
	if resources.CPUs == "" && resources.Memory == "" && resources.MemoryReservation == "" && resources.PidsLimit == 0 {
		return nil
	}
	return resources
}

func parseVisualAdvanced(serviceMap map[string]any) *ComposeVisualAdvancedView {
	if serviceMap == nil {
		return nil
	}
	view := &ComposeVisualAdvancedView{
		Privileged:  boolValueAny(serviceMap["privileged"]),
		NetworkMode: stringValue(serviceMap["network_mode"]),
		PID:         stringValue(serviceMap["pid"]),
		IPC:         stringValue(serviceMap["ipc"]),
		CapAdd:      parseStringSlice(serviceMap["cap_add"]),
		CapDrop:     parseStringSlice(serviceMap["cap_drop"]),
	}
	if !view.Privileged && view.NetworkMode == "" && view.PID == "" && view.IPC == "" && len(view.CapAdd) == 0 && len(view.CapDrop) == 0 {
		return nil
	}
	return view
}

func parseVisualVolumeString(value string) ComposeVisualVolumeMountView {
	value = strings.TrimSpace(value)
	if value == "" {
		return ComposeVisualVolumeMountView{}
	}
	parts := strings.Split(value, ":")
	switch len(parts) {
	case 1:
		return ComposeVisualVolumeMountView{Source: parts[0]}
	case 2:
		return ComposeVisualVolumeMountView{Source: parts[0], Target: parts[1]}
	default:
		return ComposeVisualVolumeMountView{Source: parts[0], Target: parts[len(parts)-1]}
	}
}

func parseStringSlice(raw any) []string {
	switch value := raw.(type) {
	case []any:
		result := make([]string, 0, len(value))
		for _, item := range value {
			if s := stringValue(item); s != "" {
				result = append(result, s)
			}
		}
		return result
	case map[string]any:
		result := make([]string, 0, len(value))
		for key := range value {
			result = append(result, key)
		}
		sort.Strings(result)
		return result
	case string:
		if strings.TrimSpace(value) == "" {
			return nil
		}
		return []string{strings.TrimSpace(value)}
	default:
		return nil
	}
}

func normalizeScalarOrSlice(raw any) any {
	switch value := raw.(type) {
	case []any:
		result := make([]string, 0, len(value))
		for _, item := range value {
			result = append(result, fmt.Sprint(item))
		}
		return result
	case string:
		return value
	default:
		if value == nil {
			return nil
		}
		return fmt.Sprint(value)
	}
}

func deployResourceValue(serviceMap map[string]any, bucket, field string) any {
	deploy, _ := serviceMap["deploy"].(map[string]any)
	if deploy == nil {
		return nil
	}
	resources, _ := deploy["resources"].(map[string]any)
	if resources == nil {
		return nil
	}
	nested, _ := resources[bucket].(map[string]any)
	if nested == nil {
		return nil
	}
	return nested[field]
}

func boolValueAny(raw any) bool {
	switch value := raw.(type) {
	case bool:
		return value
	case string:
		normalized := strings.ToLower(strings.TrimSpace(value))
		return normalized == "true" || normalized == "1" || normalized == "yes"
	case int:
		return value != 0
	case int64:
		return value != 0
	case float64:
		return value != 0
	default:
		return false
	}
}

func parseComposePorts(raw any) []ContainerPortView {
	items, ok := raw.([]any)
	if !ok {
		return nil
	}
	result := make([]ContainerPortView, 0, len(items))
	for _, item := range items {
		switch value := item.(type) {
		case string:
			result = append(result, parseComposePortString(value))
		case map[string]any:
			result = append(result, ContainerPortView{
				IP:          stringValue(value["host_ip"]),
				PublicPort:  uint16(intValue(value["published"])),
				PrivatePort: uint16(intValue(value["target"])),
				Type:        firstNonBlank(stringValue(value["protocol"]), "tcp"),
			})
		}
	}
	return result
}

func parseComposePortString(value string) ContainerPortView {
	value = strings.TrimSpace(value)
	protocol := "tcp"
	if left, right, ok := strings.Cut(value, "/"); ok {
		value = left
		protocol = right
	}
	parts := strings.Split(value, ":")
	view := ContainerPortView{Type: protocol}
	if len(parts) == 1 {
		view.PrivatePort = uint16(intValue(parts[0]))
		return view
	}
	view.PrivatePort = uint16(intValue(parts[len(parts)-1]))
	view.PublicPort = uint16(intValue(parts[len(parts)-2]))
	if len(parts) > 2 {
		view.IP = parts[0]
	}
	return view
}

func topLevelNames(raw any) []string {
	switch typed := raw.(type) {
	case map[string]any:
		result := make([]string, 0, len(typed))
		for key := range typed {
			result = append(result, key)
		}
		return result
	case []any:
		result := make([]string, 0, len(typed))
		for _, item := range typed {
			if value := stringValue(item); value != "" {
				result = append(result, value)
			}
		}
		return result
	default:
		return nil
	}
}

func safeJoin(base, rel string) (string, error) {
	rel = strings.TrimSpace(rel)
	if rel == "" {
		rel = "."
	}
	if filepath.IsAbs(rel) || strings.Contains(rel, "..") {
		return "", apperrors.Params("路径必须是工作目录内相对路径")
	}
	target := filepath.Clean(filepath.Join(base, rel))
	relToBase, err := filepath.Rel(base, target)
	if err != nil || relToBase == ".." || strings.HasPrefix(relToBase, "../") {
		return "", apperrors.Params("路径不能逃逸工作目录")
	}
	return target, nil
}

func existingParent(path string) string {
	path = filepath.Clean(path)
	for {
		if info, err := os.Stat(path); err == nil && info.IsDir() {
			return path
		}
		next := filepath.Dir(path)
		if next == path {
			return ""
		}
		path = next
	}
}

func isDangerousWorkspacePath(path string) bool {
	clean := filepath.Clean(path)
	for _, dangerous := range []string{"/", "/etc", "/root", "/var/run", "/usr", "/bin", "/sbin", "/proc", "/sys", "/dev"} {
		if clean == dangerous || strings.HasPrefix(clean, dangerous+"/") {
			return true
		}
	}
	return false
}

func safeProjectDirName(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		value = "compose-project"
	}
	replacer := strings.NewReplacer("/", "-", "\\", "-", ":", "-", " ", "-")
	return replacer.Replace(value)
}

func atomicWriteFile(path string, content []byte, mode os.FileMode) error {
	tmp, err := os.CreateTemp(filepath.Dir(path), "."+filepath.Base(path)+".tmp-")
	if err != nil {
		return apperrors.System("创建临时文件失败：" + err.Error())
	}
	tmpName := tmp.Name()
	defer func() { _ = os.Remove(tmpName) }()
	if _, err := tmp.Write(content); err != nil {
		_ = tmp.Close()
		return apperrors.System("写入临时文件失败：" + err.Error())
	}
	if err := tmp.Chmod(mode); err != nil {
		_ = tmp.Close()
		return apperrors.System("设置文件权限失败：" + err.Error())
	}
	if err := tmp.Close(); err != nil {
		return apperrors.System("关闭临时文件失败：" + err.Error())
	}
	if err := os.Rename(tmpName, path); err != nil {
		return apperrors.System("写入文件失败：" + err.Error())
	}
	return nil
}

func decodeProjectFileContent(file ComposeProjectFileCommand) ([]byte, error) {
	encoding := strings.ToLower(strings.TrimSpace(file.Encoding))
	switch encoding {
	case "", "utf-8", "utf8":
		return []byte(file.Content), nil
	case "base64":
		content, err := base64.StdEncoding.DecodeString(file.Content)
		if err != nil {
			return nil, apperrors.Params("base64 文件内容无效")
		}
		return content, nil
	default:
		return nil, apperrors.Params("不支持的文件编码：" + file.Encoding)
	}
}

func defaultUint32(value, fallback uint32) uint32 {
	if value == 0 {
		return fallback
	}
	return value
}
