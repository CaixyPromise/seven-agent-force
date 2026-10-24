package docker

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	apperrors "github.com/CaixyPromise/seven-agent/force/internal/common/errors"
	"github.com/CaixyPromise/seven-agent/force/internal/shared/config"
)

type composeRunner struct {
	cfg      config.DockerComposeConfig
	security config.DockerSecurityConfig
}

func newComposeRunner(cfg config.DockerComposeConfig, security config.DockerSecurityConfig) *composeRunner {
	return &composeRunner{cfg: cfg, security: security}
}

type preparedCompose struct {
	projectName string
	workDir     string
	composeFile string
	cleanupDir  string
}

func (r *composeRunner) prepare(command ComposeUpRequest) (*preparedCompose, error) {
	project := strings.TrimSpace(command.ProjectName)
	if project == "" {
		project = fmt.Sprintf("docker-ui-%d", time.Now().UnixNano())
	}
	if strings.TrimSpace(command.ComposeFilePath) != "" {
		file := filepath.Clean(strings.TrimSpace(command.ComposeFilePath))
		workDir := strings.TrimSpace(command.WorkingDir)
		if workDir == "" {
			workDir = filepath.Dir(file)
		}
		return &preparedCompose{projectName: project, workDir: workDir, composeFile: file}, nil
	}
	if strings.TrimSpace(command.ComposeYaml) == "" {
		return nil, apperrors.Params("compose 配置不能为空")
	}
	baseDir := strings.TrimSpace(r.cfg.TempDir)
	if baseDir == "" {
		baseDir = os.TempDir()
	}
	dir, err := os.MkdirTemp(baseDir, "docker-compose-ui-")
	if err != nil {
		return nil, apperrors.System("创建 compose 临时目录失败：" + err.Error())
	}
	file := filepath.Join(dir, "docker-compose.yaml")
	if err := os.WriteFile(file, []byte(strings.TrimSpace(command.ComposeYaml)), 0o600); err != nil {
		_ = os.RemoveAll(dir)
		return nil, apperrors.System("创建 compose 临时文件失败：" + err.Error())
	}
	return &preparedCompose{projectName: project, workDir: dir, composeFile: file, cleanupDir: dir}, nil
}

func (r *composeRunner) Validate(ctx context.Context, command ComposeUpRequest) (*ComposeValidationView, error) {
	prepared, err := r.prepare(command)
	if err != nil {
		return nil, err
	}
	defer prepared.cleanup()
	return r.run(ctx, prepared, "config")
}

func (r *composeRunner) Up(ctx context.Context, command ComposeUpRequest) (bool, error) {
	prepared, err := r.prepare(command)
	if err != nil {
		return false, err
	}
	defer prepared.cleanup()
	validation, err := r.run(ctx, prepared, "config")
	if err != nil {
		return false, err
	}
	if !validation.Valid {
		return false, apperrors.Params(validation.Message)
	}
	result, err := r.run(ctx, prepared, "up", "-d")
	if err != nil {
		return false, err
	}
	if !result.Valid {
		return false, apperrors.Operation(result.Message)
	}
	return true, nil
}

func (r *composeRunner) Down(ctx context.Context, command ComposeUpRequest) (bool, error) {
	prepared, err := r.prepare(command)
	if err != nil {
		return false, err
	}
	defer prepared.cleanup()
	result, err := r.run(ctx, prepared, "down")
	if err != nil {
		return false, err
	}
	if !result.Valid {
		return false, apperrors.Operation(result.Message)
	}
	return true, nil
}

func (r *composeRunner) Restart(ctx context.Context, command ComposeUpRequest) (bool, error) {
	prepared, err := r.prepare(command)
	if err != nil {
		return false, err
	}
	defer prepared.cleanup()
	result, err := r.run(ctx, prepared, "restart")
	if err != nil {
		return false, err
	}
	if !result.Valid {
		return false, apperrors.Operation(result.Message)
	}
	return true, nil
}

func (r *composeRunner) PS(ctx context.Context, command ComposeUpRequest) (*ComposePSView, error) {
	prepared, err := r.prepare(command)
	if err != nil {
		return nil, err
	}
	defer prepared.cleanup()
	result, err := r.run(ctx, prepared, "ps", "--format", "json")
	if err != nil {
		return nil, err
	}
	if !result.Valid {
		return nil, apperrors.Operation(result.Message)
	}
	return &ComposePSView{ProjectName: prepared.projectName, Containers: parseComposePSOutput(result.Message)}, nil
}

func (r *composeRunner) Logs(ctx context.Context, command ComposeUpRequest, tail int) (string, error) {
	prepared, err := r.prepare(command)
	if err != nil {
		return "", err
	}
	defer prepared.cleanup()
	if tail <= 0 {
		tail = 200
	}
	if tail > 5000 {
		tail = 5000
	}
	result, err := r.run(ctx, prepared, "logs", "--tail", fmt.Sprintf("%d", tail))
	if err != nil {
		return "", err
	}
	if !result.Valid {
		return "", apperrors.Operation(result.Message)
	}
	return result.Message, nil
}

func (r *composeRunner) run(ctx context.Context, prepared *preparedCompose, args ...string) (*ComposeValidationView, error) {
	timeout := r.cfg.Timeout
	if timeout <= 0 {
		timeout = time.Minute
	}
	runCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	binary := strings.TrimSpace(r.cfg.Binary)
	if binary == "" {
		binary = "docker"
	}
	cmdArgs := []string{"compose", "-f", prepared.composeFile, "-p", prepared.projectName}
	cmdArgs = append(cmdArgs, args...)
	cmd := exec.CommandContext(runCtx, binary, cmdArgs...)
	cmd.Dir = prepared.workDir
	output := newBoundedBuffer(r.cfg.OutputMax)
	cmd.Stdout = output
	cmd.Stderr = output
	err := cmd.Run()
	text := sanitizeOutputWithConfig(output.String(), r.cfg.OutputMax, r.security)
	if runCtx.Err() == context.DeadlineExceeded {
		return &ComposeValidationView{Valid: false, Message: "执行 docker compose 超时"}, nil
	}
	if err != nil {
		if text == "" {
			text = err.Error()
		}
		return &ComposeValidationView{Valid: false, Message: text}, nil
	}
	result := &ComposeValidationView{Valid: true, Message: firstNonBlank(text, "执行成功")}
	if len(args) > 0 && args[len(args)-1] == "config" {
		result.NormalizedYaml = text
	}
	return result, nil
}

func (p *preparedCompose) cleanup() {
	if p == nil || strings.TrimSpace(p.cleanupDir) == "" {
		return
	}
	_ = os.RemoveAll(p.cleanupDir)
}

type boundedBuffer struct {
	buf       strings.Builder
	limit     int
	truncated bool
}

func newBoundedBuffer(limit int) *boundedBuffer {
	if limit <= 0 {
		limit = 1024 * 1024
	}
	return &boundedBuffer{limit: limit}
}

func (b *boundedBuffer) Write(p []byte) (int, error) {
	if b == nil {
		return len(p), nil
	}
	remaining := b.limit - b.buf.Len()
	if remaining > 0 {
		if len(p) <= remaining {
			_, _ = b.buf.Write(p)
		} else {
			_, _ = b.buf.Write(p[:remaining])
			b.truncated = true
		}
	} else {
		b.truncated = true
	}
	return len(p), nil
}

func (b *boundedBuffer) String() string {
	if b == nil {
		return ""
	}
	value := b.buf.String()
	if b.truncated {
		value += "...(truncated)"
	}
	return value
}

func sanitizeOutput(value string, max int) string {
	return sanitizeOutputWithConfig(value, max, config.DockerSecurityConfig{})
}

func sanitizeOutputWithConfig(value string, max int, security config.DockerSecurityConfig) string {
	value = strings.TrimSpace(value)
	for _, marker := range sensitiveDockerMarkers(security) {
		value = maskMarker(value, marker)
	}
	if max > 0 && len(value) > max {
		value = value[:max] + "...(truncated)"
	}
	return value
}

func parseComposePSOutput(value string) []ContainerView {
	value = strings.TrimSpace(value)
	if value == "" {
		return []ContainerView{}
	}
	var arrayPayload []map[string]any
	if err := json.Unmarshal([]byte(value), &arrayPayload); err == nil {
		return composePSPayloadToContainers(arrayPayload)
	}
	lines := strings.Split(value, "\n")
	payload := make([]map[string]any, 0, len(lines))
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" || !strings.HasPrefix(line, "{") {
			continue
		}
		var item map[string]any
		if err := json.Unmarshal([]byte(line), &item); err == nil {
			payload = append(payload, item)
		}
	}
	return composePSPayloadToContainers(payload)
}

func composePSPayloadToContainers(payload []map[string]any) []ContainerView {
	result := make([]ContainerView, 0, len(payload))
	for _, item := range payload {
		id := firstNonBlank(stringValue(item["ID"]), stringValue(item["Id"]), stringValue(item["ContainerID"]))
		name := firstNonBlank(stringValue(item["Name"]), stringValue(item["Service"]))
		result = append(result, ContainerView{
			ID:             stripSHA(id),
			Name:           name,
			Image:          stringValue(item["Image"]),
			State:          firstNonBlank(stringValue(item["State"]), stringValue(item["Status"])),
			Status:         stringValue(item["Status"]),
			ComposeManaged: true,
			ComposeProject: stringValue(item["Project"]),
			ComposeService: stringValue(item["Service"]),
		})
	}
	return result
}

func maskMarker(value, marker string) string {
	lines := strings.Split(value, "\n")
	for i, line := range lines {
		if strings.Contains(strings.ToLower(line), marker) {
			if idx := strings.Index(line, ":"); idx >= 0 {
				lines[i] = line[:idx+1] + " ******"
			} else if idx := strings.Index(line, "="); idx >= 0 {
				lines[i] = line[:idx+1] + "******"
			} else {
				lines[i] = marker + "=******"
			}
		}
	}
	return strings.Join(lines, "\n")
}
