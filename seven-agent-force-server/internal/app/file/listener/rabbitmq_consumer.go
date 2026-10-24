package listener

import (
	"context"
	"fmt"
	"time"

	fileapp "github.com/CaixyPromise/seven-agent/force/internal/app/file/application"
	"github.com/CaixyPromise/seven-agent/force/internal/app/file/domain"
)

type UploadTaskHandler interface {
	BeginConsume(ctx context.Context, messageID, consumer, worker, detail string) (*domain.ConsumeLease, bool, error)
	MarkConsumed(ctx context.Context, messageID, consumer, leaseToken, detail string) (bool, error)
	MarkConsumeFailed(ctx context.Context, messageID, consumer, leaseToken, detail string) (bool, error)
	HandleUploadTaskMessage(ctx context.Context, message domain.UploadTaskMessage) error
	HandleFileProcessMessage(ctx context.Context, message domain.FileProcessMessage) error
}

func StartRabbitConsumers(ctx context.Context, broker fileapp.MessagePublisherPort, handler UploadTaskHandler) {
	if broker == nil || !broker.Enabled() || handler == nil {
		return
	}
	go consumeUploadTasks(ctx, broker, handler)
	go consumeFileProcessTasks(ctx, broker, handler)
}

func consumeUploadTasks(ctx context.Context, broker fileapp.MessagePublisherPort, handler UploadTaskHandler) {
	for {
		if ctx.Err() != nil {
			return
		}
		_ = broker.ConsumeUploadTasks(ctx, "file-upload-task", func(ctx context.Context, message domain.UploadTaskMessage) error {
			consumeID := retryConsumeID(message.MessageID, message.Retry)
			lease, claimed, err := handler.BeginConsume(ctx, consumeID, "file-upload-task", "file-upload-task-consumer", "upload task")
			if err != nil || !claimed {
				return err
			}
			if err := handler.HandleUploadTaskMessage(ctx, message); err != nil {
				if message.Retry < 3 {
					message.Retry++
					if publishErr := broker.PublishUploadTaskRetry(ctx, message, backoff(message.Retry)); publishErr != nil {
						_, _ = handler.MarkConsumeFailed(ctx, consumeID, "file-upload-task", lease.Token, publishErr.Error())
						return publishErr
					}
					marked, markErr := handler.MarkConsumed(ctx, consumeID, "file-upload-task", lease.Token, "upload task retry published")
					if markErr != nil {
						return markErr
					}
					if !marked {
						return fmt.Errorf("upload task consume lease was lost after retry publication")
					}
					return nil
				}
				_, _ = handler.MarkConsumeFailed(ctx, consumeID, "file-upload-task", lease.Token, err.Error())
				return fileapp.PermanentConsumeError(err)
			}
			_, err = handler.MarkConsumed(ctx, consumeID, "file-upload-task", lease.Token, "upload task")
			return err
		})
		if ctx.Err() == nil {
			_ = broker.Reconnect(ctx)
		}
		select {
		case <-ctx.Done():
			return
		case <-time.After(5 * time.Second):
		}
	}
}

func consumeFileProcessTasks(ctx context.Context, broker fileapp.MessagePublisherPort, handler UploadTaskHandler) {
	for {
		if ctx.Err() != nil {
			return
		}
		_ = broker.ConsumeFileProcessTasks(ctx, "file-process-task", func(ctx context.Context, message domain.FileProcessMessage) error {
			consumeID := retryConsumeID(message.MessageID, message.Retry)
			lease, claimed, err := handler.BeginConsume(ctx, consumeID, "file-process-task", "file-process-task-consumer", "file process task")
			if err != nil || !claimed {
				return err
			}
			if err := handler.HandleFileProcessMessage(ctx, message); err != nil {
				if message.Retry < 3 {
					message.Retry++
					if publishErr := broker.PublishFileProcessTask(ctx, message); publishErr != nil {
						_, _ = handler.MarkConsumeFailed(ctx, consumeID, "file-process-task", lease.Token, publishErr.Error())
						return publishErr
					}
					marked, markErr := handler.MarkConsumed(ctx, consumeID, "file-process-task", lease.Token, "file process retry published")
					if markErr != nil {
						return markErr
					}
					if !marked {
						return fmt.Errorf("file process consume lease was lost after retry publication")
					}
					return nil
				}
				_, _ = handler.MarkConsumeFailed(ctx, consumeID, "file-process-task", lease.Token, err.Error())
				return fileapp.PermanentConsumeError(err)
			}
			_, err = handler.MarkConsumed(ctx, consumeID, "file-process-task", lease.Token, "file process task")
			return err
		})
		if ctx.Err() == nil {
			_ = broker.Reconnect(ctx)
		}
		select {
		case <-ctx.Done():
			return
		case <-time.After(5 * time.Second):
		}
	}
}

func backoff(retryCount int) time.Duration {
	if retryCount < 0 {
		retryCount = 0
	}
	if retryCount > 6 {
		retryCount = 6
	}
	return time.Duration(1<<retryCount) * time.Minute
}

func retryConsumeID(messageID string, retry int) string {
	if retry <= 0 {
		return messageID
	}
	return messageID + ":retry:" + itoa(retry)
}

func itoa(value int) string {
	return itoa64(int64(value))
}

func itoa64(value int64) string {
	if value == 0 {
		return "0"
	}
	negative := value < 0
	if negative {
		value = -value
	}
	var buf [20]byte
	pos := len(buf)
	for value > 0 {
		pos--
		buf[pos] = byte('0' + value%10)
		value /= 10
	}
	if negative {
		pos--
		buf[pos] = '-'
	}
	return string(buf[pos:])
}
