package websocket

import (
	"context"
	"sync"
)

type textHook func(context.Context, string)
type binaryHook func(context.Context, []byte)
type errorHook func(context.Context, error)
type closeHook func(context.Context, error)

type hookSet struct {
	mu       sync.RWMutex
	onText   textHook
	onBinary binaryHook
	onError  errorHook
	onClose  closeHook
}

func (h *hookSet) setText(fn textHook) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.onText = fn
}

func (h *hookSet) setBinary(fn binaryHook) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.onBinary = fn
}

func (h *hookSet) setError(fn errorHook) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.onError = fn
}

func (h *hookSet) setClose(fn closeHook) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.onClose = fn
}

func (h *hookSet) text() textHook {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return h.onText
}

func (h *hookSet) binary() binaryHook {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return h.onBinary
}

func (h *hookSet) error() errorHook {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return h.onError
}

func (h *hookSet) close() closeHook {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return h.onClose
}
