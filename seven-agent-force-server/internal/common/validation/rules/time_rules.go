package rules

import (
	"reflect"
	"strings"
	"time"

	"github.com/go-playground/validator/v10"
)

const (
	TagAfterCurrentTime     = "after_current_time"
	TagNotBeforeCurrentTime = "not_before_current_time"
)

func RegisterTimeRules(v *validator.Validate) error {
	if err := v.RegisterValidation(TagAfterCurrentTime, afterCurrentTime); err != nil {
		return err
	}
	if err := v.RegisterValidation(TagNotBeforeCurrentTime, notBeforeCurrentTime); err != nil {
		return err
	}
	return nil
}

func afterCurrentTime(fl validator.FieldLevel) bool {
	value, exists := extractTime(fl.Field())
	if !exists {
		return allowNull(fl.Param())
	}
	return value.After(time.Now())
}

func notBeforeCurrentTime(fl validator.FieldLevel) bool {
	value, exists := extractTime(fl.Field())
	if !exists {
		return allowNull(fl.Param())
	}
	return !value.Before(time.Now())
}

func extractTime(field reflect.Value) (time.Time, bool) {
	if !field.IsValid() {
		return time.Time{}, false
	}
	if field.Kind() == reflect.Pointer {
		if field.IsNil() {
			return time.Time{}, false
		}
		return extractTime(field.Elem())
	}
	if field.Type() != reflect.TypeOf(time.Time{}) {
		return time.Time{}, false
	}
	value := field.Interface().(time.Time)
	if value.IsZero() {
		return time.Time{}, false
	}
	return value, true
}

func allowNull(param string) bool {
	switch strings.ToLower(strings.TrimSpace(param)) {
	case "false", "0", "no":
		return false
	default:
		return true
	}
}
