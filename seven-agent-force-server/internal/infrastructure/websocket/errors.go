package websocket

import (
	stderrors "errors"

	apperrors "github.com/CaixyPromise/seven-agent/force/internal/common/errors"
)

var (
	ErrWebSocketClosed             = stderrors.New("websocket client closed")
	ErrWebSocketTimeout            = stderrors.New("websocket client timeout")
	ErrWebSocketReconnectExhausted = stderrors.New("websocket reconnect exhausted")
	ErrWebSocketQueueFull          = stderrors.New("websocket queue full")
)

func wrapClosed(message string, cause error) error {
	return apperrors.Wrap(apperrors.CodeOperateError, apperrors.KindOperation, message, stderrors.Join(ErrWebSocketClosed, cause))
}

func wrapTimeout(message string, cause error) error {
	return apperrors.Wrap(apperrors.CodeOperateError, apperrors.KindOperation, message, stderrors.Join(ErrWebSocketTimeout, cause))
}

func wrapReconnectExhausted(message string, cause error) error {
	return apperrors.Wrap(apperrors.CodeOperateError, apperrors.KindOperation, message, stderrors.Join(ErrWebSocketReconnectExhausted, cause))
}

func wrapQueueFull(message string) error {
	return apperrors.Wrap(apperrors.CodeOperateError, apperrors.KindOperation, message, ErrWebSocketQueueFull)
}

func wrapOperation(message string, cause error) error {
	return apperrors.Wrap(apperrors.CodeOperateError, apperrors.KindOperation, message, cause)
}
