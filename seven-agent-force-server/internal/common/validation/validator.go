package validation

import (
	"errors"
	"fmt"
	"reflect"
	"strings"
	"sync"

	"github.com/CaixyPromise/seven-agent/force/internal/common/validation/messages"
	"github.com/CaixyPromise/seven-agent/force/internal/common/validation/rules"
	"github.com/go-playground/validator/v10"
)

type Violation struct {
	Field   string `json:"field"`
	Rule    string `json:"rule"`
	Message string `json:"message"`
}

type ValidationError struct {
	Violations []Violation `json:"violations"`
}

func (e *ValidationError) Error() string {
	if e == nil || len(e.Violations) == 0 {
		return ""
	}
	parts := make([]string, 0, len(e.Violations))
	for _, violation := range e.Violations {
		parts = append(parts, fmt.Sprintf("%s: %s", violation.Field, violation.Message))
	}
	return "参数错误: {" + strings.Join(parts, ", ") + "}"
}

var (
	once sync.Once
	v    *validator.Validate
)

func Validator() *validator.Validate {
	once.Do(func() {
		v = validator.New(validator.WithRequiredStructEnabled())
		v.RegisterTagNameFunc(resolveFieldName)
		if err := rules.RegisterTimeRules(v); err != nil {
			panic(err)
		}
	})
	return v
}

func ValidateStruct(value any) error {
	if value == nil {
		return nil
	}
	if err := Validator().Struct(value); err != nil {
		var validationErrors validator.ValidationErrors
		if errors.As(err, &validationErrors) {
			return Normalize(validationErrors)
		}
		return err
	}
	return nil
}

func Normalize(validationErrors validator.ValidationErrors) *ValidationError {
	violations := make([]Violation, 0, len(validationErrors))
	for _, item := range validationErrors {
		field := item.Field()
		if field == "" {
			field = item.StructField()
		}
		violations = append(violations, Violation{
			Field:   field,
			Rule:    item.Tag(),
			Message: messages.Default(field, item),
		})
	}
	return &ValidationError{Violations: violations}
}

func resolveFieldName(field reflect.StructField) string {
	for _, tag := range []string{"json", "query", "form", "path", "header"} {
		value := field.Tag.Get(tag)
		if value == "" || value == "-" {
			continue
		}
		name := strings.TrimSpace(strings.Split(value, ",")[0])
		if name != "" && name != "-" {
			return name
		}
	}
	return field.Name
}
