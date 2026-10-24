package listener

import (
	"context"
	"errors"
	"testing"
	"time"

	fileapp "github.com/CaixyPromise/seven-agent/force/internal/app/file/application"
	"github.com/CaixyPromise/seven-agent/force/internal/app/file/domain"
)

func TestUploadRetryPublicationCompletesOriginalConsumeLeaseAndDeduplicatesRedelivery(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	handler := newRetryingHandler()
	broker := &retryingBroker{
		uploadMessage:    domain.UploadTaskMessage{MessageID: "upload-message", TaskID: "task-1"},
		uploadDeliveries: 2,
		afterConsume:     cancel,
	}

	consumeUploadTasks(ctx, broker, handler)

	if broker.uploadRetryCalls != 1 {
		t.Fatalf("PublishUploadTaskRetry calls=%d, want 1", broker.uploadRetryCalls)
	}
	if handler.failedCalls != 0 {
		t.Fatalf("MarkConsumeFailed calls=%d, want 0 after successful retry publication", handler.failedCalls)
	}
	if handler.consumedCalls != 1 {
		t.Fatalf("MarkConsumed calls=%d, want 1", handler.consumedCalls)
	}
}

func TestFileProcessRetryPublicationCompletesOriginalConsumeLeaseAndDeduplicatesRedelivery(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	handler := newRetryingHandler()
	broker := &retryingBroker{
		processMessage:    domain.FileProcessMessage{MessageID: "process-message", TaskID: 7, FileID: 9, TaskType: "thumbnail"},
		processDeliveries: 2,
		afterConsume:      cancel,
	}

	consumeFileProcessTasks(ctx, broker, handler)

	if broker.fileRetryCalls != 1 {
		t.Fatalf("PublishFileProcessTask calls=%d, want 1", broker.fileRetryCalls)
	}
	if handler.failedCalls != 0 {
		t.Fatalf("MarkConsumeFailed calls=%d, want 0 after successful retry publication", handler.failedCalls)
	}
	if handler.consumedCalls != 1 {
		t.Fatalf("MarkConsumed calls=%d, want 1", handler.consumedCalls)
	}
}

func TestUploadRetryExhaustionIsTerminalInsteadOfRequeuedForever(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	handler := newRetryingHandler()
	broker := &retryingBroker{
		uploadMessage:    domain.UploadTaskMessage{MessageID: "upload-poison", TaskID: "task-1", Retry: 3},
		uploadDeliveries: 1,
		afterConsume:     cancel,
	}

	consumeUploadTasks(ctx, broker, handler)

	if broker.uploadRetryCalls != 0 {
		t.Fatalf("PublishUploadTaskRetry calls=%d, want 0 after retry exhaustion", broker.uploadRetryCalls)
	}
	if handler.failedCalls != 1 {
		t.Fatalf("MarkConsumeFailed calls=%d, want 1", handler.failedCalls)
	}
	var permanent interface{ Permanent() bool }
	if !errors.As(broker.lastConsumeErr, &permanent) || !permanent.Permanent() {
		t.Fatalf("consume error=%v, want explicit permanent failure", broker.lastConsumeErr)
	}
}

type retryingBroker struct {
	fileapp.MessagePublisherPort
	uploadMessage     domain.UploadTaskMessage
	processMessage    domain.FileProcessMessage
	uploadDeliveries  int
	processDeliveries int
	uploadRetryCalls  int
	fileRetryCalls    int
	lastConsumeErr    error
	afterConsume      context.CancelFunc
}

func (b *retryingBroker) Enabled() bool { return true }

func (b *retryingBroker) ConsumeUploadTasks(ctx context.Context, _ string, callback func(context.Context, domain.UploadTaskMessage) error) error {
	for range b.uploadDeliveries {
		if err := callback(ctx, b.uploadMessage); err != nil {
			b.lastConsumeErr = err
			if b.afterConsume != nil {
				b.afterConsume()
			}
			return err
		}
	}
	if b.afterConsume != nil {
		b.afterConsume()
	}
	return nil
}

func (b *retryingBroker) ConsumeFileProcessTasks(ctx context.Context, _ string, callback func(context.Context, domain.FileProcessMessage) error) error {
	for range b.processDeliveries {
		if err := callback(ctx, b.processMessage); err != nil {
			b.lastConsumeErr = err
			if b.afterConsume != nil {
				b.afterConsume()
			}
			return err
		}
	}
	if b.afterConsume != nil {
		b.afterConsume()
	}
	return nil
}

func (b *retryingBroker) PublishUploadTaskRetry(context.Context, domain.UploadTaskMessage, time.Duration) error {
	b.uploadRetryCalls++
	return nil
}

func (b *retryingBroker) PublishFileProcessTask(context.Context, domain.FileProcessMessage) error {
	b.fileRetryCalls++
	return nil
}

func (b *retryingBroker) Reconnect(context.Context) error { return nil }

type retryingHandler struct {
	done          map[string]bool
	failedCalls   int
	consumedCalls int
}

func newRetryingHandler() *retryingHandler {
	return &retryingHandler{done: make(map[string]bool)}
}

func (h *retryingHandler) BeginConsume(_ context.Context, messageID, _, _, _ string) (*domain.ConsumeLease, bool, error) {
	if h.done[messageID] {
		return nil, false, nil
	}
	return &domain.ConsumeLease{Token: "lease-" + messageID}, true, nil
}

func (h *retryingHandler) MarkConsumed(_ context.Context, messageID, _, _, _ string) (bool, error) {
	h.done[messageID] = true
	h.consumedCalls++
	return true, nil
}

func (h *retryingHandler) MarkConsumeFailed(context.Context, string, string, string, string) (bool, error) {
	h.failedCalls++
	return true, nil
}

func (h *retryingHandler) HandleUploadTaskMessage(context.Context, domain.UploadTaskMessage) error {
	return errors.New("temporary upload dependency failure")
}

func (h *retryingHandler) HandleFileProcessMessage(context.Context, domain.FileProcessMessage) error {
	return errors.New("temporary process dependency failure")
}
