package docker

import (
	"encoding/json"
	"fmt"
	"strings"

	jsoncompat "github.com/CaixyPromise/seven-agent/force/internal/common/json"
	"github.com/CaixyPromise/seven-agent/force/internal/shared/config"
)

const (
	permissionDockerDangerous      = "admin:docker:dangerous"
	permissionDockerPolicyOverride = "admin:docker:policy:override"
)

type policyDecision struct {
	Warnings   []PolicyViolationVO
	Violations []PolicyViolationVO
}

func (d policyDecision) safe() bool { return len(d.Violations) == 0 }

func evaluatePolicy(cfg config.DockerSecurityConfig, actor OperationActor, operationType string, payload any) policyDecision {
	return evaluatePolicyWithMode(cfg, actor, operationType, payload, policyEnforcedOperation(operationType))
}

func evaluatePolicyWithMode(cfg config.DockerSecurityConfig, actor OperationActor, operationType string, payload any, enforce bool) policyDecision {
	profile := strings.ToLower(strings.TrimSpace(cfg.PolicyProfile))
	if profile == "" {
		profile = "compatible"
	}
	findings := detectPolicyFindings(cfg, operationType, payload)
	decision := policyDecision{}
	for _, finding := range findings {
		action := PolicyActionWarn
		switch profile {
		case "safe":
			if finding.Severity == "HIGH" || finding.Severity == "CRITICAL" {
				action = PolicyActionDeny
			}
		case "locked-down":
			if finding.Severity == "MEDIUM" || finding.Severity == "HIGH" || finding.Severity == "CRITICAL" {
				action = PolicyActionDeny
			}
		default:
			action = PolicyActionWarn
		}
		if !enforce {
			action = PolicyActionWarn
		}
		if action == PolicyActionDeny && hasDockerPermission(actor, permissionDockerDangerous) && profile == "safe" {
			action = PolicyActionWarn
			finding.Remediation = "已通过 admin:docker:dangerous 运维权限放行，仍需审计"
		}
		if action == PolicyActionDeny && hasDockerPermission(actor, permissionDockerPolicyOverride) {
			action = PolicyActionWarn
			finding.Remediation = "已通过 admin:docker:policy:override 策略覆盖权限放行"
		}
		finding.Action = action
		if action == PolicyActionDeny {
			decision.Violations = append(decision.Violations, finding)
		} else {
			decision.Warnings = append(decision.Warnings, finding)
		}
	}
	return decision
}

func policyEnforcedOperation(operationType string) bool {
	switch operationType {
	case OperationTypeComposeValidate:
		return false
	default:
		return true
	}
}

func detectPolicyFindings(cfg config.DockerSecurityConfig, operationType string, payload any) []PolicyViolationVO {
	switch typed := payload.(type) {
	case ContainerCreateRequest:
		return inspectContainerCreatePolicy(cfg, typed)
	case ComposeUpRequest:
		return inspectComposePolicy(cfg, typed)
	case ImagePullCommand:
		return inspectImagePolicy(cfg, typed.Repository, typed.Tag, typed.RegistryID)
	case RemoteImagePullRequest:
		return inspectImagePolicy(cfg, typed.Repository, typed.Tag, typed.RegistryID)
	case ImagePushCommand:
		target := firstNonBlank(typed.TargetRepository, typed.SourceImage)
		return inspectImagePolicy(cfg, target, typed.TargetTag, typed.RegistryID)
	case map[string]any:
		return inspectMapPolicy(cfg, operationType, typed)
	default:
		return nil
	}
}

func inspectContainerCreatePolicy(cfg config.DockerSecurityConfig, command ContainerCreateRequest) []PolicyViolationVO {
	var result []PolicyViolationVO
	networkMode := strings.ToLower(strings.TrimSpace(command.NetworkMode))
	if command.Privileged {
		result = append(result, violation("DOCKER_PRIVILEGED", "CRITICAL", "privileged", "true", "privileged 容器可突破隔离边界"))
	}
	if networkMode == "host" {
		result = append(result, violation("DOCKER_HOST_NETWORK", "HIGH", "networkMode", command.NetworkMode, "host network 会绕过端口隔离"))
	}
	if networkMode != "" && !isAllowed(cfg.AllowedNetworks, networkMode) && strings.EqualFold(cfg.PolicyProfile, "locked-down") {
		result = append(result, violation("DOCKER_UNTRUSTED_NETWORK", "MEDIUM", "networkMode", command.NetworkMode, "locked-down 模式只允许白名单网络"))
	}
	for _, item := range command.VolumeBindings {
		source := strings.TrimSpace(item.Source)
		if source == "" {
			continue
		}
		if strings.Contains(source, "docker.sock") {
			result = append(result, violation("DOCKER_SOCKET_MOUNT", "CRITICAL", "volume.source", source, "挂载 docker.sock 等同宿主 Docker root 权限"))
		}
		clean := cleanHostPath(source)
		for _, dangerous := range []string{"/", "/etc", "/var/run", "/root"} {
			if clean == dangerous || strings.HasPrefix(clean, dangerous+"/") {
				result = append(result, violation("DOCKER_DANGEROUS_BIND", "HIGH", "volume.source", source, "高危宿主路径 bind mount"))
				break
			}
		}
		if strings.EqualFold(item.Type, "bind") && !isAllowed(cfg.AllowedVolumes, source) && strings.EqualFold(cfg.PolicyProfile, "locked-down") {
			result = append(result, violation("DOCKER_UNTRUSTED_VOLUME", "MEDIUM", "volume.source", source, "locked-down 模式只允许白名单 volume/bind"))
		}
	}
	result = append(result, inspectKeyValuePolicy(cfg, "env", command.Environment)...)
	result = append(result, inspectKeyValuePolicy(cfg, "label", command.Labels)...)
	result = append(result, inspectImageReferencePolicy(cfg, command.ImageReference)...)
	return result
}

func inspectComposePolicy(cfg config.DockerSecurityConfig, command ComposeUpRequest) []PolicyViolationVO {
	spec, err := parseComposePolicySpec(command.ComposeYaml)
	if err != nil {
		return []PolicyViolationVO{violation("COMPOSE_PARSE_FAILED", "MEDIUM", "composeYaml", "parse failed", err.Error())}
	}
	var result []PolicyViolationVO
	for _, service := range spec.Services {
		prefix := "services." + service.Name
		if service.Privileged {
			result = append(result, violation("COMPOSE_PRIVILEGED", "CRITICAL", prefix+".privileged", "true", "Compose 服务启用 privileged"))
		}
		if strings.EqualFold(strings.TrimSpace(service.NetworkMode), "host") {
			result = append(result, violation("COMPOSE_HOST_NETWORK", "HIGH", prefix+".network_mode", service.NetworkMode, "Compose 服务使用 host network"))
		}
		if strings.EqualFold(strings.TrimSpace(service.PIDMode), "host") {
			result = append(result, violation("COMPOSE_HOST_NAMESPACE", "HIGH", prefix+".pid", service.PIDMode, "Compose 服务使用 host pid namespace"))
		}
		if strings.EqualFold(strings.TrimSpace(service.IPCMode), "host") {
			result = append(result, violation("COMPOSE_HOST_NAMESPACE", "HIGH", prefix+".ipc", service.IPCMode, "Compose 服务使用 host ipc namespace"))
		}
		for _, network := range service.Networks {
			if strings.EqualFold(cfg.PolicyProfile, "locked-down") && !isAllowed(cfg.AllowedNetworks, network) {
				result = append(result, violation("COMPOSE_UNTRUSTED_NETWORK", "MEDIUM", prefix+".networks", network, "locked-down 模式只允许白名单 network"))
			}
		}
		for _, volume := range service.Volumes {
			source := strings.TrimSpace(volume.Source)
			if source == "" {
				continue
			}
			if strings.Contains(source, "docker.sock") {
				result = append(result, violation("COMPOSE_DOCKER_SOCKET", "CRITICAL", prefix+".volumes", source, "Compose 服务挂载 docker.sock"))
			}
			clean := cleanHostPath(source)
			for _, dangerous := range []string{"/", "/etc", "/var/run", "/root"} {
				if clean == dangerous || strings.HasPrefix(clean, dangerous+"/") {
					result = append(result, violation("COMPOSE_DANGEROUS_BIND", "HIGH", prefix+".volumes", source, "Compose 存在高危宿主路径挂载"))
					break
				}
			}
			if strings.EqualFold(cfg.PolicyProfile, "locked-down") && !isAllowed(cfg.AllowedVolumes, source) {
				result = append(result, violation("COMPOSE_UNTRUSTED_VOLUME", "MEDIUM", prefix+".volumes", source, "locked-down 模式只允许白名单 volume/bind"))
			}
		}
		for _, finding := range inspectKeyValuePolicy(cfg, prefix+".environment", service.Environment) {
			result = append(result, finding)
		}
		for _, finding := range inspectKeyValuePolicy(cfg, prefix+".labels", service.Labels) {
			result = append(result, finding)
		}
		for _, finding := range inspectImageReferencePolicy(cfg, service.Image) {
			finding.Field = prefix + ".image"
			result = append(result, finding)
		}
	}
	return result
}

func inspectImagePolicy(cfg config.DockerSecurityConfig, repository, tag string, registryID int64) []PolicyViolationVO {
	var result []PolicyViolationVO
	result = append(result, inspectImageReferencePolicy(cfg, repository)...)
	if strings.TrimSpace(tag) == "" || strings.EqualFold(strings.TrimSpace(tag), "latest") {
		result = append(result, violation("DOCKER_TAG_LATEST", "MEDIUM", "tag", firstNonBlank(tag, "latest"), "latest tag 不可复现，生产链路应使用 digest"))
	}
	if registryID <= 0 && !isKnownRegistry(cfg.TrustedRegistries, repository) {
		result = append(result, violation("DOCKER_UNKNOWN_REGISTRY", "MEDIUM", "repository", repository, "镜像来源不在可信 registry 列表"))
	}
	if !strings.Contains(repository, "@sha256:") && !isAllowed(cfg.TrustedImages, repository) {
		result = append(result, violation("DOCKER_UNSIGNED_IMAGE", "MEDIUM", "repository", repository, "镜像未解析到可信 digest 或白名单"))
	}
	return result
}

func inspectImageReferencePolicy(cfg config.DockerSecurityConfig, image string) []PolicyViolationVO {
	image = strings.TrimSpace(image)
	if image == "" {
		return nil
	}
	var result []PolicyViolationVO
	if strings.HasSuffix(image, ":latest") || !strings.Contains(image, ":") && !strings.Contains(image, "@sha256:") {
		result = append(result, violation("DOCKER_TAG_LATEST", "MEDIUM", "image", image, "latest tag 不可复现，生产链路应使用 digest"))
	}
	if !isKnownRegistry(cfg.TrustedRegistries, image) {
		result = append(result, violation("DOCKER_UNKNOWN_REGISTRY", "MEDIUM", "image", image, "镜像来源不在可信 registry 列表"))
	}
	if !strings.Contains(image, "@sha256:") && !isAllowed(cfg.TrustedImages, image) {
		result = append(result, violation("DOCKER_UNSIGNED_IMAGE", "MEDIUM", "image", image, "镜像未解析到可信 digest 或白名单"))
	}
	return result
}

func inspectKeyValuePolicy(cfg config.DockerSecurityConfig, field string, values []KeyValueCommand) []PolicyViolationVO {
	var result []PolicyViolationVO
	for _, item := range values {
		key := strings.TrimSpace(item.Key)
		if key == "" {
			continue
		}
		if isSensitiveDockerKey(cfg, key) {
			codeSuffix := strings.ToUpper(strings.NewReplacer(".", "_", "-", "_").Replace(field))
			result = append(result, violation("DOCKER_SENSITIVE_"+codeSuffix, "MEDIUM", field+"."+key, "******", "运行参数包含敏感字段，输出必须脱敏"))
		}
	}
	return result
}

func inspectMapPolicy(cfg config.DockerSecurityConfig, operationType string, payload map[string]any) []PolicyViolationVO {
	if raw, ok := payload["composeYaml"].(string); ok && strings.TrimSpace(raw) != "" {
		return inspectComposePolicy(cfg, ComposeUpRequest{ProjectName: stringValue(payload["projectName"]), ComposeYaml: raw})
	}
	encoded, _ := json.Marshal(payload)
	if strings.Contains(strings.ToLower(string(encoded)), "privileged") {
		return []PolicyViolationVO{violation("DOCKER_RISKY_PAYLOAD", "MEDIUM", operationType, "privileged", "操作载荷存在疑似高危字段")}
	}
	return nil
}

func violation(code, severity, field, value, message string) PolicyViolationVO {
	return PolicyViolationVO{
		Code:        code,
		Severity:    severity,
		Action:      PolicyActionWarn,
		Message:     message,
		Field:       field,
		Value:       truncate(value, 240),
		Remediation: "请使用固定 digest、白名单资源或调整 Docker 安全策略",
	}
}

func hasDockerPermission(actor OperationActor, permission string) bool {
	for _, item := range actor.Permissions {
		if strings.TrimSpace(item) == permission || strings.TrimSpace(item) == "*" {
			return true
		}
	}
	return false
}

func isAllowed(list []string, value string) bool {
	value = strings.TrimSpace(value)
	if value == "" || len(list) == 0 {
		return false
	}
	for _, item := range list {
		if strings.EqualFold(strings.TrimSpace(item), value) || strings.HasSuffix(value, strings.TrimSpace(item)) {
			return true
		}
	}
	return false
}

func isKnownRegistry(list []string, image string) bool {
	image = strings.TrimSpace(image)
	if image == "" {
		return false
	}
	if len(list) == 0 {
		return false
	}
	first := image
	if idx := strings.Index(first, "/"); idx >= 0 {
		first = first[:idx]
	}
	if !strings.Contains(first, ".") && !strings.Contains(first, ":") && first != "localhost" {
		first = "docker.io"
	}
	return isAllowed(list, first)
}

func isSensitiveDockerKey(cfg config.DockerSecurityConfig, key string) bool {
	key = strings.ToLower(strings.TrimSpace(key))
	if key == "" {
		return false
	}
	markers := append([]string{"password", "passwd", "pwd", "token", "secret", "credential", "authorization", "private_key", "configvalue", "config_value", "issensitive", "is_sensitive"}, cfg.SensitiveKeys...)
	for _, marker := range markers {
		if strings.Contains(key, strings.ToLower(strings.TrimSpace(marker))) {
			return true
		}
	}
	return false
}

func sensitiveDockerMarkers(cfg config.DockerSecurityConfig) []string {
	defaults := []string{
		"password",
		"passwd",
		"token",
		"secret",
		"credential",
		"authorization",
		"private_key",
		"privatekey",
		"apikey",
		"api_key",
		"access_key",
		"accesskey",
		"configvalue",
		"config_value",
		"issensitive",
		"is_sensitive",
	}
	result := append([]string{}, defaults...)
	result = append(result, cfg.SensitiveKeys...)
	return result
}

func sanitizeTextBySensitiveKeys(value string, cfg config.DockerSecurityConfig) string {
	value = strings.TrimSpace(value)
	markers := sensitiveDockerMarkers(cfg)
	value = jsoncompat.MaskSensitiveText(value, markers, 0)
	for _, marker := range markers {
		marker = strings.TrimSpace(marker)
		if marker != "" && !isStructuredSensitiveMarker(marker) {
			value = maskMarker(value, marker)
		}
	}
	return value
}

func isStructuredSensitiveMarker(marker string) bool {
	normalized := strings.ToLower(strings.TrimSpace(marker))
	return normalized == "configvalue" || normalized == "config_value" || normalized == "issensitive" || normalized == "is_sensitive"
}

func policyErrorMessage(violations []PolicyViolationVO) string {
	if len(violations) == 0 {
		return ""
	}
	return fmt.Sprintf("Docker 安全策略拒绝：%s", violations[0].Message)
}
