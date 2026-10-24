package infrastructure

import (
	"context"
	"log"

	"github.com/CaixyPromise/seven-agent/force/internal/app/system/admin/domain"
)

const asyncOperationLogQueueSize = 256

type AsyncOperationLogWriter struct {
	repository domain.OperationLogRepository
	queue      chan domain.OperationLog
}

func NewAsyncOperationLogWriter(repository domain.OperationLogRepository) *AsyncOperationLogWriter {
	if repository == nil {
		return nil
	}
	writer := &AsyncOperationLogWriter{
		repository: repository,
		queue:      make(chan domain.OperationLog, asyncOperationLogQueueSize),
	}
	go writer.run()
	return writer
}

func (w *AsyncOperationLogWriter) Enqueue(_ context.Context, item domain.OperationLog) {
	if w == nil || w.repository == nil {
		return
	}
	select {
	case w.queue <- item:
	default:
		go w.save(item)
	}
}

func (w *AsyncOperationLogWriter) run() {
	for item := range w.queue {
		w.save(item)
	}
}

func (w *AsyncOperationLogWriter) save(item domain.OperationLog) {
	if w == nil || w.repository == nil {
		return
	}
	if _, err := w.repository.InsertOperationLog(context.Background(), &item); err != nil {
		log.Printf("system-admin operation log async save failed: %v", err)
	}
}
