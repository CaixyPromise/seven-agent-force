package domain

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"html"
	"reflect"
	"regexp"
	"strconv"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"
)

const (
	TemplateRevisionStateDraft      = "DRAFT"
	TemplateRevisionStatePublished  = "PUBLISHED"
	TemplateRevisionStateSuperseded = "SUPERSEDED"

	TemplateVariableTypeString          = "STRING"
	TemplateVariableTypeNumber          = "NUMBER"
	TemplateVariableTypeBoolean         = "BOOLEAN"
	TemplateVariableTypeDateTime        = "DATETIME"
	TemplateVariableTypeSecretEphemeral = "SECRET_EPHEMERAL"

	TemplateVariableClassificationPublic    = "PUBLIC"
	TemplateVariableClassificationSensitive = "SENSITIVE"

	maxTemplateVariableNameLength = 64
	maxTemplateStringLength       = 4096
	maxTemplateSubjectLength      = 512
	maxTemplateBodyLength         = 64 * 1024
)

var (
	ErrTemplateDefinitionNotFound = errors.New("notification template definition was not found in the current scope")
	ErrTemplateRevisionNotFound   = errors.New("notification template revision was not found in the current scope")
	ErrTemplateRevisionConflict   = errors.New("notification template revision has changed; refresh and try again")
	ErrTemplateRevisionImmutable  = errors.New("published notification template revisions are immutable")

	templateVariableNamePattern   = regexp.MustCompile(`^[A-Za-z][A-Za-z0-9_]{0,63}$`)
	templateDefinitionCodePattern = regexp.MustCompile(`^[A-Za-z][A-Za-z0-9_-]{0,95}$`)
)

// TemplateDefinition is the stable versioned-template identity.
type TemplateDefinition struct {
	ID                         int64     `db:"id"`
	ScopeID                    string    `db:"scopeId"`
	TemplateCode               string    `db:"templateCode"`
	TemplateName               string    `db:"templateName"`
	Locale                     string    `db:"locale"`
	CurrentDraftRevisionID     *int64    `db:"currentDraftRevisionId"`
	CurrentPublishedRevisionID *int64    `db:"currentPublishedRevisionId"`
	Version                    int       `db:"version"`
	CreatorID                  *int64    `db:"creatorId"`
	UpdaterID                  *int64    `db:"updaterId"`
	CreateTime                 time.Time `db:"createTime"`
	UpdateTime                 time.Time `db:"updateTime"`
	IsDeleted                  int       `db:"isDeleted"`
}

// TemplateRevision is immutable once published. VariableSchemaJSON is an
// internal representation; callers use TemplateVariable instead.
type TemplateRevision struct {
	ID                         int64      `db:"id"`
	TemplateDefinitionID       int64      `db:"templateDefinitionId"`
	RevisionNo                 int        `db:"revisionNo"`
	State                      string     `db:"state"`
	RevisionVersion            int        `db:"revisionVersion"`
	SubjectTemplate            string     `db:"subjectTemplate"`
	TextTemplate               string     `db:"textTemplate"`
	HTMLTemplate               string     `db:"htmlTemplate"`
	MarkdownTemplate           string     `db:"markdownTemplate"`
	PrimaryActionLabelTemplate string     `db:"primaryActionLabelTemplate"`
	VariableSchemaJSON         string     `db:"variableSchemaJson"`
	ContentDigest              string     `db:"contentDigest"`
	PublishedAt                *time.Time `db:"publishedAt"`
	PublishedBy                *int64     `db:"publishedBy"`
	CreatorID                  *int64     `db:"creatorId"`
	UpdaterID                  *int64     `db:"updaterId"`
	CreateTime                 time.Time  `db:"createTime"`
	UpdateTime                 time.Time  `db:"updateTime"`
}

// TemplateRevisionAudit deliberately contains only identity and lifecycle
// metadata. It never records body content, variable values, or previews.
type TemplateRevisionAudit struct {
	ID                   int64     `db:"id"`
	TemplateDefinitionID int64     `db:"templateDefinitionId"`
	ScopeID              string    `db:"scopeId"`
	Action               string    `db:"action"`
	FromRevisionNo       *int      `db:"fromRevisionNo"`
	ToRevisionNo         *int      `db:"toRevisionNo"`
	ActorID              *int64    `db:"actorId"`
	CreateTime           time.Time `db:"createTime"`
}

type TemplateVariable struct {
	Name           string `json:"name"`
	Type           string `json:"type"`
	Required       bool   `json:"required"`
	MaxLength      int    `json:"maxLength,omitempty"`
	SampleValue    any    `json:"sampleValue,omitempty"`
	Classification string `json:"classification"`
}

// TemplateRevisionDraft is the only editable content shape for G6.1. It has
// no provider, route, scene, target or secret field by design.
type TemplateRevisionDraft struct {
	TemplateName               string             `json:"templateName"`
	Locale                     string             `json:"locale"`
	SubjectTemplate            string             `json:"subjectTemplate"`
	TextTemplate               string             `json:"textTemplate"`
	HTMLTemplate               string             `json:"htmlTemplate"`
	MarkdownTemplate           string             `json:"markdownTemplate"`
	PrimaryActionLabelTemplate string             `json:"primaryActionLabelTemplate"`
	Variables                  []TemplateVariable `json:"variables"`
}

type RenderedTemplateRevision struct {
	Subject            string
	Text               string
	HTML               string
	Markdown           string
	PrimaryActionLabel string
}

type TemplateDefinitionQuery struct {
	ScopeID  string
	Keyword  string
	Current  int
	PageSize int
}

// ValidateTemplateRevisionDraft validates the constrained G6.1 syntax and
// normalizes sensitive sample values away before persistence. It is purposely
// not shared with the V1 Go-template renderer, whose permissive compatibility
// contract must remain unchanged.
func ValidateTemplateRevisionDraft(draft *TemplateRevisionDraft) error {
	if draft == nil {
		return templateValidationError("TEMPLATE_DRAFT_REQUIRED", "模板草稿不能为空")
	}
	draft.TemplateName = strings.TrimSpace(draft.TemplateName)
	draft.Locale = strings.TrimSpace(draft.Locale)
	if draft.TemplateName == "" {
		return templateValidationError("TEMPLATE_NAME_REQUIRED", "模板名称不能为空")
	}
	if draft.Locale == "" {
		draft.Locale = "zh-CN"
	}
	if len(draft.SubjectTemplate) > maxTemplateSubjectLength {
		return templateValidationError("TEMPLATE_SUBJECT_TOO_LONG", "标题模板超过长度限制")
	}
	if utf8.RuneCountInString(draft.PrimaryActionLabelTemplate) > 32 {
		return templateValidationError("TEMPLATE_ACTION_LABEL_TOO_LONG", "主操作文案超过长度限制")
	}
	for _, content := range []struct {
		name  string
		value string
	}{
		{"文本", draft.TextTemplate},
		{"HTML", draft.HTMLTemplate},
		{"Markdown", draft.MarkdownTemplate},
	} {
		if len(content.value) > maxTemplateBodyLength {
			return templateValidationError("TEMPLATE_BODY_TOO_LONG", content.name+"模板超过长度限制")
		}
	}
	if strings.TrimSpace(draft.TextTemplate) == "" && strings.TrimSpace(draft.HTMLTemplate) == "" && strings.TrimSpace(draft.MarkdownTemplate) == "" {
		return templateValidationError("TEMPLATE_BODY_REQUIRED", "至少填写一种正文模板")
	}

	declared := make(map[string]TemplateVariable, len(draft.Variables))
	for index := range draft.Variables {
		variable := &draft.Variables[index]
		variable.Name = strings.TrimSpace(variable.Name)
		variable.Type = strings.ToUpper(strings.TrimSpace(variable.Type))
		variable.Classification = strings.ToUpper(strings.TrimSpace(variable.Classification))
		if variable.Classification == "" {
			variable.Classification = TemplateVariableClassificationPublic
		}
		if !templateVariableNamePattern.MatchString(variable.Name) || len(variable.Name) > maxTemplateVariableNameLength {
			return templateValidationError("TEMPLATE_VARIABLE_NAME_INVALID", "变量名称不合法")
		}
		if _, found := declared[variable.Name]; found {
			return templateValidationError("TEMPLATE_VARIABLE_DUPLICATE", "变量名称重复: "+variable.Name)
		}
		if variable.Type == TemplateVariableTypeSecretEphemeral {
			return templateValidationError("TEMPLATE_SECRET_EPHEMERAL_FORBIDDEN", "普通模板不支持 SECRET_EPHEMERAL 变量")
		}
		if !validTemplateVariableType(variable.Type) {
			return templateValidationError("TEMPLATE_VARIABLE_TYPE_INVALID", "变量类型不支持: "+variable.Name)
		}
		if variable.Classification != TemplateVariableClassificationPublic && variable.Classification != TemplateVariableClassificationSensitive {
			return templateValidationError("TEMPLATE_VARIABLE_CLASSIFICATION_INVALID", "变量分类不支持: "+variable.Name)
		}
		if variable.Type == TemplateVariableTypeString {
			if variable.MaxLength <= 0 || variable.MaxLength > maxTemplateStringLength {
				return templateValidationError("TEMPLATE_VARIABLE_LENGTH_INVALID", "字符串变量最大长度必须在 1 到 4096 之间: "+variable.Name)
			}
		} else if variable.MaxLength != 0 {
			return templateValidationError("TEMPLATE_VARIABLE_LENGTH_INVALID", "仅字符串变量可以设置最大长度: "+variable.Name)
		}
		if variable.Classification == TemplateVariableClassificationSensitive {
			// Sensitive sample values are never persisted, returned, rendered in
			// preview logs, or used as an implicit preview fallback.
			variable.SampleValue = nil
		} else if variable.SampleValue != nil {
			if err := validateTemplateVariableValue(*variable, variable.SampleValue); err != nil {
				return templateValidationError("TEMPLATE_VARIABLE_SAMPLE_INVALID", "变量示例值不合法: "+variable.Name)
			}
		}
		declared[variable.Name] = *variable
	}

	used := make(map[string]struct{}, len(declared))
	for _, content := range []string{draft.SubjectTemplate, draft.TextTemplate, draft.HTMLTemplate, draft.MarkdownTemplate, draft.PrimaryActionLabelTemplate} {
		names, err := collectTemplatePlaceholders(content)
		if err != nil {
			return err
		}
		for _, name := range names {
			if _, found := declared[name]; !found {
				return templateValidationError("TEMPLATE_VARIABLE_UNDECLARED", "模板使用了未声明变量: "+name)
			}
			used[name] = struct{}{}
		}
	}
	for name := range declared {
		if _, found := used[name]; !found {
			return templateValidationError("TEMPLATE_VARIABLE_UNUSED", "变量未使用: "+name)
		}
	}
	return nil
}

// RenderTemplateRevision validates every caller-supplied value before doing a
// direct substitution. It makes no repository, Outbox, channel, provider,
// inbox, realtime, or log call, which keeps preview intrinsically side-effect
// free at the domain boundary.
func RenderTemplateRevision(draft TemplateRevisionDraft, values map[string]any) (RenderedTemplateRevision, error) {
	if err := ValidateTemplateRevisionDraft(&draft); err != nil {
		return RenderedTemplateRevision{}, err
	}
	normalized, err := normalizeTemplateValues(draft.Variables, values)
	if err != nil {
		return RenderedTemplateRevision{}, err
	}
	subject, err := renderTemplateRevisionString(draft.SubjectTemplate, plainTemplateValues(normalized))
	if err != nil {
		return RenderedTemplateRevision{}, err
	}
	text, err := renderTemplateRevisionString(draft.TextTemplate, plainTemplateValues(normalized))
	if err != nil {
		return RenderedTemplateRevision{}, err
	}
	htmlBody, err := renderTemplateRevisionString(draft.HTMLTemplate, htmlTemplateValues(normalized))
	if err != nil {
		return RenderedTemplateRevision{}, err
	}
	markdown, err := renderTemplateRevisionString(draft.MarkdownTemplate, markdownTemplateValues(normalized))
	if err != nil {
		return RenderedTemplateRevision{}, err
	}
	actionLabel, err := renderTemplateRevisionString(draft.PrimaryActionLabelTemplate, plainTemplateValues(normalized))
	if err != nil {
		return RenderedTemplateRevision{}, err
	}
	actionLabel = strings.TrimSpace(actionLabel)
	if actionLabel != "" {
		if _, err := NewPrimaryInternalAction(actionLabel, InternalDeepLink{value: "/"}); err != nil {
			return RenderedTemplateRevision{}, templateValidationError("TEMPLATE_ACTION_LABEL_INVALID", "主操作文案不合法")
		}
	}
	if strings.TrimSpace(text) == "" && strings.TrimSpace(htmlBody) == "" && strings.TrimSpace(markdown) == "" {
		return RenderedTemplateRevision{}, templateValidationError("TEMPLATE_RENDERED_BODY_REQUIRED", "渲染后的正文不能为空")
	}
	return RenderedTemplateRevision{Subject: subject, Text: text, HTML: htmlBody, Markdown: markdown, PrimaryActionLabel: actionLabel}, nil
}

// TemplateRevisionDigest is stored with an accepted revision. It is a content
// identity for audit and optimistic diagnostics, not a substitute for the
// persistence version counter.
func TemplateRevisionDigest(draft TemplateRevisionDraft) (string, error) {
	if err := ValidateTemplateRevisionDraft(&draft); err != nil {
		return "", err
	}
	canonical, err := json.Marshal(struct {
		TemplateName               string             `json:"templateName"`
		Locale                     string             `json:"locale"`
		SubjectTemplate            string             `json:"subjectTemplate"`
		TextTemplate               string             `json:"textTemplate"`
		HTMLTemplate               string             `json:"htmlTemplate"`
		MarkdownTemplate           string             `json:"markdownTemplate"`
		PrimaryActionLabelTemplate string             `json:"primaryActionLabelTemplate"`
		Variables                  []TemplateVariable `json:"variables"`
	}{draft.TemplateName, draft.Locale, draft.SubjectTemplate, draft.TextTemplate, draft.HTMLTemplate, draft.MarkdownTemplate, draft.PrimaryActionLabelTemplate, draft.Variables})
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(canonical)
	return hex.EncodeToString(sum[:]), nil
}

// ValidateTemplateDefinitionCode keeps the new identity namespace small and
// portable across the two supported databases. It does not change legacy V1
// template-code validation or selection.
func ValidateTemplateDefinitionCode(code string) error {
	if !templateDefinitionCodePattern.MatchString(strings.TrimSpace(code)) {
		return templateValidationError("TEMPLATE_CODE_INVALID", "模板编码只能包含字母、数字、下划线和连字符，且必须以字母开头")
	}
	return nil
}

func TemplateVariablesFromJSON(raw string) ([]TemplateVariable, error) {
	if strings.TrimSpace(raw) == "" {
		return []TemplateVariable{}, nil
	}
	var variables []TemplateVariable
	if err := json.Unmarshal([]byte(raw), &variables); err != nil {
		return nil, templateValidationError("TEMPLATE_VARIABLE_SCHEMA_INVALID", "模板变量配置不合法")
	}
	return variables, nil
}

func TemplateVariablesJSON(variables []TemplateVariable) (string, error) {
	value, err := json.Marshal(variables)
	if err != nil {
		return "", err
	}
	return string(value), nil
}

func templateValidationError(code, message string) error {
	return &TemplateValidationError{Code: code, Message: message}
}

// TemplateValidationError intentionally contains a stable code and a safe
// field-level message. It never wraps input values, body content, or secrets.
type TemplateValidationError struct {
	Code    string
	Message string
}

func (e *TemplateValidationError) Error() string {
	if e == nil {
		return ""
	}
	return e.Message
}

func validTemplateVariableType(value string) bool {
	switch value {
	case TemplateVariableTypeString, TemplateVariableTypeNumber, TemplateVariableTypeBoolean, TemplateVariableTypeDateTime:
		return true
	default:
		return false
	}
}

func collectTemplatePlaceholders(pattern string) ([]string, error) {
	if pattern == "" {
		return nil, nil
	}
	names := make([]string, 0)
	for offset := 0; offset < len(pattern); {
		open := strings.Index(pattern[offset:], "{{")
		strayClose := strings.Index(pattern[offset:], "}}")
		if strayClose >= 0 && (open < 0 || strayClose < open) {
			return nil, templateValidationError("TEMPLATE_SYNTAX_INVALID", "模板语法不合法，只允许 {{.变量名}}")
		}
		if open < 0 {
			break
		}
		open += offset
		closeRelative := strings.Index(pattern[open+2:], "}}")
		if closeRelative < 0 {
			return nil, templateValidationError("TEMPLATE_SYNTAX_INVALID", "模板语法不合法，只允许 {{.变量名}}")
		}
		close := open + 2 + closeRelative
		action := strings.TrimSpace(pattern[open+2 : close])
		if !strings.HasPrefix(action, ".") || !templateVariableNamePattern.MatchString(strings.TrimPrefix(action, ".")) {
			return nil, templateValidationError("TEMPLATE_SYNTAX_INVALID", "模板语法不合法，只允许 {{.变量名}}")
		}
		names = append(names, strings.TrimPrefix(action, "."))
		offset = close + 2
	}
	return names, nil
}

func normalizeTemplateValues(schema []TemplateVariable, values map[string]any) (map[string]string, error) {
	if values == nil {
		values = map[string]any{}
	}
	declared := make(map[string]TemplateVariable, len(schema))
	for _, variable := range schema {
		declared[variable.Name] = variable
	}
	for name := range values {
		if _, found := declared[name]; !found {
			return nil, templateValidationError("TEMPLATE_VARIABLE_EXTRA", "预览输入包含未声明变量: "+name)
		}
	}
	normalized := make(map[string]string, len(schema))
	for _, variable := range schema {
		value, found := values[variable.Name]
		if !found || value == nil {
			if variable.Required {
				return nil, templateValidationError("TEMPLATE_VARIABLE_REQUIRED", "缺少必填变量: "+variable.Name)
			}
			normalized[variable.Name] = ""
			continue
		}
		if err := validateTemplateVariableValue(variable, value); err != nil {
			return nil, templateValidationError("TEMPLATE_VARIABLE_VALUE_INVALID", "变量值不合法: "+variable.Name)
		}
		normalized[variable.Name] = templateVariableString(value)
	}
	return normalized, nil
}

func validateTemplateVariableValue(variable TemplateVariable, value any) error {
	switch variable.Type {
	case TemplateVariableTypeString:
		text, ok := value.(string)
		if !ok || utf8.RuneCountInString(text) > variable.MaxLength {
			return fmt.Errorf("invalid string")
		}
	case TemplateVariableTypeNumber:
		if !isNumber(value) {
			return fmt.Errorf("invalid number")
		}
	case TemplateVariableTypeBoolean:
		if _, ok := value.(bool); !ok {
			return fmt.Errorf("invalid boolean")
		}
	case TemplateVariableTypeDateTime:
		switch typed := value.(type) {
		case time.Time:
			if typed.IsZero() {
				return fmt.Errorf("invalid datetime")
			}
		case string:
			if _, err := time.Parse(time.RFC3339, typed); err != nil {
				return fmt.Errorf("invalid datetime")
			}
		default:
			return fmt.Errorf("invalid datetime")
		}
	default:
		return fmt.Errorf("invalid type")
	}
	return nil
}

func isNumber(value any) bool {
	if _, ok := value.(json.Number); ok {
		return true
	}
	kind := reflect.ValueOf(value).Kind()
	switch kind {
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64,
		reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64,
		reflect.Float32, reflect.Float64:
		return true
	default:
		return false
	}
}

func templateVariableString(value any) string {
	switch typed := value.(type) {
	case time.Time:
		return typed.Format(time.RFC3339)
	case string:
		return typed
	case bool:
		return strconv.FormatBool(typed)
	default:
		return fmt.Sprint(value)
	}
}

func renderTemplateRevisionString(pattern string, values map[string]string) (string, error) {
	if pattern == "" {
		return "", nil
	}
	var out strings.Builder
	offset := 0
	for offset < len(pattern) {
		openRelative := strings.Index(pattern[offset:], "{{")
		if openRelative < 0 {
			out.WriteString(pattern[offset:])
			break
		}
		open := offset + openRelative
		out.WriteString(pattern[offset:open])
		closeRelative := strings.Index(pattern[open+2:], "}}")
		if closeRelative < 0 {
			return "", templateValidationError("TEMPLATE_SYNTAX_INVALID", "模板语法不合法，只允许 {{.变量名}}")
		}
		close := open + 2 + closeRelative
		name := strings.TrimPrefix(strings.TrimSpace(pattern[open+2:close]), ".")
		value, found := values[name]
		if !found {
			return "", templateValidationError("TEMPLATE_VARIABLE_UNDECLARED", "模板使用了未声明变量: "+name)
		}
		out.WriteString(value)
		offset = close + 2
	}
	return out.String(), nil
}

func plainTemplateValues(values map[string]string) map[string]string {
	result := make(map[string]string, len(values))
	for key, value := range values {
		result[key] = strings.Map(func(character rune) rune {
			if unicode.IsControl(character) && character != '\t' {
				return ' '
			}
			return character
		}, value)
	}
	return result
}

func htmlTemplateValues(values map[string]string) map[string]string {
	result := make(map[string]string, len(values))
	for key, value := range plainTemplateValues(values) {
		result[key] = html.EscapeString(value)
	}
	return result
}

func markdownTemplateValues(values map[string]string) map[string]string {
	const markdownSpecial = `\\` + "`*_{}[]<>()#+-.!|>~="
	result := make(map[string]string, len(values))
	for key, value := range plainTemplateValues(values) {
		var builder strings.Builder
		for _, character := range value {
			if strings.ContainsRune(markdownSpecial, character) {
				builder.WriteRune('\\')
			}
			builder.WriteRune(character)
		}
		result[key] = builder.String()
	}
	return result
}
