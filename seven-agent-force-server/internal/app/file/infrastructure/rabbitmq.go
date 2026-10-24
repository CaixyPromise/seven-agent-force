package infrastructure

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strings"
	"time"

	"github.com/CaixyPromise/seven-agent/force/internal/app/file/domain"
	rabbitinfra "github.com/CaixyPromise/seven-agent/force/internal/infrastructure/messaging/rabbitmq"
)

const (
	FileProcessExchange   = "file.process.exchange"
	FileProcessQueue      = "file.process.queue"
	FileProcessRoutingKey = "file.process"
	FileProcessDLX        = "file.process.dlx"
	FileProcessDLQ        = "file.process.dlq"
	deadLetterRoutingKey  = "dlq"

	UploadTaskExchange        = "upload.task.exchange"
	UploadTaskQueue           = "upload.task.queue"
	UploadTaskRoutingKey      = "upload.task"
	UploadTaskDLX             = "upload.task.dlx"
	UploadTaskDLQ             = "upload.task.dlq"
	UploadTaskRetryExchange   = "upload.task.retry.exchange"
	UploadTaskRetryQueue      = "upload.task.retry.queue"
	UploadTaskRetryRoutingKey = "upload.task.retry"
)

type RabbitMQ struct {
	client *rabbitinfra.Client
}

func NewRabbitMQ(client *rabbitinfra.Client, declare bool) (*RabbitMQ, error) {
	if client == nil || !client.Enabled() {
		return nil, fmt.Errorf("rabbitmq client is not available")
	}
	adapter := &RabbitMQ{client: client}
	if declare {
		if err := adapter.Declare(); err != nil {
			return nil, err
		}
	}
	return adapter, nil
}

func NewDisabledRabbitMQ() *RabbitMQ {
	return &RabbitMQ{client: rabbitinfra.Disabled()}
}

func (r *RabbitMQ) Enabled() bool {
	return r != nil && r.client != nil && r.client.Enabled()
}

func (r *RabbitMQ) Reconnect(ctx context.Context) error {
	if r == nil || r.client == nil {
		return nil
	}
	return r.client.Reconnect(ctx)
}

func (r *RabbitMQ) Declare() error {
	if !r.Enabled() {
		return nil
	}
	if err := r.client.DeclareDirectExchange(FileProcessExchange); err != nil {
		return err
	}
	if err := r.client.DeclareDirectExchange(FileProcessDLX); err != nil {
		return err
	}
	if err := r.client.DeclareQueue(FileProcessQueue, rabbitinfra.QueueOptions{DeadLetterExchange: FileProcessDLX, DeadLetterRoutingKey: deadLetterRoutingKey}, FileProcessExchange, FileProcessRoutingKey); err != nil {
		return err
	}
	if err := r.client.DeclareQueue(FileProcessDLQ, rabbitinfra.QueueOptions{}, FileProcessDLX, deadLetterRoutingKey); err != nil {
		return err
	}
	if err := r.client.DeclareDirectExchange(UploadTaskExchange); err != nil {
		return err
	}
	if err := r.client.DeclareDirectExchange(UploadTaskDLX); err != nil {
		return err
	}
	if err := r.client.DeclareDirectExchange(UploadTaskRetryExchange); err != nil {
		return err
	}
	if err := r.client.DeclareQueue(UploadTaskQueue, rabbitinfra.QueueOptions{DeadLetterExchange: UploadTaskDLX, DeadLetterRoutingKey: deadLetterRoutingKey}, UploadTaskExchange, UploadTaskRoutingKey); err != nil {
		return err
	}
	if err := r.client.DeclareQueue(UploadTaskDLQ, rabbitinfra.QueueOptions{}, UploadTaskDLX, deadLetterRoutingKey); err != nil {
		return err
	}
	return r.client.DeclareQueue(UploadTaskRetryQueue, rabbitinfra.QueueOptions{DeadLetterExchange: UploadTaskExchange, DeadLetterRoutingKey: UploadTaskRoutingKey}, UploadTaskRetryExchange, UploadTaskRetryRoutingKey)
}

func (r *RabbitMQ) PublishUploadTask(ctx context.Context, message domain.UploadTaskMessage) error {
	if !r.Enabled() {
		return nil
	}
	return r.client.PublishJSON(ctx, UploadTaskExchange, UploadTaskRoutingKey, message.MessageID, message, 0)
}

func (r *RabbitMQ) PublishUploadTaskRetry(ctx context.Context, message domain.UploadTaskMessage, delay time.Duration) error {
	if !r.Enabled() {
		return nil
	}
	return r.client.PublishJSON(ctx, UploadTaskRetryExchange, UploadTaskRetryRoutingKey, message.MessageID, message, delay)
}

func (r *RabbitMQ) PublishFileProcessTask(ctx context.Context, message domain.FileProcessMessage) error {
	if !r.Enabled() {
		return nil
	}
	return r.client.PublishJSON(ctx, FileProcessExchange, FileProcessRoutingKey, message.MessageID, message, 0)
}

func (r *RabbitMQ) ConsumeUploadTasks(ctx context.Context, consumer string, handler func(context.Context, domain.UploadTaskMessage) error) error {
	return rabbitinfra.ConsumeJSON(ctx, r.client, UploadTaskQueue, consumer, func(ctx context.Context, delivery rabbitinfra.Delivery[domain.UploadTaskMessage]) error {
		message := delivery.Message
		message.MessageID = normalizeDeliveryMessageID(message.MessageID, delivery.MessageID, delivery.Body)
		return handler(ctx, message)
	})
}

func (r *RabbitMQ) ConsumeFileProcessTasks(ctx context.Context, consumer string, handler func(context.Context, domain.FileProcessMessage) error) error {
	return rabbitinfra.ConsumeJSON(ctx, r.client, FileProcessQueue, consumer, func(ctx context.Context, delivery rabbitinfra.Delivery[domain.FileProcessMessage]) error {
		message := delivery.Message
		message.MessageID = normalizeDeliveryMessageID(message.MessageID, delivery.MessageID, delivery.Body)
		return handler(ctx, message)
	})
}

func normalizeDeliveryMessageID(bodyMessageID, amqpMessageID string, body []byte) string {
	if value := strings.TrimSpace(bodyMessageID); value != "" {
		return value
	}
	if value := strings.TrimSpace(amqpMessageID); value != "" {
		return value
	}
	sum := sha256.Sum256(body)
	return "body-sha256:" + hex.EncodeToString(sum[:])
}
