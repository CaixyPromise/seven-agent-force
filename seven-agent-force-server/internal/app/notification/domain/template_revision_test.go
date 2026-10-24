package domain

import (
	"errors"
	"strings"
	"testing"
)

func validTemplateRevisionDraft() TemplateRevisionDraft {
	return TemplateRevisionDraft{
		TemplateName:    "账户提醒",
		Locale:          "zh-CN",
		SubjectTemplate: "{{.title}}",
		TextTemplate:    "你好，{{.name}}，余额 {{.amount}}。",
		Variables: []TemplateVariable{
			{Name: "title", Type: TemplateVariableTypeString, Required: true, MaxLength: 80, Classification: TemplateVariableClassificationPublic},
			{Name: "name", Type: TemplateVariableTypeString, Required: true, MaxLength: 80, Classification: TemplateVariableClassificationPublic},
			{Name: "amount", Type: TemplateVariableTypeNumber, Required: false, Classification: TemplateVariableClassificationPublic},
		},
	}
}

func TestValidateTemplateRevisionDraftRejectsUnsafeOrAmbiguousTemplate(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*TemplateRevisionDraft)
		want   string
	}{
		{
			name:   "undeclared placeholder",
			mutate: func(draft *TemplateRevisionDraft) { draft.TextTemplate = "{{.missing}}" },
			want:   "未声明",
		},
		{
			name: "unused variable",
			mutate: func(draft *TemplateRevisionDraft) {
				draft.Variables = append(draft.Variables, TemplateVariable{Name: "unused", Type: TemplateVariableTypeString, MaxLength: 32, Classification: TemplateVariableClassificationPublic})
			},
			want: "未使用",
		},
		{
			name:   "template action",
			mutate: func(draft *TemplateRevisionDraft) { draft.TextTemplate = "{{if .name}}x{{end}}" },
			want:   "只允许",
		},
		{
			name: "secret ephemeral",
			mutate: func(draft *TemplateRevisionDraft) {
				draft.Variables = append(draft.Variables, TemplateVariable{Name: "token", Type: TemplateVariableTypeSecretEphemeral, Classification: TemplateVariableClassificationSensitive})
				draft.TextTemplate += "{{.token}}"
			},
			want: "SECRET_EPHEMERAL",
		},
		{
			name: "empty body",
			mutate: func(draft *TemplateRevisionDraft) {
				draft.TextTemplate = ""
				draft.Variables = draft.Variables[:1]
			},
			want: "正文",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			draft := validTemplateRevisionDraft()
			tt.mutate(&draft)
			err := ValidateTemplateRevisionDraft(&draft)
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("ValidateTemplateRevisionDraft() error=%v, want %q", err, tt.want)
			}
		})
	}
}

func TestRenderTemplateRevisionStrictlyValidatesInputAndDoesNotRetainSensitiveSamples(t *testing.T) {
	draft := validTemplateRevisionDraft()
	draft.Variables[0].SampleValue = "公开示例"
	draft.Variables[1].Classification = TemplateVariableClassificationSensitive
	draft.Variables[1].SampleValue = "不应保存"
	if err := ValidateTemplateRevisionDraft(&draft); err != nil {
		t.Fatalf("ValidateTemplateRevisionDraft() error=%v", err)
	}
	if draft.Variables[1].SampleValue != nil {
		t.Fatalf("sensitive sample value was retained: %#v", draft.Variables[1].SampleValue)
	}

	rendered, err := RenderTemplateRevision(draft, map[string]any{"title": "余额提醒", "name": "小七", "amount": 12.5})
	if err != nil {
		t.Fatalf("RenderTemplateRevision() error=%v", err)
	}
	if rendered.Subject != "余额提醒" || rendered.Text != "你好，小七，余额 12.5。" {
		t.Fatalf("RenderTemplateRevision()=%+v", rendered)
	}

	for _, variables := range []map[string]any{
		{"title": "余额提醒", "name": "小七", "extra": "not allowed"},
		{"title": "余额提醒", "name": 7},
		{"title": "余额提醒"},
		{"title": strings.Repeat("x", 81), "name": "小七"},
	} {
		if _, err := RenderTemplateRevision(draft, variables); err == nil {
			t.Fatalf("RenderTemplateRevision(%#v) unexpectedly succeeded", variables)
		}
	}
}

func TestRenderTemplateRevisionRejectsBlankRenderedBody(t *testing.T) {
	draft := TemplateRevisionDraft{
		TemplateName: "空正文",
		Locale:       "zh-CN",
		TextTemplate: "{{.optional}}",
		Variables: []TemplateVariable{{
			Name:           "optional",
			Type:           TemplateVariableTypeString,
			Required:       false,
			MaxLength:      80,
			Classification: TemplateVariableClassificationPublic,
		}},
	}
	_, err := RenderTemplateRevision(draft, nil)
	var validationErr *TemplateValidationError
	if !errors.As(err, &validationErr) || validationErr.Code != "TEMPLATE_RENDERED_BODY_REQUIRED" {
		t.Fatalf("RenderTemplateRevision() error=%v, want TEMPLATE_RENDERED_BODY_REQUIRED", err)
	}
}
