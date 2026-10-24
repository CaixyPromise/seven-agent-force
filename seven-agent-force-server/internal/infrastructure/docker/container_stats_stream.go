package docker

import (
	"context"
	"encoding/json"
	"io"
	"time"

	apperrors "github.com/CaixyPromise/seven-agent/force/internal/common/errors"
)

const dockerStatsStreamHeartbeat = 5 * time.Second

func (s *service) StreamContainerStats(ctx context.Context, id string) (io.ReadCloser, error) {
	cli, resolved, _, err := s.resolveContainer(ctx, id)
	if err != nil {
		return nil, err
	}
	streamCtx, cancel := context.WithCancel(ctx)
	result, err := cli.ContainerStats(streamCtx, resolved, true)
	if err != nil {
		cancel()
		return nil, apperrors.Operation("读取 Docker 容器资源统计流失败：" + err.Error())
	}
	pipeReader, pipeWriter := io.Pipe()
	go s.pipeContainerStats(streamCtx, cancel, result.Body, pipeWriter)
	return pipeReader, nil
}

func (s *service) pipeContainerStats(ctx context.Context, cancel context.CancelFunc, reader io.ReadCloser, writer *io.PipeWriter) {
	defer cancel()
	defer reader.Close()
	defer writer.Close()

	type statsDecodeResult struct {
		payload map[string]any
		err     error
	}
	results := make(chan statsDecodeResult, 1)
	go func() {
		decoder := json.NewDecoder(reader)
		for {
			var payload map[string]any
			if err := decoder.Decode(&payload); err != nil {
				results <- statsDecodeResult{err: err}
				return
			}
			select {
			case results <- statsDecodeResult{payload: payload}:
			case <-ctx.Done():
				return
			}
		}
	}()

	ticker := time.NewTicker(dockerStatsStreamHeartbeat)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case result := <-results:
			if result.err != nil {
				if result.err == io.EOF || ctx.Err() != nil {
					_ = writeDockerSSEEvent(writer, "done", map[string]any{"reason": "closed"})
					return
				}
				_ = writeDockerSSEEvent(writer, "error", map[string]any{"message": s.maskOperationMessage(result.err.Error())})
				return
			}
			if err := writeDockerSSEEvent(writer, "stats", sanitizeMap(result.payload, s.cfg.Security)); err != nil {
				return
			}
		case <-ticker.C:
			if err := writeDockerSSEEvent(writer, "heartbeat", map[string]any{"ts": time.Now().UTC().Format(time.RFC3339Nano)}); err != nil {
				return
			}
		}
	}
}
