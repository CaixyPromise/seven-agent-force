package docker

import (
	"bytes"
	"context"
	"io"
	"strconv"
	"strings"
	"sync"
	"time"

	apperrors "github.com/CaixyPromise/seven-agent/force/internal/common/errors"
	dockercontainer "github.com/docker/docker/api/types/container"
	"github.com/docker/docker/pkg/stdcopy"
)

const (
	dockerLogStreamDefaultTail = 200
	dockerLogStreamMaxTail     = 5000
	dockerLogStreamMaxLine     = 16 * 1024
	dockerLogStreamHeartbeat   = time.Second
	dockerLogHTTPMaxBytes      = 2 << 20
	dockerLogGrepMaxLength     = 256
)

func NormalizeContainerLogQuery(query ContainerLogQuery) (ContainerLogQuery, error) {
	if query.Tail <= 0 {
		query.Tail = dockerLogStreamDefaultTail
	}
	if query.Tail > dockerLogStreamMaxTail {
		query.Tail = dockerLogStreamMaxTail
	}
	query.Since = strings.TrimSpace(query.Since)
	query.Until = strings.TrimSpace(query.Until)
	query.Grep = strings.TrimSpace(query.Grep)
	if len([]rune(query.Grep)) > dockerLogGrepMaxLength {
		return ContainerLogQuery{}, apperrors.Params("grep 长度不能超过 256 个字符")
	}
	sinceTime, err := parseDockerLogTime(query.Since)
	if err != nil {
		return ContainerLogQuery{}, apperrors.Params("since 时间格式错误")
	}
	untilTime, err := parseDockerLogTime(query.Until)
	if err != nil {
		return ContainerLogQuery{}, apperrors.Params("until 时间格式错误")
	}
	if !sinceTime.IsZero() && !untilTime.IsZero() && untilTime.Before(sinceTime) {
		return ContainerLogQuery{}, apperrors.Params("until 不能早于 since")
	}
	return query, nil
}

func (s *service) GetContainerLogsQuery(ctx context.Context, id string, query ContainerLogQuery) (string, error) {
	query, err := NormalizeContainerLogQuery(query)
	if err != nil {
		return "", err
	}
	cli, resolved, _, err := s.resolveContainer(ctx, id)
	if err != nil {
		return "", err
	}
	inspectCtx, inspectCancel := s.withTimeout(ctx)
	inspect, err := cli.ContainerInspect(inspectCtx, resolved)
	inspectCancel()
	if err != nil {
		return "", apperrors.Operation("获取 Docker 容器详情失败：" + err.Error())
	}
	runCtx, cancel := s.withTimeout(ctx)
	defer cancel()
	reader, err := cli.ContainerLogs(runCtx, resolved, buildContainerLogOptions(query, false))
	if err != nil {
		return "", apperrors.Operation("读取容器日志失败：" + err.Error())
	}
	defer reader.Close()
	limited := io.LimitReader(reader, dockerLogHTTPMaxBytes+1)
	var buffer bytes.Buffer
	stdout := &dockerPlainLogWriter{service: s, writer: &buffer, grep: query.Grep}
	stderr := &dockerPlainLogWriter{service: s, writer: &buffer, grep: query.Grep}
	if inspect.Config != nil && inspect.Config.Tty {
		_, err = io.Copy(stdout, limited)
	} else {
		_, err = stdcopy.StdCopy(stdout, stderr, limited)
	}
	if err != nil {
		return "", apperrors.Operation("读取容器日志失败：" + err.Error())
	}
	if err := stdout.Flush(); err != nil {
		return "", apperrors.Operation("读取容器日志失败：" + err.Error())
	}
	if err := stderr.Flush(); err != nil {
		return "", apperrors.Operation("读取容器日志失败：" + err.Error())
	}
	body := buffer.Bytes()
	if len(body) > dockerLogHTTPMaxBytes {
		body = append(body[:dockerLogHTTPMaxBytes], []byte("...(truncated)")...)
	}
	return string(body), nil
}

func (s *service) StreamContainerLogs(ctx context.Context, id string, query ContainerLogQuery) (io.ReadCloser, error) {
	query, err := NormalizeContainerLogQuery(query)
	if err != nil {
		return nil, err
	}
	cli, resolved, _, err := s.resolveContainer(ctx, id)
	if err != nil {
		return nil, err
	}
	inspectCtx, inspectCancel := s.withTimeout(ctx)
	inspect, err := cli.ContainerInspect(inspectCtx, resolved)
	inspectCancel()
	if err != nil {
		return nil, apperrors.Operation("获取 Docker 容器详情失败：" + err.Error())
	}
	tty := inspect.Config != nil && inspect.Config.Tty
	streamCtx, cancel := context.WithCancel(ctx)
	reader, err := cli.ContainerLogs(streamCtx, resolved, dockercontainer.LogsOptions{
		ShowStdout: true,
		ShowStderr: true,
		Tail:       strconv.Itoa(query.Tail),
		Since:      query.Since,
		Until:      query.Until,
		Timestamps: query.Timestamps,
		Follow:     query.Follow,
	})
	if err != nil {
		cancel()
		return nil, apperrors.Operation("读取容器日志失败：" + err.Error())
	}

	pipeReader, pipeWriter := io.Pipe()
	go s.pipeContainerLogs(streamCtx, cancel, reader, pipeWriter, tty, query.Grep)
	return pipeReader, nil
}

func (s *service) pipeContainerLogs(ctx context.Context, cancel context.CancelFunc, reader io.ReadCloser, writer *io.PipeWriter, tty bool, grep string) {
	defer cancel()
	defer reader.Close()
	defer writer.Close()

	var writeMu sync.Mutex
	stdout := &dockerLogSSEWriter{stream: "stdout", service: s, writer: writer, writeMu: &writeMu, grep: grep}
	stderr := &dockerLogSSEWriter{stream: "stderr", service: s, writer: writer, writeMu: &writeMu, grep: grep}
	done := make(chan error, 1)
	go func() {
		var err error
		if tty {
			_, err = io.Copy(stdout, reader)
		} else {
			_, err = stdcopy.StdCopy(stdout, stderr, reader)
		}
		done <- err
	}()

	ticker := time.NewTicker(dockerLogStreamHeartbeat)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case err := <-done:
			_ = stdout.Flush()
			_ = stderr.Flush()
			writeMu.Lock()
			if err != nil && ctx.Err() == nil {
				_ = writeDockerSSEEvent(writer, "error", map[string]any{"message": s.maskOperationMessage(err.Error())})
			} else {
				_ = writeDockerSSEEvent(writer, "done", map[string]any{"reason": "closed"})
			}
			writeMu.Unlock()
			return
		case <-ticker.C:
			writeMu.Lock()
			_ = stdout.flushLocked()
			_ = stderr.flushLocked()
			if err := writeDockerSSEEvent(writer, "heartbeat", map[string]any{"ts": time.Now().UTC().Format(time.RFC3339Nano)}); err != nil {
				writeMu.Unlock()
				return
			}
			writeMu.Unlock()
		}
	}
}

type dockerLogSSEWriter struct {
	stream  string
	service *service
	writer  *io.PipeWriter
	writeMu *sync.Mutex
	grep    string
	buffer  []byte
}

func (w *dockerLogSSEWriter) Write(p []byte) (int, error) {
	w.writeMu.Lock()
	defer w.writeMu.Unlock()
	w.buffer = append(w.buffer, p...)
	for {
		index := bytes.IndexByte(w.buffer, '\n')
		if index < 0 {
			break
		}
		line := string(w.buffer[:index])
		w.buffer = w.buffer[index+1:]
		if err := w.emitLineLocked(line); err != nil {
			return 0, err
		}
	}
	if len(w.buffer) > dockerLogStreamMaxLine {
		line := string(w.buffer[:dockerLogStreamMaxLine]) + "...(truncated)"
		w.buffer = w.buffer[:0]
		if err := w.emitLineLocked(line); err != nil {
			return 0, err
		}
	}
	return len(p), nil
}

func (w *dockerLogSSEWriter) Flush() error {
	w.writeMu.Lock()
	defer w.writeMu.Unlock()
	return w.flushLocked()
}

func (w *dockerLogSSEWriter) flushLocked() error {
	if len(w.buffer) == 0 {
		return nil
	}
	line := string(w.buffer)
	w.buffer = w.buffer[:0]
	return w.emitLineLocked(line)
}

func (w *dockerLogSSEWriter) emitLineLocked(line string) error {
	line = strings.TrimRight(line, "\r")
	if strings.TrimSpace(line) == "" {
		return nil
	}
	if w.grep != "" && !strings.Contains(line, w.grep) {
		return nil
	}
	return writeDockerSSEEvent(w.writer, "log", map[string]any{
		"line":   truncate(w.service.maskOperationMessage(line), dockerLogStreamMaxLine),
		"stream": w.stream,
	})
}

type dockerPlainLogWriter struct {
	service *service
	writer  io.Writer
	grep    string
	buffer  []byte
}

func (w *dockerPlainLogWriter) Write(p []byte) (int, error) {
	w.buffer = append(w.buffer, p...)
	for {
		index := bytes.IndexByte(w.buffer, '\n')
		if index < 0 {
			break
		}
		line := string(w.buffer[:index])
		w.buffer = w.buffer[index+1:]
		if err := w.emit(line); err != nil {
			return 0, err
		}
	}
	if len(w.buffer) > dockerLogStreamMaxLine {
		line := string(w.buffer[:dockerLogStreamMaxLine]) + "...(truncated)"
		w.buffer = w.buffer[:0]
		if err := w.emit(line); err != nil {
			return 0, err
		}
	}
	return len(p), nil
}

func (w *dockerPlainLogWriter) Flush() error {
	if len(w.buffer) == 0 {
		return nil
	}
	line := string(w.buffer)
	w.buffer = w.buffer[:0]
	return w.emit(line)
}

func (w *dockerPlainLogWriter) emit(line string) error {
	line = strings.TrimRight(line, "\r")
	if strings.TrimSpace(line) == "" {
		return nil
	}
	if w.grep != "" && !strings.Contains(line, w.grep) {
		return nil
	}
	_, err := w.writer.Write([]byte(truncate(w.service.maskOperationMessage(line), dockerLogStreamMaxLine) + "\n"))
	return err
}

func buildContainerLogOptions(query ContainerLogQuery, follow bool) dockercontainer.LogsOptions {
	return dockercontainer.LogsOptions{
		ShowStdout: true,
		ShowStderr: true,
		Tail:       strconv.Itoa(query.Tail),
		Since:      query.Since,
		Until:      query.Until,
		Timestamps: query.Timestamps,
		Follow:     follow,
	}
}

func parseDockerLogTime(value string) (time.Time, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return time.Time{}, nil
	}
	if seconds, err := strconv.ParseInt(value, 10, 64); err == nil {
		return time.Unix(seconds, 0), nil
	}
	for _, layout := range []string{time.RFC3339Nano, time.RFC3339, "2006-01-02 15:04:05", "2006-01-02"} {
		if parsed, err := time.Parse(layout, value); err == nil {
			return parsed, nil
		}
	}
	return time.Time{}, apperrors.Params("时间格式错误")
}
