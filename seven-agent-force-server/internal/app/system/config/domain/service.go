package domain

import (
	"slices"
	"strconv"
	"strings"

	apperrors "github.com/CaixyPromise/seven-agent/force/internal/common/errors"
	"github.com/bytedance/sonic"
)

const maskedValue = "******"

type Service struct{}

func NewService() *Service {
	return &Service{}
}

func (s *Service) NormalizePage(current, size int64) (int64, int64) {
	if current <= 0 {
		current = 1
	}
	if size <= 0 {
		size = 10
	}
	return current, size
}

func (s *Service) NormalizeValueType(valueType string) string {
	return strings.ToLower(strings.TrimSpace(valueType))
}

func (s *Service) NormalizeEffectType(value string) string {
	trimmed := strings.ToLower(strings.TrimSpace(value))
	if trimmed == "" {
		return string(ConfigEffectRealtime)
	}
	return trimmed
}

func (s *Service) NormalizeGroupCode(value string) string {
	return strings.TrimSpace(value)
}

func (s *Service) NormalizeConfigKey(value string) string {
	return strings.TrimSpace(value)
}

func (s *Service) ValidateStatus(status int) error {
	if status != 0 && status != 1 {
		return apperrors.Params("状态值无效，只能为 0 或 1")
	}
	return nil
}

func (s *Service) ValidateValueType(valueType string) error {
	switch s.NormalizeValueType(valueType) {
	case "string", "int", "boolean", "json", "enum":
		return nil
	default:
		return apperrors.Params("无效的值类型：" + valueType)
	}
}

func (s *Service) NormalizeExtJSON(ext *ConfigExtJSON, valueType string) (*ConfigExtJSON, error) {
	if ext == nil {
		ext = &ConfigExtJSON{}
	}
	copyValue := ext.Copy()
	if s.NormalizeValueType(valueType) == "enum" {
		copyValue.Enums = normalizeEnums(copyValue.Enums)
		if len(copyValue.Enums) == 0 {
			return nil, apperrors.Params("enum类型必须配置 extJson.enums")
		}
		return copyValue, nil
	}
	copyValue.Enums = nil
	return copyValue, nil
}

func (s *Service) MergeExtJSON(existing, incoming *ConfigExtJSON) *ConfigExtJSON {
	if incoming == nil {
		if existing == nil {
			return &ConfigExtJSON{}
		}
		return existing.Copy()
	}
	merged := incoming.Copy()
	if merged.Secret == nil && existing != nil && existing.Secret != nil {
		copied := *existing.Secret
		merged.Secret = &copied
	}
	return merged
}

func (s *Service) ValidateConfigValue(value, valueType string, ext *ConfigExtJSON) error {
	switch s.NormalizeValueType(valueType) {
	case "string":
		return nil
	case "int":
		if strings.TrimSpace(value) == "" {
			return apperrors.Params("int类型的配置值不能为空")
		}
		if _, err := strconv.Atoi(strings.TrimSpace(value)); err != nil {
			return apperrors.Params("配置值必须为整数，当前值：" + value)
		}
		return nil
	case "boolean":
		switch strings.ToLower(strings.TrimSpace(value)) {
		case "true", "false", "0", "1":
			return nil
		default:
			return apperrors.Params("配置值必须为 true/false 或 0/1，当前值：" + value)
		}
	case "json":
		if strings.TrimSpace(value) == "" {
			return apperrors.Params("json类型的配置值不能为空")
		}
		if !sonic.Valid([]byte(value)) {
			return apperrors.Params("配置值必须是合法的JSON格式")
		}
		return nil
	case "enum":
		if strings.TrimSpace(value) == "" {
			return apperrors.Params("enum类型的配置值不能为空")
		}
		if ext == nil || len(ext.Enums) == 0 {
			return apperrors.Params("enum类型必须配置 extJson.enums")
		}
		if !slices.Contains(ext.Enums, strings.TrimSpace(value)) {
			return apperrors.Params("配置值不在允许的枚举范围内：" + value)
		}
		return nil
	default:
		return apperrors.Params("无效的值类型：" + valueType)
	}
}

func (s *Service) BuildChangeLog(input CreateChangeLogInput) *ConfigChangeLog {
	now := input.Now.UTC()
	item := &ConfigChangeLog{
		ConfigID:        input.ConfigID,
		ConfigKey:       strings.TrimSpace(input.ConfigKey),
		OperationType:   string(input.OperationType),
		OldValue:        input.OldValue,
		NewValue:        input.NewValue,
		EffectType:      s.NormalizeEffectType(input.EffectType),
		ParentLogID:     input.ParentLogID,
		RelatedLogID:    input.RelatedLogID,
		OperatorID:      input.OperatorID,
		OperationReason: strings.TrimSpace(input.OperationReason),
		OperationTime:   &now,
	}
	// These values are only constructed by the application from the private
	// file facade capture result. Their simple, typed representation makes JSON
	// marshaling deterministic; validation happens again before restore.
	_ = item.SetPrivateAssetSnapshots(input.OldAssetSnapshot, input.NewAssetSnapshot)
	switch {
	case input.OperationType == ConfigOperationApply:
		item.Status = string(ConfigStatusApplied)
		appliedBy := input.OperatorID
		item.AppliedBy = &appliedBy
		item.AppliedTime = &now
	case s.NormalizeEffectType(input.EffectType) == string(ConfigEffectRealtime):
		item.Status = string(ConfigStatusApplied)
	case s.NormalizeEffectType(input.EffectType) == string(ConfigEffectRestart):
		item.Status = string(ConfigStatusPending)
	default:
		item.Status = string(ConfigStatusPending)
	}
	if input.IsStartup && input.OperatorID == 0 {
		item.OperatorID = 0
	}
	return item
}

func (s *Service) SanitizeExtJSON(ext *ConfigExtJSON) *ConfigExtJSON {
	if ext == nil {
		return nil
	}
	sanitized := ext.Copy()
	sanitized.Secret = nil
	return sanitized
}

func (s *Service) MaskSensitive(value string, isSensitive int) string {
	if isSensitive == 1 {
		return maskedValue
	}
	return value
}

func (s *Service) IsMaskedSensitivePlaceholder(value string) bool {
	return strings.TrimSpace(value) == maskedValue
}

func normalizeEnums(values []string) []string {
	if len(values) == 0 {
		return nil
	}
	result := make([]string, 0, len(values))
	seen := make(map[string]struct{}, len(values))
	for _, item := range values {
		trimmed := strings.TrimSpace(item)
		if trimmed == "" {
			continue
		}
		if _, ok := seen[trimmed]; ok {
			continue
		}
		seen[trimmed] = struct{}{}
		result = append(result, trimmed)
	}
	return result
}

func (e *ConfigExtJSON) Copy() *ConfigExtJSON {
	if e == nil {
		return &ConfigExtJSON{}
	}
	copied := &ConfigExtJSON{}
	if len(e.Enums) > 0 {
		copied.Enums = append([]string(nil), e.Enums...)
	}
	if e.Secret != nil {
		secret := *e.Secret
		copied.Secret = &secret
	}
	return copied
}
