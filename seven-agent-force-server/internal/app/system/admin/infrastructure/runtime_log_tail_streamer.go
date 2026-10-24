package infrastructure

import (
	"context"
	stdjson "encoding/json"
	"fmt"
	"io"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/CaixyPromise/seven-agent/force/internal/app/system/admin/domain"
	adminfacade "github.com/CaixyPromise/seven-agent/force/internal/app/system/admin/facade"
	apperrors "github.com/CaixyPromise/seven-agent/force/internal/common/errors"
	"github.com/CaixyPromise/seven-agent/force/internal/shared/config"
)

type RuntimeLogTailStreamer struct {
	cfg    config.AdminRuntimeLogConfig
	reader *RuntimeLogFileReader
	parser *RuntimeLogLineParser

	mu             sync.Mutex
	globalCount    int
	perUserCounter map[int64]int
}

func NewRuntimeLogTailStreamer(cfg config.AdminRuntimeLogConfig, reader *RuntimeLogFileReader, parser *RuntimeLogLineParser) *RuntimeLogTailStreamer {
	return &RuntimeLogTailStreamer{
		cfg:            cfg,
		reader:         reader,
		parser:         parser,
		perUserCounter: make(map[int64]int),
	}
}

func (s *RuntimeLogTailStreamer) Stream(ctx context.Context, query domain.RuntimeLogStreamQuery, userID int64) (io.ReadCloser, error) {
	if s == nil || s.reader == nil || s.parser == nil {
		return nil, apperrors.Operation("运行日志流能力未配置")
	}
	if err := s.acquireConnection(userID); err != nil {
		return nil, err
	}
	pr, pw := io.Pipe()
	go s.streamLoop(ctx, query, userID, pw)
	return pr, nil
}

func (s *RuntimeLogTailStreamer) streamLoop(ctx context.Context, query domain.RuntimeLogStreamQuery, userID int64, writer *io.PipeWriter) {
	defer s.releaseConnection(userID)
	defer writer.Close()

	streamCtx := ctx
	var cancel context.CancelFunc
	if timeout := s.cfg.EmitterTimeoutMs; timeout > 0 {
		streamCtx, cancel = context.WithTimeout(ctx, time.Duration(timeout)*time.Millisecond)
		defer cancel()
	}

	lastN := query.LastN
	if lastN <= 0 {
		lastN = s.cfg.DefaultLastN
	}
	if lastN <= 0 {
		lastN = 100
	}
	if s.cfg.MaxLastN > 0 && lastN > s.cfg.MaxLastN {
		lastN = s.cfg.MaxLastN
	}
	if err := s.writeWarmup(streamCtx, query, writer, lastN); err != nil {
		return
	}

	offset := s.initialOffset()
	pollTicker := time.NewTicker(time.Duration(maxInt64(s.cfg.TailPollIntervalMs, 1000)) * time.Millisecond)
	defer pollTicker.Stop()
	heartbeatTicker := time.NewTicker(time.Duration(maxInt64(s.cfg.HeartbeatIntervalMs, 5000)) * time.Millisecond)
	defer heartbeatTicker.Stop()

	var partial string
	for {
		select {
		case <-streamCtx.Done():
			return
		case <-heartbeatTicker.C:
			if err := writeSSEEvent(writer, "heartbeat", map[string]any{"ts": time.Now().UTC().Format(time.RFC3339Nano)}); err != nil {
				return
			}
		case <-pollTicker.C:
			chunk, nextOffset, err := s.readChunk(offset)
			if err != nil {
				if os.IsNotExist(err) {
					offset = 0
					continue
				}
				_ = writer.CloseWithError(err)
				return
			}
			offset = nextOffset
			if len(chunk) == 0 {
				continue
			}
			text := partial + string(chunk)
			lines := strings.Split(text, "\n")
			partial = lines[len(lines)-1]
			for idx := 0; idx < len(lines)-1; idx++ {
				item, ok := s.parser.Parse(lines[idx], fmt.Sprintf("%d-%d", time.Now().UTC().UnixMilli(), idx))
				if !ok || !matchRuntimeStreamLine(item, query, s.cfg.MaxSearchWindowDays) {
					continue
				}
				if err := writeSSEEvent(writer, "log", adminfacade.RuntimeLogLineDTO(item)); err != nil {
					return
				}
			}
		}
	}
}

func (s *RuntimeLogTailStreamer) writeWarmup(ctx context.Context, query domain.RuntimeLogStreamQuery, writer *io.PipeWriter, lastN int) error {
	lines, err := s.reader.ReadTailLines(ctx, maxInt(lastN*4, lastN))
	if err != nil {
		return err
	}
	items := make([]domain.RuntimeLogLine, 0, len(lines))
	for idx, line := range lines {
		item, ok := s.parser.Parse(line, fmt.Sprintf("warmup-%d", idx))
		if !ok || !matchRuntimeStreamLine(item, query, s.cfg.MaxSearchWindowDays) {
			continue
		}
		items = append(items, item)
	}
	if len(items) > lastN {
		items = items[len(items)-lastN:]
	}
	for _, item := range items {
		if err := writeSSEEvent(writer, "log", adminfacade.RuntimeLogLineDTO(item)); err != nil {
			return err
		}
	}
	return nil
}

func (s *RuntimeLogTailStreamer) acquireConnection(userID int64) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.cfg.MaxGlobalConnections > 0 && s.globalCount >= s.cfg.MaxGlobalConnections {
		return apperrors.Forbidden("运行日志订阅连接数已达上限")
	}
	if s.cfg.MaxConnectionsPerUser > 0 && userID > 0 && s.perUserCounter[userID] >= s.cfg.MaxConnectionsPerUser {
		return apperrors.Forbidden("当前用户运行日志订阅连接数已达上限")
	}
	s.globalCount++
	if userID > 0 {
		s.perUserCounter[userID]++
	}
	return nil
}

func (s *RuntimeLogTailStreamer) releaseConnection(userID int64) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.globalCount > 0 {
		s.globalCount--
	}
	if userID > 0 {
		if count := s.perUserCounter[userID]; count <= 1 {
			delete(s.perUserCounter, userID)
		} else {
			s.perUserCounter[userID] = count - 1
		}
	}
}

func (s *RuntimeLogTailStreamer) initialOffset() int64 {
	path := s.reader.ActiveFilePath()
	if strings.TrimSpace(path) == "" {
		return 0
	}
	info, err := os.Stat(path)
	if err != nil {
		return 0
	}
	return info.Size()
}

func (s *RuntimeLogTailStreamer) readChunk(offset int64) ([]byte, int64, error) {
	path := s.reader.ActiveFilePath()
	file, err := os.Open(path)
	if err != nil {
		return nil, offset, err
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil {
		return nil, offset, err
	}
	if info.Size() < offset {
		offset = 0
	}
	if _, err := file.Seek(offset, io.SeekStart); err != nil {
		return nil, offset, err
	}
	data, err := io.ReadAll(file)
	if err != nil {
		return nil, offset, err
	}
	return data, info.Size(), nil
}

func writeSSEEvent(writer *io.PipeWriter, event string, payload any) error {
	data, err := stdjson.Marshal(payload)
	if err != nil {
		return err
	}
	_, err = writer.Write([]byte("event: " + strings.TrimSpace(event) + "\n" + "data: " + string(data) + "\n\n"))
	return err
}

func maxInt(value, fallback int) int {
	if value > 0 {
		return value
	}
	return fallback
}

func maxInt64(value, fallback int64) int64 {
	if value > 0 {
		return value
	}
	return fallback
}
