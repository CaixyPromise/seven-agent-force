package httpx

import (
	"fmt"

	apperrors "github.com/CaixyPromise/seven-agent/force/internal/common/errors"
	"github.com/CaixyPromise/seven-agent/force/internal/common/validation"
	"github.com/cloudwego/hertz/pkg/app"
)

func Bind(c *app.RequestContext, request any) error {
	if err := c.Bind(request); err != nil {
		return apperrors.Params(fmt.Sprintf("参数错误: %v", err))
	}
	return nil
}

func Validate(request any) error {
	if err := validation.ValidateStruct(request); err != nil {
		return ValidationError(err)
	}
	return nil
}

func BindAndValidate(c *app.RequestContext, request any) error {
	if err := Bind(c, request); err != nil {
		return err
	}
	if err := Validate(request); err != nil {
		return err
	}
	return nil
}

func ValidationError(err error) error {
	if err == nil {
		return nil
	}
	if typed, ok := err.(*validation.ValidationError); ok {
		return apperrors.ParamsWithDetails(typed.Error(), typed.Violations)
	}
	return apperrors.Params(fmt.Sprintf("参数错误: %v", err))
}
