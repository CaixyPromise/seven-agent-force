package domain

import (
	"regexp"
	"sort"
	"strings"
	"time"

	apperrors "github.com/CaixyPromise/seven-agent/force/internal/common/errors"
	"github.com/bytedance/sonic"
)

var dictCodePattern = regexp.MustCompile(`^[a-zA-Z0-9_]{2,50}$`)

type Service struct{}

func NewService() *Service {
	return &Service{}
}

func (s *Service) NormalizeDictCode(value string) string {
	return strings.ToLower(strings.TrimSpace(value))
}

func (s *Service) BuildDictCodeVariants(value string) []string {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return nil
	}
	seen := map[string]struct{}{}
	result := make([]string, 0, 3)
	for _, item := range []string{trimmed, strings.ToLower(trimmed), strings.ToUpper(trimmed)} {
		if _, ok := seen[item]; ok {
			continue
		}
		seen[item] = struct{}{}
		result = append(result, item)
	}
	return result
}

func (s *Service) CanonicalBatchCodes(codes []string) []string {
	result := make([]string, 0, len(codes))
	seen := make(map[string]struct{}, len(codes))
	for _, code := range codes {
		trimmed := strings.TrimSpace(code)
		normalized := s.NormalizeDictCode(trimmed)
		if normalized == "" {
			continue
		}
		if _, ok := seen[normalized]; ok {
			continue
		}
		seen[normalized] = struct{}{}
		result = append(result, trimmed)
	}
	sort.Strings(result)
	return result
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

func (s *Service) ValidateDictCode(code string) error {
	trimmed := strings.TrimSpace(code)
	if trimmed == "" {
		return apperrors.Params("dictCode不能为空")
	}
	if !dictCodePattern.MatchString(trimmed) {
		return apperrors.Params("dictCode格式不正确，只能包含字母、数字和下划线，长度2-50")
	}
	return nil
}

func (s *Service) ValidateStatus(status int) error {
	if status != 0 && status != 1 {
		return apperrors.Params("状态值无效，只能为 0（禁用）或 1（启用）")
	}
	return nil
}

func (s *Service) EnsureSystemTypeAllowed(isSystem int, isAdmin bool) error {
	if isSystem == 1 && !isAdmin {
		return apperrors.Forbidden("只有超管才能添加系统内置字典类型")
	}
	return nil
}

func (s *Service) NewDictType(actorID int64, input CreateDictTypeInput, now time.Time) (*DictType, error) {
	if err := s.ValidateDictCode(input.DictCode); err != nil {
		return nil, err
	}
	name := strings.TrimSpace(input.DictName)
	if name == "" {
		return nil, apperrors.Params("dictName不能为空")
	}
	status := 1
	if input.Status != nil {
		status = *input.Status
	}
	if err := s.ValidateStatus(status); err != nil {
		return nil, err
	}
	isSystem := 0
	if input.IsSystem != nil {
		isSystem = *input.IsSystem
	}
	requiredLogin := 0
	if input.RequiredLogin != nil {
		requiredLogin = *input.RequiredLogin
	}
	trimmedDesc := strings.TrimSpace(input.DictDesc)
	trimmedModule := strings.TrimSpace(input.Module)
	valueType, widget, exposure, sensitivity, schemaVersion, err := normalizeDictMetadata(
		input.ValueType, input.UIWidget, input.Exposure, input.Sensitivity, input.SchemaVersion,
	)
	if err != nil {
		return nil, err
	}
	if err := validateDictValidationJSON(input.ValidationJSON); err != nil {
		return nil, err
	}
	now = now.UTC()
	return &DictType{
		DictCode:       s.NormalizeDictCode(input.DictCode),
		DictName:       name,
		DictDesc:       trimmedDesc,
		Module:         trimmedModule,
		Status:         status,
		RequiredLogin:  requiredLogin,
		ValueType:      valueType,
		UIWidget:       widget,
		ValidationJSON: strings.TrimSpace(input.ValidationJSON),
		Exposure:       exposure,
		Sensitivity:    sensitivity,
		SchemaVersion:  schemaVersion,
		Version:        1,
		IsSystem:       isSystem,
		SortOrder:      0,
		CreatedBy:      actorID,
		UpdatedBy:      actorID,
		CreateTime:     &now,
		UpdateTime:     &now,
		IsDeleted:      0,
	}, nil
}

func (s *Service) ApplyDictTypeUpdate(item *DictType, actorID int64, input UpdateDictTypeInput, now time.Time) error {
	if item == nil {
		return apperrors.NotFound("字典类型不存在")
	}
	if input.DictName != nil {
		name := strings.TrimSpace(*input.DictName)
		if name != "" {
			item.DictName = name
		}
	}
	if input.DictDesc != nil {
		item.DictDesc = strings.TrimSpace(*input.DictDesc)
	}
	if input.Module != nil {
		item.Module = strings.TrimSpace(*input.Module)
	}
	if input.Status != nil {
		if err := s.ValidateStatus(*input.Status); err != nil {
			return err
		}
		item.Status = *input.Status
	}
	if input.SortOrder != nil {
		item.SortOrder = *input.SortOrder
	}
	if input.RequiredLogin != nil {
		item.RequiredLogin = *input.RequiredLogin
	}
	valueType, widget, exposure, sensitivity, schemaVersion, err := normalizeDictMetadata(
		item.ValueType, item.UIWidget, item.Exposure, item.Sensitivity, &item.SchemaVersion,
	)
	if input.ValueType != nil {
		valueType, widget, exposure, sensitivity, schemaVersion, err = normalizeDictMetadata(
			*input.ValueType, widget, exposure, sensitivity, &schemaVersion,
		)
	}
	if input.UIWidget != nil {
		widget = strings.ToUpper(strings.TrimSpace(*input.UIWidget))
	}
	if input.Exposure != nil {
		exposure = strings.ToUpper(strings.TrimSpace(*input.Exposure))
	}
	if input.Sensitivity != nil {
		sensitivity = strings.ToUpper(strings.TrimSpace(*input.Sensitivity))
	}
	if input.SchemaVersion != nil {
		schemaVersion = *input.SchemaVersion
	}
	if err == nil {
		valueType, widget, exposure, sensitivity, schemaVersion, err = normalizeDictMetadata(valueType, widget, exposure, sensitivity, &schemaVersion)
	}
	if err != nil {
		return err
	}
	if input.ValidationJSON != nil {
		if err := validateDictValidationJSON(*input.ValidationJSON); err != nil {
			return err
		}
		item.ValidationJSON = strings.TrimSpace(*input.ValidationJSON)
	}
	item.ValueType, item.UIWidget, item.Exposure, item.Sensitivity, item.SchemaVersion = valueType, widget, exposure, sensitivity, schemaVersion
	now = now.UTC()
	item.UpdatedBy = actorID
	item.UpdateTime = &now
	return nil
}

func (s *Service) ChangeDictTypeStatus(item *DictType, actorID int64, status int, now time.Time) error {
	if item == nil {
		return apperrors.NotFound("字典类型不存在")
	}
	if err := s.ValidateStatus(status); err != nil {
		return err
	}
	now = now.UTC()
	item.Status = status
	item.UpdatedBy = actorID
	item.UpdateTime = &now
	return nil
}

func (s *Service) MarkDictTypeDeleted(item *DictType, actorID int64, now time.Time) error {
	if item == nil {
		return apperrors.NotFound("字典类型不存在")
	}
	now = now.UTC()
	item.IsDeleted = 1
	item.UpdatedBy = actorID
	item.UpdateTime = &now
	return nil
}

func (s *Service) NewDictItem(actorID, typeID int64, input CreateDictItemInput, now time.Time) (*DictItem, error) {
	itemValue := strings.TrimSpace(input.ItemValue)
	if itemValue == "" {
		return nil, apperrors.Params("itemValue不能为空")
	}
	itemLabel := strings.TrimSpace(input.ItemLabel)
	if itemLabel == "" {
		return nil, apperrors.Params("itemLabel不能为空")
	}
	status := 1
	if input.Status != nil {
		status = *input.Status
	}
	if err := s.ValidateStatus(status); err != nil {
		return nil, err
	}
	sortOrder := 0
	if input.SortOrder != nil {
		sortOrder = *input.SortOrder
	}
	colorToken := normalizeColorToken(input.ColorToken)
	if strings.TrimSpace(input.ColorToken) != "" && colorToken == "" {
		return nil, apperrors.Params("不支持的颜色令牌")
	}
	iconToken := normalizeIconToken(input.IconToken)
	if strings.TrimSpace(input.IconToken) != "" && iconToken == "" {
		return nil, apperrors.Params("不支持的图标令牌")
	}
	now = now.UTC()
	return &DictItem{
		DictTypeID:          typeID,
		ItemValue:           itemValue,
		ItemLabel:           itemLabel,
		ItemDesc:            strings.TrimSpace(input.ItemDesc),
		SortOrder:           sortOrder,
		Status:              status,
		ExtJSON:             strings.TrimSpace(input.ExtJSON),
		ColorToken:          colorToken,
		IconToken:           iconToken,
		PresentationVersion: 1,
		Version:             1,
		CreatedBy:           actorID,
		UpdatedBy:           actorID,
		CreateTime:          &now,
		UpdateTime:          &now,
		IsDeleted:           0,
	}, nil
}

func (s *Service) ApplyDictItemUpdate(item *DictItem, actorID int64, input UpdateDictItemInput, now time.Time) error {
	if item == nil {
		return apperrors.NotFound("字典项不存在")
	}
	if input.ItemLabel != nil {
		label := strings.TrimSpace(*input.ItemLabel)
		if label != "" {
			item.ItemLabel = label
		}
	}
	if input.ItemDesc != nil {
		item.ItemDesc = strings.TrimSpace(*input.ItemDesc)
	}
	if input.SortOrder != nil {
		item.SortOrder = *input.SortOrder
	}
	if input.Status != nil {
		if err := s.ValidateStatus(*input.Status); err != nil {
			return err
		}
		item.Status = *input.Status
	}
	if input.ExtJSON != nil {
		item.ExtJSON = strings.TrimSpace(*input.ExtJSON)
	}
	if input.ColorToken != nil {
		item.ColorToken = normalizeColorToken(*input.ColorToken)
		if strings.TrimSpace(*input.ColorToken) != "" && item.ColorToken == "" {
			return apperrors.Params("不支持的颜色令牌")
		}
	}
	if input.IconToken != nil {
		item.IconToken = normalizeIconToken(*input.IconToken)
		if strings.TrimSpace(*input.IconToken) != "" && item.IconToken == "" {
			return apperrors.Params("不支持的图标令牌")
		}
	}
	now = now.UTC()
	item.UpdatedBy = actorID
	item.UpdateTime = &now
	return nil
}

func normalizeDictMetadata(valueType, widget, exposure, sensitivity string, schemaVersion *int) (string, string, string, string, int, error) {
	valueType = strings.ToUpper(strings.TrimSpace(valueType))
	if valueType == "" {
		valueType = "STRING"
	}
	switch valueType {
	case "STRING", "INTEGER", "DECIMAL", "BOOLEAN", "DATE", "DATETIME", "DURATION", "COLOR":
	default:
		return "", "", "", "", 0, apperrors.Params("字典值类型不受支持")
	}
	widget = strings.ToUpper(strings.TrimSpace(widget))
	if widget == "" {
		widget = "SELECT"
	}
	if widget != "SELECT" {
		return "", "", "", "", 0, apperrors.Params("字典首版仅支持 SELECT 控件")
	}
	exposure = strings.ToUpper(strings.TrimSpace(exposure))
	if exposure == "" {
		exposure = "INTERNAL"
	}
	if exposure != "INTERNAL" && exposure != "AUTHENTICATED" && exposure != "PUBLIC" {
		return "", "", "", "", 0, apperrors.Params("字典暴露级别不受支持")
	}
	sensitivity = strings.ToUpper(strings.TrimSpace(sensitivity))
	if sensitivity == "" {
		sensitivity = "NORMAL"
	}
	if sensitivity != "NORMAL" && sensitivity != "SENSITIVE" && sensitivity != "SECRET" {
		return "", "", "", "", 0, apperrors.Params("字典敏感级别不受支持")
	}
	version := 1
	if schemaVersion != nil {
		version = *schemaVersion
	}
	if version != 1 {
		return "", "", "", "", 0, apperrors.Params("字典 schemaVersion 不受支持")
	}
	return valueType, widget, exposure, sensitivity, version, nil
}

func validateDictValidationJSON(raw string) error {
	if strings.TrimSpace(raw) == "" {
		return nil
	}
	var value map[string]any
	if err := sonic.Unmarshal([]byte(raw), &value); err != nil {
		return apperrors.Params("字典 validation 必须是合法 JSON 对象")
	}
	for key := range value {
		switch key {
		case "required", "minLength", "maxLength":
		default:
			return apperrors.Params("字典 validation 包含不支持字段")
		}
	}
	return nil
}

func normalizeColorToken(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	switch value {
	case "", "gray", "blue", "pink", "green", "orange", "red", "purple":
		return value
	default:
		return ""
	}
}

func normalizeIconToken(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	switch value {
	case "", "unknown", "male", "female", "check", "close", "info":
		return value
	default:
		return ""
	}
}

func (s *Service) ChangeDictItemStatus(item *DictItem, actorID int64, status int, now time.Time) error {
	if item == nil {
		return apperrors.NotFound("字典项不存在")
	}
	if err := s.ValidateStatus(status); err != nil {
		return err
	}
	now = now.UTC()
	item.Status = status
	item.UpdatedBy = actorID
	item.UpdateTime = &now
	return nil
}

func (s *Service) MarkDictItemDeleted(item *DictItem, actorID int64, now time.Time) error {
	if item == nil {
		return apperrors.NotFound("字典项不存在")
	}
	now = now.UTC()
	item.IsDeleted = 1
	item.UpdatedBy = actorID
	item.UpdateTime = &now
	return nil
}
