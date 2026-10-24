package docker

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	apperrors "github.com/CaixyPromise/seven-agent/force/internal/common/errors"
	dockercontainer "github.com/docker/docker/api/types/container"
	"go.yaml.in/yaml/v3"
)

func (s *service) ListComposeProjects(ctx context.Context, current, size int64, keyword, status string) (*PageResult[ComposeProjectSummaryVO], error) {
	records, err := s.managedComposeProjects(ctx)
	if err != nil {
		return nil, err
	}
	containers, err := s.allContainerViews(ctx)
	if err != nil {
		return nil, err
	}
	discovered := discoverComposeProjects(containers)
	exactOperationIDs := make([]int64, 0, len(records))
	targetIDs := make([]string, 0, len(records)+len(discovered))
	for _, row := range records {
		if row.LastOperationID.Valid {
			exactOperationIDs = append(exactOperationIDs, row.LastOperationID.Int64)
		}
		targetIDs = append(targetIDs, row.ProjectID)
	}
	for _, summary := range discovered {
		targetIDs = append(targetIDs, summary.ProjectID)
	}
	operationsByID := map[int64]OperationRecord{}
	latestByTargetID := map[string]OperationRecord{}
	if s.operations != nil {
		operationsByID, err = s.operations.FindOperationsByIDs(ctx, exactOperationIDs)
		if err != nil {
			return nil, err
		}
		latestByTargetID, err = s.operations.LatestOperationsByTargetIDs(ctx, "compose", targetIDs)
		if err != nil {
			return nil, err
		}
	}
	summaries := make([]ComposeProjectSummaryVO, 0, len(records)+len(discovered))
	seenRuntimeKeys := map[string]struct{}{}
	for _, row := range records {
		projectContainers := matchComposeContainers(row.ProjectName, containers)
		var latest *OperationRecord
		if row.LastOperationID.Valid {
			if item, ok := operationsByID[row.LastOperationID.Int64]; ok {
				copyItem := item
				latest = &copyItem
			}
		}
		if latest == nil {
			if item, ok := latestByTargetID[row.ProjectID]; ok {
				copyItem := item
				latest = &copyItem
			}
		}
		summary := s.composeProjectSummary(row, projectContainers, latest)
		summaries = append(summaries, summary)
		seenRuntimeKeys[composeRuntimeKey(row.ProjectName, row.WorkingDir.String, row.ConfigFilesJSON.String)] = struct{}{}
	}
	for _, summary := range discovered {
		if _, ok := seenRuntimeKeys[composeRuntimeKey(summary.ProjectName, summary.WorkingDir, strings.Join(summary.ConfigFiles, "\n"))]; ok {
			continue
		}
		if _, ok := findManagedByProjectName(records, summary.ProjectName); ok {
			continue
		}
		if latest, ok := latestByTargetID[summary.ProjectID]; ok {
			applyLatestOperation(&summary, operationVO(latest))
		}
		summaries = append(summaries, summary)
	}
	filtered := make([]ComposeProjectSummaryVO, 0, len(summaries))
	for _, item := range summaries {
		if !matchComposeProject(item, keyword, status) {
			continue
		}
		filtered = append(filtered, item)
	}
	sort.SliceStable(filtered, func(i, j int) bool {
		if filtered[i].UpdatedAt != filtered[j].UpdatedAt {
			return filtered[i].UpdatedAt > filtered[j].UpdatedAt
		}
		return filtered[i].ProjectName < filtered[j].ProjectName
	})
	return paginate(filtered, current, size), nil
}

func (s *service) GetComposeProject(ctx context.Context, projectID string) (*ComposeProjectDetailVO, error) {
	projectID = strings.TrimSpace(projectID)
	if projectID == "" {
		return nil, apperrors.Params("projectId 不能为空")
	}
	records, err := s.managedComposeProjects(ctx)
	if err != nil {
		return nil, err
	}
	containers, err := s.allContainerViews(ctx)
	if err != nil {
		return nil, err
	}
	if row, ok := findManagedByProjectID(records, projectID); ok {
		detail := s.composeProjectDetail(ctx, row, matchComposeContainers(row.ProjectName, containers))
		return &detail, nil
	}
	for _, summary := range discoverComposeProjects(containers) {
		if summary.ProjectID != projectID {
			continue
		}
		detail := ComposeProjectDetailVO{
			ProjectID:    summary.ProjectID,
			ProjectName:  summary.ProjectName,
			Source:       summary.Source,
			WorkingDir:   summary.WorkingDir,
			ConfigFiles:  summary.ConfigFiles,
			FileManifest: nil,
			Services:     buildComposeServiceViews("", matchComposeContainers(summary.ProjectName, containers), nil),
			Containers:   matchComposeContainers(summary.ProjectName, containers),
		}
		if latest, _ := s.latestOperationForCompose(ctx, summary.ProjectID, summary.ProjectName, ""); latest != nil {
			detail.LastOperation = latest
			detail.ActiveOperation = activeOperationSummary(latest)
			detail.RecentEvents, _ = s.ListOperationEvents(ctx, latest.OperationID, 0, 100)
		}
		detail.AvailableActions = composeAvailableActions(detail.Source, composeStatus(detail.Containers), len(detail.Containers), false, detail.ActiveOperation != nil)
		return &detail, nil
	}
	return nil, apperrors.NotFound("Docker Compose 项目不存在")
}

func (s *service) CreateComposeProject(ctx context.Context, actor OperationActor, request ComposeProjectCreateRequest) (*ComposeProjectCreateResult, error) {
	if s.projects == nil {
		return nil, apperrors.Operation("Docker compose project repository 未配置 datasource")
	}
	projectName := strings.TrimSpace(request.ProjectName)
	composeYaml := strings.TrimSpace(request.ComposeYaml)
	if projectName == "" {
		return nil, apperrors.Params("projectName 不能为空")
	}
	if composeYaml == "" {
		return nil, apperrors.Params("composeYaml 不能为空")
	}
	exists, err := s.projects.ProjectNameExists(ctx, projectName, "")
	if err != nil {
		return nil, err
	}
	if exists {
		return nil, apperrors.Params("Compose 项目名已存在")
	}
	projectID := "compose_" + strings.ToLower(strconvBase36(s.idGen.NextID()))
	composeReq := ComposeUpRequest{ProjectName: projectName, ComposeYaml: composeYaml}
	var writeResult *composeWriteResult
	if request.WriteFiles {
		var cleanup func()
		writeResult, cleanup, err = newComposeWorkspaceManager(s.cfg.Compose).PrepareSandbox(projectName, request.WorkingDir, composeYaml, request.BuildFiles)
		if err != nil {
			return nil, err
		}
		defer cleanup()
		composeReq.WorkingDir = writeResult.WorkingDir
		composeReq.ComposeFilePath = writeResult.ComposeFilePath
	}
	preview, err := s.PreviewCompose(ctx, actor, composeReq)
	if err != nil {
		return nil, err
	}
	if request.WriteFiles {
		writeResult, err = newComposeWorkspaceManager(s.cfg.Compose).WriteProjectFiles(projectName, request.WorkingDir, composeYaml, request.BuildFiles, request.OverwriteExisting)
		if err != nil {
			return nil, err
		}
	}
	previewJSON, validationJSON := composeDiagnosticsJSON(preview)
	configFiles := []string{}
	if writeResult != nil {
		configFiles = []string{writeResult.ComposeFilePath}
	} else if strings.TrimSpace(request.WorkingDir) != "" {
		configFiles = []string{filepath.Join(strings.TrimSpace(request.WorkingDir), "docker-compose.yml")}
	}
	configFilesJSON, _ := json.Marshal(configFiles)
	row := ComposeProjectRecord{
		ID:                 s.idGen.NextID(),
		ProjectID:          projectID,
		ProjectName:        projectName,
		WorkingDir:         sql.NullString{String: firstNonBlank(resultWorkingDir(writeResult), request.WorkingDir), Valid: firstNonBlank(resultWorkingDir(writeResult), request.WorkingDir) != ""},
		ConfigFilesJSON:    sql.NullString{String: firstNonBlank(resultConfigFilesJSON(writeResult), string(configFilesJSON)), Valid: len(configFiles) > 0 || resultConfigFilesJSON(writeResult) != ""},
		ComposeYaml:        sql.NullString{String: composeYaml, Valid: true},
		ComposeFilePath:    sql.NullString{String: resultComposeFilePath(writeResult), Valid: resultComposeFilePath(writeResult) != ""},
		FileManifestJSON:   sql.NullString{String: resultFileManifestJSON(writeResult), Valid: resultFileManifestJSON(writeResult) != ""},
		Description:        sql.NullString{String: strings.TrimSpace(request.Description), Valid: strings.TrimSpace(request.Description) != ""},
		Status:             string(ComposeProjectStatusUnknown),
		LastPreviewJSON:    sql.NullString{String: previewJSON, Valid: previewJSON != ""},
		LastValidationJSON: sql.NullString{String: validationJSON, Valid: validationJSON != ""},
		Source:             string(ComposeProjectSourceManaged),
		CreatedBy:          sql.NullInt64{Int64: actor.UserID, Valid: actor.UserID > 0},
	}
	if err := s.projects.Insert(ctx, row); err != nil {
		return nil, err
	}
	result := &ComposeProjectCreateResult{ProjectID: projectID, ProjectName: projectName}
	if request.AutoUp {
		accepted, err := s.SubmitComposeProjectOperation(ctx, actor, projectID, OperationTypeComposeUp, 0)
		if err != nil {
			return nil, err
		}
		if accepted != nil {
			result.OperationID = accepted.OperationID
		}
	}
	return result, nil
}

func (s *service) ImportDiscoveredComposeProject(ctx context.Context, actor OperationActor, request DockerComposeImportDiscoveredRequest) (*ComposeProjectDetailVO, error) {
	if s.projects == nil {
		return nil, apperrors.Operation("Docker compose project repository 未配置 datasource")
	}
	if s.idGen == nil {
		return nil, apperrors.System("docker compose project id generator is not configured")
	}
	containers, err := s.allContainerViews(ctx)
	if err != nil {
		return nil, err
	}
	summary, ok := findDiscoveredComposeProject(discoverComposeProjects(containers), request)
	if !ok {
		return nil, apperrors.NotFound("未找到可导入的 Docker Compose 运行时项目")
	}
	if exists, err := s.projects.ProjectNameExists(ctx, summary.ProjectName, ""); err != nil {
		return nil, err
	} else if exists {
		return nil, apperrors.Params("Compose 项目名已存在")
	}
	resolved, err := resolveDiscoveredComposeFile(summary.WorkingDir, summary.ConfigFiles, maxInt(s.cfg.Compose.MaxComposeBytes, 1024*1024))
	if err != nil {
		return nil, err
	}
	projectID := "compose_" + strings.ToLower(strconvBase36(s.idGen.NextID()))
	configFilesJSON, _ := json.Marshal([]string{resolved.Path})
	manifestJSON, _ := json.Marshal([]map[string]any{{
		"path":   resolved.Path,
		"kind":   "compose",
		"size":   resolved.Size,
		"sha256": resolved.SHA256,
	}})
	validationJSON, _ := json.Marshal(ComposeValidationView{Valid: true, Message: "discovered compose file imported"})
	row := ComposeProjectRecord{
		ID:                 s.idGen.NextID(),
		ProjectID:          projectID,
		ProjectName:        summary.ProjectName,
		WorkingDir:         sql.NullString{String: filepath.Dir(resolved.Path), Valid: true},
		ConfigFilesJSON:    sql.NullString{String: string(configFilesJSON), Valid: true},
		ComposeYaml:        sql.NullString{String: resolved.Content, Valid: true},
		ComposeFilePath:    sql.NullString{String: resolved.Path, Valid: true},
		FileManifestJSON:   sql.NullString{String: string(manifestJSON), Valid: true},
		Status:             string(ComposeProjectStatusUnknown),
		LastValidationJSON: sql.NullString{String: string(validationJSON), Valid: true},
		Source:             string(ComposeProjectSourceManaged),
		CreatedBy:          sql.NullInt64{Int64: actor.UserID, Valid: actor.UserID > 0},
	}
	if err := s.projects.Insert(ctx, row); err != nil {
		return nil, err
	}
	return s.GetComposeProject(ctx, projectID)
}

func (s *service) UpdateComposeProjectCompose(ctx context.Context, actor OperationActor, projectID string, request ComposeProjectUpdateRequest) (*ComposeProjectDetailVO, error) {
	row, err := s.requireManagedComposeProject(ctx, projectID)
	if err != nil {
		return nil, err
	}
	composeYaml := strings.TrimSpace(request.ComposeYaml)
	if composeYaml == "" {
		return nil, apperrors.Params("composeYaml 不能为空")
	}
	composeReq := ComposeUpRequest{ProjectName: row.ProjectName, ComposeYaml: composeYaml}
	var writeResult *composeWriteResult
	externalComposeFile := strings.TrimSpace(row.ComposeFilePath.String)
	externalWrite := false
	if request.WriteFiles && externalComposeFile != "" {
		manager := newComposeWorkspaceManager(s.cfg.Compose)
		externalWrite = !manager.pathAllowed(filepath.Dir(externalComposeFile))
		if externalWrite {
			if len(request.BuildFiles) > 0 {
				return nil, apperrors.Params("外部 Compose 文件托管暂不支持写入附加 build 文件")
			}
			composeReq.WorkingDir = filepath.Dir(externalComposeFile)
			composeReq.ComposeFilePath = externalComposeFile
		}
	}
	if request.WriteFiles {
		if !externalWrite {
			var cleanup func()
			writeResult, cleanup, err = newComposeWorkspaceManager(s.cfg.Compose).PrepareSandbox(row.ProjectName, row.WorkingDir.String, composeYaml, request.BuildFiles)
			if err != nil {
				return nil, err
			}
			defer cleanup()
			composeReq.WorkingDir = writeResult.WorkingDir
			composeReq.ComposeFilePath = writeResult.ComposeFilePath
		}
	}
	preview, err := s.PreviewCompose(ctx, actor, composeReq)
	if err != nil {
		return nil, err
	}
	if request.ValidateBeforeSave && !preview.Validation.Valid {
		return nil, apperrors.Params(firstNonBlank(preview.Validation.Message, "Compose 配置校验失败"))
	}
	previewJSON, validationJSON := composeDiagnosticsJSON(preview)
	if request.WriteFiles {
		if externalWrite {
			writeResult, err = writeExternalManagedComposeFile(externalComposeFile, composeYaml, maxInt(s.cfg.Compose.MaxComposeBytes, 1024*1024))
			if err != nil {
				return nil, err
			}
		} else {
			writeResult, err = newComposeWorkspaceManager(s.cfg.Compose).WriteProjectFiles(row.ProjectName, row.WorkingDir.String, composeYaml, request.BuildFiles, true)
			if err != nil {
				return nil, err
			}
		}
	}
	row.ComposeYaml = sql.NullString{String: composeYaml, Valid: true}
	if writeResult != nil {
		row.WorkingDir = sql.NullString{String: writeResult.WorkingDir, Valid: writeResult.WorkingDir != ""}
		row.ConfigFilesJSON = sql.NullString{String: writeResult.ConfigFilesJSON, Valid: writeResult.ConfigFilesJSON != ""}
		row.ComposeFilePath = sql.NullString{String: writeResult.ComposeFilePath, Valid: writeResult.ComposeFilePath != ""}
		row.FileManifestJSON = sql.NullString{String: writeResult.FileManifestJSON, Valid: writeResult.FileManifestJSON != ""}
	}
	row.LastPreviewJSON = sql.NullString{String: previewJSON, Valid: previewJSON != ""}
	row.LastValidationJSON = sql.NullString{String: validationJSON, Valid: validationJSON != ""}
	row.Status = string(ComposeProjectStatusUnknown)
	if err := s.projects.UpdateCompose(ctx, *row); err != nil {
		return nil, err
	}
	return s.GetComposeProject(ctx, row.ProjectID)
}

func writeExternalManagedComposeFile(path string, composeYaml string, maxBytes int) (*composeWriteResult, error) {
	path = strings.TrimSpace(path)
	if path == "" {
		return nil, apperrors.Params("外部 Compose 文件路径为空")
	}
	resolved, err := filepath.EvalSymlinks(path)
	if err != nil {
		return nil, apperrors.Params("外部 Compose 文件路径无效：" + err.Error())
	}
	info, err := os.Stat(resolved)
	if err != nil {
		return nil, apperrors.Params("外部 Compose 文件不存在：" + err.Error())
	}
	if !info.Mode().IsRegular() {
		return nil, apperrors.Params("外部 Compose 文件必须是普通文件")
	}
	data := []byte(strings.TrimSpace(composeYaml) + "\n")
	if maxBytes <= 0 {
		maxBytes = 1024 * 1024
	}
	if len(data) > maxBytes {
		return nil, apperrors.Params("composeYaml 超过大小限制")
	}
	if err := validateDiscoveredComposeYAML(data); err != nil {
		return nil, err
	}
	if err := atomicWriteFile(resolved, data, info.Mode().Perm()); err != nil {
		return nil, err
	}
	manifestBytes, _ := json.Marshal([]composeFileManifestEntry{{
		Path: resolved,
		Kind: "compose",
		Size: len(data),
	}})
	configFilesBytes, _ := json.Marshal([]string{resolved})
	return &composeWriteResult{
		WorkingDir:       filepath.Dir(resolved),
		ComposeFilePath:  resolved,
		ConfigFilesJSON:  string(configFilesBytes),
		FileManifestJSON: string(manifestBytes),
		WrittenFilePaths: []string{resolved},
	}, nil
}

type discoveredComposeFile struct {
	Path    string
	Content string
	Size    int64
	SHA256  string
}

func findDiscoveredComposeProject(items []ComposeProjectSummaryVO, request DockerComposeImportDiscoveredRequest) (ComposeProjectSummaryVO, bool) {
	projectID := strings.TrimSpace(request.ProjectID)
	projectName := strings.TrimSpace(request.ProjectName)
	for _, item := range items {
		if item.Source != ComposeProjectSourceDiscovered {
			continue
		}
		if projectID != "" && item.ProjectID == projectID {
			return item, true
		}
		if projectID == "" && projectName != "" && item.ProjectName == projectName {
			return item, true
		}
	}
	return ComposeProjectSummaryVO{}, false
}

func resolveDiscoveredComposeFile(workingDir string, configFiles []string, maxBytes int) (*discoveredComposeFile, error) {
	if len(configFiles) == 0 {
		return nil, apperrors.Params("Docker labels 未提供 compose 配置文件路径")
	}
	if maxBytes <= 0 {
		maxBytes = 1024 * 1024
	}
	for _, candidate := range configFiles {
		resolved, err := validateDiscoveredComposeFilePath(workingDir, candidate, maxBytes)
		if err == nil {
			return resolved, nil
		}
	}
	return nil, apperrors.Params("未找到有效的 discovered compose 配置文件")
}

func validateDiscoveredComposeFilePath(workingDir, candidate string, maxBytes int) (*discoveredComposeFile, error) {
	candidate = strings.TrimSpace(candidate)
	if candidate == "" {
		return nil, apperrors.Params("compose 配置文件路径为空")
	}
	cleaned := filepath.Clean(candidate)
	if !filepath.IsAbs(cleaned) {
		base := strings.TrimSpace(workingDir)
		if base == "" {
			return nil, apperrors.Params("相对 compose 配置文件缺少 workingDir")
		}
		cleaned = filepath.Join(filepath.Clean(base), cleaned)
	}
	resolved, err := filepath.EvalSymlinks(cleaned)
	if err != nil {
		return nil, apperrors.Params("compose 配置文件路径无效：" + err.Error())
	}
	info, err := os.Stat(resolved)
	if err != nil {
		return nil, apperrors.Params("compose 配置文件不存在：" + err.Error())
	}
	if !info.Mode().IsRegular() {
		return nil, apperrors.Params("compose 配置文件必须是普通文件")
	}
	if info.Size() > int64(maxBytes) {
		return nil, apperrors.Params("compose 配置文件超过大小限制")
	}
	data, err := os.ReadFile(resolved)
	if err != nil {
		return nil, apperrors.Operation("读取 compose 配置文件失败：" + err.Error())
	}
	if len(data) > maxBytes {
		return nil, apperrors.Params("compose 配置文件超过大小限制")
	}
	if err := validateDiscoveredComposeYAML(data); err != nil {
		return nil, err
	}
	sum := sha256.Sum256(data)
	return &discoveredComposeFile{
		Path:    resolved,
		Content: strings.TrimSpace(string(data)),
		Size:    info.Size(),
		SHA256:  hex.EncodeToString(sum[:]),
	}, nil
}

func validateDiscoveredComposeYAML(data []byte) error {
	var root map[string]any
	if err := yaml.Unmarshal(data, &root); err != nil {
		return apperrors.Params("compose 配置文件 YAML 无效：" + err.Error())
	}
	services, ok := root["services"].(map[string]any)
	if !ok || len(services) == 0 {
		return apperrors.Params("compose 配置文件缺少 services")
	}
	for name := range services {
		if strings.TrimSpace(name) == "" {
			return apperrors.Params("compose service 名称不能为空")
		}
	}
	return nil
}

func (s *service) PreviewComposeProject(ctx context.Context, actor OperationActor, projectID string) (*ComposePreviewView, error) {
	row, err := s.requireManagedComposeProject(ctx, projectID)
	if err != nil {
		return nil, err
	}
	preview, err := s.PreviewCompose(ctx, actor, composeRequestFromRecord(*row))
	if err != nil {
		return nil, err
	}
	previewJSON, validationJSON := composeDiagnosticsJSON(preview)
	_ = s.projects.UpdateDiagnostics(ctx, row.ProjectID, previewJSON, validationJSON, row.Status)
	return preview, nil
}

func (s *service) ValidateComposeProject(ctx context.Context, actor OperationActor, projectID string) (*ComposePreviewView, error) {
	return s.PreviewComposeProject(ctx, actor, projectID)
}

func (s *service) ComposeProjectPS(ctx context.Context, projectID string) (*ComposePSView, error) {
	if err := s.validateComposeProjectAction(ctx, projectID, composeActionPS); err != nil {
		return nil, err
	}
	row, err := s.requireManagedComposeProject(ctx, projectID)
	if err != nil {
		return nil, err
	}
	result, err := s.ComposePS(ctx, composeRequestFromRecord(*row))
	if err != nil {
		return nil, err
	}
	result.ProjectID = row.ProjectID
	result.Services = buildComposeServiceViews(row.ComposeYaml.String, result.Containers, nil)
	return result, nil
}

func (s *service) SubmitComposeProjectOperation(ctx context.Context, actor OperationActor, projectID, operationType string, tail int) (*OperationAcceptedVO, error) {
	if err := s.ValidateComposeProjectOperation(ctx, projectID, operationType); err != nil {
		return nil, err
	}
	row, err := s.requireManagedComposeProject(ctx, projectID)
	if err != nil {
		return nil, err
	}
	composeReq := composeRequestFromRecord(*row)
	payload := any(composeReq)
	if operationType == OperationTypeComposeLogs {
		payload = map[string]any{"projectName": composeReq.ProjectName, "composeYaml": composeReq.ComposeYaml, "workingDir": composeReq.WorkingDir, "composeFilePath": composeReq.ComposeFilePath, "tail": tail}
	}
	result, err := s.SubmitOperation(ctx, OperationSubmitCommand{
		OperationType: operationType,
		TargetType:    "compose",
		TargetID:      row.ProjectID,
		TargetName:    row.ProjectName,
		Payload:       payload,
		Actor:         actor,
	})
	if err == nil && result != nil {
		_ = s.projects.UpdateLastOperation(ctx, row.ProjectID, result.OperationID)
	}
	return result, err
}

func (s *service) LatestOperation(ctx context.Context, query LatestOperationQuery) (*LatestOperationView, error) {
	if s.operations == nil {
		return nil, apperrors.Operation("Docker operation runtime 未配置 datasource")
	}
	row, err := s.operations.LatestOperation(ctx, query)
	if err != nil {
		return nil, err
	}
	if row == nil {
		return &LatestOperationView{}, nil
	}
	op := operationVO(*row)
	events, _ := s.ListOperationEvents(ctx, op.OperationID, 0, 100)
	return &LatestOperationView{Operation: &op, Events: events}, nil
}

func (s *service) managedComposeProjects(ctx context.Context) ([]ComposeProjectRecord, error) {
	if s.projects == nil {
		return nil, apperrors.Operation("Docker compose project repository 未配置 datasource")
	}
	return s.projects.List(ctx)
}

func (s *service) requireManagedComposeProject(ctx context.Context, projectID string) (*ComposeProjectRecord, error) {
	if s.projects == nil {
		return nil, apperrors.Operation("Docker compose project repository 未配置 datasource")
	}
	row, err := s.projects.GetByProjectID(ctx, strings.TrimSpace(projectID))
	if err != nil {
		return nil, err
	}
	if row == nil {
		return nil, apperrors.NotFound("Docker Compose 项目不存在或不是托管项目")
	}
	if strings.TrimSpace(row.ComposeYaml.String) == "" {
		return nil, apperrors.Params("当前 Compose 项目没有可执行 YAML")
	}
	return row, nil
}

func (s *service) allContainerViews(ctx context.Context) ([]ContainerView, error) {
	cli, err := s.requireClient()
	if err != nil {
		return nil, err
	}
	runCtx, cancel := s.withTimeout(ctx)
	defer cancel()
	items, err := cli.ContainerList(runCtx, dockercontainer.ListOptions{All: true})
	if err != nil {
		return nil, apperrors.Operation("获取 Docker 容器列表失败：" + err.Error())
	}
	result := make([]ContainerView, 0, len(items))
	for _, item := range items {
		result = append(result, s.toContainerView(item, nil))
	}
	return result, nil
}

func (s *service) composeProjectSummary(row ComposeProjectRecord, containers []ContainerView, latest *OperationRecord) ComposeProjectSummaryVO {
	preview, _ := composePreviewFromJSON(row.LastPreviewJSON.String)
	configFiles := configFilesFromRecord(row.ConfigFilesJSON.String)
	summary := summarizeComposeProject(row.ProjectID, row.ProjectName, ComposeProjectSourceManaged, row.WorkingDir.String, configFiles, containers, preview)
	summary.CreatedAt = timeString(row.CreateTime)
	summary.UpdatedAt = timeString(row.UpdateTime)
	hasSpec := strings.TrimSpace(row.ComposeYaml.String) != "" || strings.TrimSpace(row.ComposeFilePath.String) != ""
	summary.AvailableActions = composeAvailableActions(summary.Source, summary.Status, summary.ContainerCount, hasSpec, false)
	if latest != nil {
		applyLatestOperation(&summary, operationVO(*latest))
	}
	return summary
}

func (s *service) composeProjectDetail(ctx context.Context, row ComposeProjectRecord, containers []ContainerView) ComposeProjectDetailVO {
	preview, validation := composeDiagnosticsFromRecord(row)
	services := buildComposeServiceViews(row.ComposeYaml.String, containers, preview)
	detail := ComposeProjectDetailVO{
		ProjectID:       row.ProjectID,
		ProjectName:     row.ProjectName,
		Source:          ComposeProjectSourceManaged,
		WorkingDir:      row.WorkingDir.String,
		ConfigFiles:     configFilesFromRecord(row.ConfigFilesJSON.String),
		ComposeFilePath: row.ComposeFilePath.String,
		FileManifest:    composeProjectFileManifestFromRecord(row.FileManifestJSON.String),
		ComposeYaml:     row.ComposeYaml.String,
		VisualDraft:     composeVisualDraftFromYaml(row.ComposeYaml.String),
		Services:        services,
		Containers:      containers,
		Preview:         preview,
		Validation:      validation,
		NormalizedYaml:  normalizedYamlFromValidation(validation),
	}
	if row.LastOperationID.Valid {
		if latest, err := s.GetOperation(ctx, row.LastOperationID.Int64); err == nil && latest != nil {
			detail.LastOperation = latest
		}
	}
	if detail.LastOperation == nil {
		detail.LastOperation, _ = s.latestOperationForCompose(ctx, row.ProjectID, row.ProjectName, "")
	}
	if detail.LastOperation != nil {
		detail.ActiveOperation = activeOperationSummary(detail.LastOperation)
		detail.RecentEvents, _ = s.ListOperationEvents(ctx, detail.LastOperation.OperationID, 0, 100)
	}
	detail.AvailableActions = composeAvailableActions(detail.Source, composeStatus(containers), len(containers), strings.TrimSpace(detail.ComposeYaml) != "" || strings.TrimSpace(detail.ComposeFilePath) != "", detail.ActiveOperation != nil)
	return detail
}

func (s *service) latestOperationForCompose(ctx context.Context, projectID, projectName, operationType string) (*OperationVO, error) {
	if s.operations == nil {
		return nil, nil
	}
	query := LatestOperationQuery{TargetType: "compose", TargetID: strings.TrimSpace(projectID), OperationType: operationType}
	if query.TargetID == "" {
		query.TargetName = strings.TrimSpace(projectName)
	}
	row, err := s.operations.LatestOperation(ctx, query)
	if err != nil || row == nil {
		return nil, err
	}
	vo := operationVO(*row)
	return &vo, nil
}

func discoverComposeProjects(containers []ContainerView) []ComposeProjectSummaryVO {
	grouped := map[string][]ContainerView{}
	for _, item := range containers {
		if !item.ComposeManaged || strings.TrimSpace(item.ComposeProject) == "" {
			continue
		}
		key := composeRuntimeKey(item.ComposeProject, item.ComposeWorkingDir, item.ComposeConfigFiles)
		grouped[key] = append(grouped[key], item)
	}
	result := make([]ComposeProjectSummaryVO, 0, len(grouped))
	for key, projectContainers := range grouped {
		if len(projectContainers) == 0 {
			continue
		}
		first := projectContainers[0]
		projectID := "runtime_" + shortHash(key)
		result = append(result, summarizeComposeProject(projectID, first.ComposeProject, ComposeProjectSourceDiscovered, first.ComposeWorkingDir, splitConfigFiles(first.ComposeConfigFiles), projectContainers, nil))
	}
	sort.SliceStable(result, func(i, j int) bool { return result[i].ProjectName < result[j].ProjectName })
	return result
}

func summarizeComposeProject(projectID, projectName string, source ComposeProjectSource, workingDir string, configFiles []string, containers []ContainerView, preview *PreviewVO) ComposeProjectSummaryVO {
	running, exited := composeContainerCounts(containers)
	status := composeStatus(containers)
	serviceCount := len(buildComposeServiceViews("", containers, preview))
	summary := ComposeProjectSummaryVO{
		ProjectID:      projectID,
		ProjectName:    projectName,
		Source:         source,
		WorkingDir:     workingDir,
		ConfigFiles:    configFiles,
		ServiceCount:   serviceCount,
		ContainerCount: len(containers),
		RunningCount:   running,
		ExitedCount:    exited,
		Status:         status,
	}
	summary.AvailableActions = composeAvailableActions(source, status, len(containers), source == ComposeProjectSourceManaged, false)
	if preview != nil {
		safe := preview.Safe
		summary.Safe = &safe
		summary.WarningCount = len(preview.Warnings)
		summary.ViolationCount = len(preview.Violations)
	}
	return summary
}

func buildComposeServiceViews(composeYaml string, containers []ContainerView, preview *PreviewVO) []ComposeServiceVO {
	serviceMap := map[string]*ComposeServiceVO{}
	for _, container := range containers {
		name := firstNonBlank(container.ComposeService, container.Name, stripSHA(container.ID))
		service := serviceMap[name]
		if service == nil {
			service = &ComposeServiceVO{ServiceName: name}
			serviceMap[name] = service
		}
		service.ContainerCount++
		if strings.EqualFold(container.State, "running") {
			service.RunningCount++
		}
		if strings.EqualFold(container.State, "exited") {
			service.ExitedCount++
		}
		if service.Image == "" {
			service.Image = container.Image
		}
		service.Ports = append(service.Ports, container.Ports...)
		service.Containers = append(service.Containers, container)
	}
	if strings.TrimSpace(composeYaml) != "" {
		if spec, err := parseComposePolicySpec(composeYaml); err == nil && spec != nil {
			for _, item := range spec.Services {
				if strings.TrimSpace(item.Name) == "" {
					continue
				}
				service := serviceMap[item.Name]
				if service == nil {
					service = &ComposeServiceVO{ServiceName: item.Name}
					serviceMap[item.Name] = service
				}
				if service.Image == "" {
					service.Image = item.Image
				}
			}
		}
	}
	if preview != nil {
		for _, item := range append(append([]PolicyViolationVO{}, preview.Violations...), preview.Warnings...) {
			serviceName := serviceNameFromPolicyField(item.Field)
			if serviceName == "" {
				continue
			}
			service := serviceMap[serviceName]
			if service == nil {
				service = &ComposeServiceVO{ServiceName: serviceName}
				serviceMap[serviceName] = service
			}
			if item.Action == PolicyActionDeny {
				service.ViolationCount++
			} else {
				service.WarningCount++
			}
		}
	}
	result := make([]ComposeServiceVO, 0, len(serviceMap))
	for _, service := range serviceMap {
		service.Status = composeStatus(service.Containers)
		result = append(result, *service)
	}
	sort.SliceStable(result, func(i, j int) bool { return result[i].ServiceName < result[j].ServiceName })
	return result
}

func matchComposeContainers(projectName string, containers []ContainerView) []ContainerView {
	result := make([]ContainerView, 0)
	for _, container := range containers {
		if container.ComposeManaged && container.ComposeProject == projectName {
			result = append(result, container)
		}
	}
	sort.SliceStable(result, func(i, j int) bool { return result[i].Created > result[j].Created })
	return result
}

func composeStatus(containers []ContainerView) ComposeProjectStatus {
	if len(containers) == 0 {
		return ComposeProjectStatusUnknown
	}
	running := 0
	for _, item := range containers {
		if strings.EqualFold(item.State, "running") {
			running++
		}
	}
	if running == len(containers) {
		return ComposeProjectStatusRunning
	}
	if running > 0 {
		return ComposeProjectStatusDegraded
	}
	return ComposeProjectStatusStopped
}

func composeContainerCounts(containers []ContainerView) (running, exited int) {
	for _, item := range containers {
		if strings.EqualFold(item.State, "running") {
			running++
		}
		if strings.EqualFold(item.State, "exited") {
			exited++
		}
	}
	return running, exited
}

func matchComposeProject(item ComposeProjectSummaryVO, keyword, status string) bool {
	if strings.TrimSpace(status) != "" && !strings.EqualFold(string(item.Status), strings.TrimSpace(status)) {
		return false
	}
	needle := strings.ToLower(strings.TrimSpace(keyword))
	if needle == "" {
		return true
	}
	values := []string{item.ProjectID, item.ProjectName, item.WorkingDir, strings.Join(item.ConfigFiles, " ")}
	for _, value := range values {
		if strings.Contains(strings.ToLower(value), needle) {
			return true
		}
	}
	return false
}

func composeDiagnosticsFromRecord(row ComposeProjectRecord) (*PreviewVO, *ComposeValidationView) {
	preview, _ := composePreviewFromJSON(row.LastPreviewJSON.String)
	validation := composeValidationFromJSON(row.LastValidationJSON.String)
	return preview, validation
}

func composeVisualDraftFromYaml(raw string) *ComposeVisualDraftView {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil
	}
	parsed, err := parseComposeYamlForBuilder(raw)
	if err != nil || parsed == nil {
		return nil
	}
	return parsed.VisualDraft
}

func composePreviewFromJSON(raw string) (*PreviewVO, bool) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil, false
	}
	var full ComposePreviewView
	if err := json.Unmarshal([]byte(raw), &full); err == nil {
		return &full.Preview, true
	}
	var preview PreviewVO
	if err := json.Unmarshal([]byte(raw), &preview); err == nil {
		return &preview, true
	}
	return nil, false
}

func composeValidationFromJSON(raw string) *ComposeValidationView {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil
	}
	var validation ComposeValidationView
	if err := json.Unmarshal([]byte(raw), &validation); err != nil {
		return nil
	}
	return &validation
}

func composeDiagnosticsJSON(preview *ComposePreviewView) (string, string) {
	if preview == nil {
		return "", ""
	}
	previewBytes, _ := json.Marshal(preview)
	validationBytes, _ := json.Marshal(preview.Validation)
	return string(previewBytes), string(validationBytes)
}

func composeProjectFileManifestFromRecord(raw string) []ComposeProjectFileManifestView {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil
	}
	var entries []struct {
		Path        string `json:"path"`
		ServiceName string `json:"serviceName,omitempty"`
		Kind        string `json:"kind"`
		Size        int    `json:"size"`
	}
	if err := json.Unmarshal([]byte(raw), &entries); err != nil {
		return nil
	}
	result := make([]ComposeProjectFileManifestView, 0, len(entries))
	for _, item := range entries {
		result = append(result, ComposeProjectFileManifestView{
			Path:        item.Path,
			Type:        item.Kind,
			ServiceName: item.ServiceName,
			SizeBytes:   item.Size,
		})
	}
	return result
}

func normalizedYamlFromValidation(validation *ComposeValidationView) string {
	if validation == nil {
		return ""
	}
	return validation.NormalizedYaml
}

func configFilesFromRecord(raw string) []string {
	values := splitConfigFiles(raw)
	if len(values) > 0 {
		return values
	}
	var result []string
	if err := json.Unmarshal([]byte(strings.TrimSpace(raw)), &result); err == nil {
		return safeStrings(result)
	}
	return nil
}

func splitConfigFiles(raw string) []string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil
	}
	if strings.HasPrefix(raw, "[") {
		var result []string
		if err := json.Unmarshal([]byte(raw), &result); err == nil {
			return safeStrings(result)
		}
	}
	parts := strings.FieldsFunc(raw, func(r rune) bool { return r == ',' || r == '\n' || r == ';' })
	result := make([]string, 0, len(parts))
	for _, part := range parts {
		if value := strings.TrimSpace(part); value != "" {
			result = append(result, value)
		}
	}
	return result
}

func composeRuntimeKey(projectName, workingDir, configFiles string) string {
	return strings.Join([]string{strings.TrimSpace(projectName), strings.TrimSpace(workingDir), strings.TrimSpace(configFiles)}, "|")
}

func shortHash(value string) string {
	sum := sha256.Sum256([]byte(value))
	return hex.EncodeToString(sum[:])[:12]
}

func findManagedByProjectID(records []ComposeProjectRecord, projectID string) (ComposeProjectRecord, bool) {
	for _, row := range records {
		if row.ProjectID == projectID {
			return row, true
		}
	}
	return ComposeProjectRecord{}, false
}

func findManagedByProjectName(records []ComposeProjectRecord, projectName string) (ComposeProjectRecord, bool) {
	for _, row := range records {
		if row.ProjectName == projectName {
			return row, true
		}
	}
	return ComposeProjectRecord{}, false
}

func applyLatestOperation(summary *ComposeProjectSummaryVO, op OperationVO) {
	if summary == nil {
		return
	}
	summary.LastOperationID = op.OperationID
	summary.LastOperationType = op.OperationType
	summary.LastOperationStatus = op.Status
	summary.LastOperationProgress = op.Progress
	summary.LastOperationStage = op.CurrentStage
	summary.ActiveOperation = activeOperationSummary(&op)
	summary.AvailableActions = composeAvailableActions(summary.Source, summary.Status, summary.ContainerCount, summary.Source == ComposeProjectSourceManaged, summary.ActiveOperation != nil)
}

func serviceNameFromPolicyField(field string) string {
	parts := strings.Split(strings.TrimSpace(field), ".")
	for i, part := range parts {
		if part == "services" && i+1 < len(parts) {
			return parts[i+1]
		}
	}
	return ""
}

func timeString(value sql.NullTime) string {
	if !value.Valid || value.Time.IsZero() {
		return ""
	}
	return value.Time.Format(time.RFC3339)
}

func strconvBase36(value int64) string {
	const alphabet = "0123456789abcdefghijklmnopqrstuvwxyz"
	if value <= 0 {
		return "0"
	}
	var out []byte
	for value > 0 {
		out = append([]byte{alphabet[value%36]}, out...)
		value /= 36
	}
	return string(out)
}

func composeRequestFromRecord(row ComposeProjectRecord) ComposeUpRequest {
	return ComposeUpRequest{
		ProjectName:     row.ProjectName,
		ComposeYaml:     row.ComposeYaml.String,
		WorkingDir:      row.WorkingDir.String,
		ComposeFilePath: row.ComposeFilePath.String,
	}
}

func resultWorkingDir(result *composeWriteResult) string {
	if result == nil {
		return ""
	}
	return result.WorkingDir
}

func resultConfigFilesJSON(result *composeWriteResult) string {
	if result == nil {
		return ""
	}
	return result.ConfigFilesJSON
}

func resultComposeFilePath(result *composeWriteResult) string {
	if result == nil {
		return ""
	}
	return result.ComposeFilePath
}

func resultFileManifestJSON(result *composeWriteResult) string {
	if result == nil {
		return ""
	}
	return result.FileManifestJSON
}
