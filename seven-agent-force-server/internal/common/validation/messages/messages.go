package messages

import (
	"fmt"

	"github.com/go-playground/validator/v10"
)

func Default(field string, err validator.FieldError) string {
	switch err.Tag() {
	case "required":
		return "不能为空"
	case "after_current_time":
		return "必须晚于当前时间"
	case "not_before_current_time":
		return "不能早于当前时间"
	default:
		if err.Param() != "" {
			return fmt.Sprintf("不满足校验规则 %s=%s", err.Tag(), err.Param())
		}
		return fmt.Sprintf("不满足校验规则 %s", err.Tag())
	}
}
