package application

import (
	"strings"

	"github.com/CaixyPromise/seven-agent/force/internal/app/notification/domain"
	"github.com/CaixyPromise/seven-agent/force/internal/app/notification/facade"
)

func domainTemplateRevisionDraft(input facade.TemplateRevisionDraftInput) domain.TemplateRevisionDraft {
	variables := make([]domain.TemplateVariable, 0, len(input.Variables))
	for _, variable := range input.Variables {
		variables = append(variables, domain.TemplateVariable{
			Name:           variable.Name,
			Type:           variable.Type,
			Required:       variable.Required,
			MaxLength:      variable.MaxLength,
			SampleValue:    variable.SampleValue,
			Classification: variable.Classification,
		})
	}
	return domain.TemplateRevisionDraft{
		TemplateName:               strings.TrimSpace(input.TemplateName),
		Locale:                     strings.TrimSpace(input.Locale),
		SubjectTemplate:            input.SubjectTemplate,
		TextTemplate:               input.TextTemplate,
		HTMLTemplate:               input.HTMLTemplate,
		MarkdownTemplate:           input.MarkdownTemplate,
		PrimaryActionLabelTemplate: input.PrimaryActionLabelTemplate,
		Variables:                  variables,
	}
}

func facadeTemplateRevisionVariables(variables []domain.TemplateVariable) []facade.TemplateRevisionVariable {
	result := make([]facade.TemplateRevisionVariable, 0, len(variables))
	for _, variable := range variables {
		sample := variable.SampleValue
		if strings.EqualFold(variable.Classification, domain.TemplateVariableClassificationSensitive) {
			sample = nil
		}
		result = append(result, facade.TemplateRevisionVariable{
			Name:           variable.Name,
			Type:           variable.Type,
			Required:       variable.Required,
			MaxLength:      variable.MaxLength,
			SampleValue:    sample,
			Classification: variable.Classification,
		})
	}
	return result
}

func mapTemplateRevision(item domain.TemplateRevision) *facade.TemplateRevisionRecord {
	variables, err := domain.TemplateVariablesFromJSON(item.VariableSchemaJSON)
	if err != nil {
		// Stored data must already be validated. Treat a corrupt legacy/manual
		// row as an empty schema instead of echoing its raw JSON to callers.
		variables = nil
	}
	return &facade.TemplateRevisionRecord{
		ID:                         item.ID,
		RevisionNo:                 item.RevisionNo,
		State:                      item.State,
		RevisionVersion:            item.RevisionVersion,
		SubjectTemplate:            item.SubjectTemplate,
		TextTemplate:               item.TextTemplate,
		HTMLTemplate:               item.HTMLTemplate,
		MarkdownTemplate:           item.MarkdownTemplate,
		PrimaryActionLabelTemplate: item.PrimaryActionLabelTemplate,
		Variables:                  facadeTemplateRevisionVariables(variables),
		ContentDigest:              item.ContentDigest,
		PublishedAt:                item.PublishedAt,
		PublishedBy:                item.PublishedBy,
		CreateTime:                 item.CreateTime,
		UpdateTime:                 item.UpdateTime,
	}
}

func mapTemplateDefinition(item domain.TemplateDefinition, draft, published *domain.TemplateRevision) *facade.TemplateDefinitionRecord {
	record := &facade.TemplateDefinitionRecord{
		ID:           item.ID,
		TemplateCode: item.TemplateCode,
		TemplateName: item.TemplateName,
		Locale:       item.Locale,
		Version:      item.Version,
		CreateTime:   item.CreateTime,
		UpdateTime:   item.UpdateTime,
	}
	if draft != nil {
		record.CurrentDraft = mapTemplateRevision(*draft)
	}
	if published != nil {
		record.CurrentPublished = mapTemplateRevision(*published)
	}
	return record
}

func mapTemplateRevisions(items []domain.TemplateRevision) []facade.TemplateRevisionRecord {
	if len(items) == 0 {
		return nil
	}
	records := make([]facade.TemplateRevisionRecord, 0, len(items))
	for _, item := range items {
		records = append(records, *mapTemplateRevision(item))
	}
	return records
}
