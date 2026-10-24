package infrastructure

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/CaixyPromise/seven-agent/force/internal/app/system/admin/domain"
	apperrors "github.com/CaixyPromise/seven-agent/force/internal/common/errors"
	"github.com/CaixyPromise/seven-agent/force/internal/shared/config"
)

const runtimeLogScannerBuffer = 1024 * 1024

type RuntimeLogProvider struct {
	cfg      config.AdminRuntimeLogConfig
	reader   *RuntimeLogFileReader
	parser   *RuntimeLogLineParser
	streamer *RuntimeLogTailStreamer
}

type RuntimeLogFileReader struct {
	activeFile string
}

func NewRuntimeLogProvider(cfg config.AdminRuntimeLogConfig, maskedFields []string, maxFieldLength int) *RuntimeLogProvider {
	reader := NewRuntimeLogFileReader(cfg)
	parser := NewRuntimeLogLineParser(NewRuntimeLogMaskingSupport(maskedFields, maxFieldLength))
	return &RuntimeLogProvider{
		cfg:      cfg,
		reader:   reader,
		parser:   parser,
		streamer: NewRuntimeLogTailStreamer(cfg, reader, parser),
	}
}

func NewRuntimeLogFileReader(cfg config.AdminRuntimeLogConfig) *RuntimeLogFileReader {
	return &RuntimeLogFileReader{
		activeFile: filepath.Join(strings.TrimSpace(cfg.BaseDir), strings.TrimSpace(cfg.ActiveFile)),
	}
}

func (p *RuntimeLogProvider) Page(ctx context.Context, query domain.RuntimeLogPageQuery) ([]domain.RuntimeLogLine, int64, error) {
	if p == nil || p.reader == nil || p.parser == nil {
		return nil, 0, apperrors.Operation("运行日志能力未配置")
	}
	lines, err := p.reader.ReadActiveLines(ctx)
	if err != nil {
		return nil, 0, err
	}
	if p.cfg.MaxScanLines > 0 && len(lines) > p.cfg.MaxScanLines {
		lines = lines[len(lines)-p.cfg.MaxScanLines:]
	}
	items := make([]domain.RuntimeLogLine, 0, len(lines))
	for idx, line := range lines {
		item, ok := p.parser.Parse(line, fmt.Sprintf("%d", idx+1))
		if !ok || !matchRuntimeLogLine(item, query, p.cfg.MaxSearchWindowDays) {
			continue
		}
		items = append(items, item)
	}
	sort.SliceStable(items, func(i, j int) bool {
		left := runtimeLogTime(items[i])
		right := runtimeLogTime(items[j])
		if left.Equal(right) {
			return items[i].LineID > items[j].LineID
		}
		return left.After(right)
	})
	total := int64(len(items))
	current := normalizePageValue(query.Current, 1)
	size := normalizePageValue(query.Size, 20)
	maxPageSize := int64(p.cfg.MaxPageSize)
	if maxPageSize > 0 && size > maxPageSize {
		size = maxPageSize
	}
	offset := (current - 1) * size
	if offset >= total {
		return []domain.RuntimeLogLine{}, total, nil
	}
	end := offset + size
	if end > total {
		end = total
	}
	return items[offset:end], total, nil
}

func (p *RuntimeLogProvider) Stream(ctx context.Context, query domain.RuntimeLogStreamQuery, userID int64) (io.ReadCloser, error) {
	if p == nil || p.streamer == nil {
		return nil, apperrors.Operation("运行日志能力未配置")
	}
	return p.streamer.Stream(ctx, query, userID)
}

func (r *RuntimeLogFileReader) ActiveFilePath() string {
	if r == nil {
		return ""
	}
	return r.activeFile
}

func (r *RuntimeLogFileReader) ReadActiveLines(ctx context.Context) ([]string, error) {
	if r == nil || strings.TrimSpace(r.activeFile) == "" {
		return nil, apperrors.Operation("运行日志文件未配置")
	}
	file, err := os.Open(r.activeFile)
	if err != nil {
		if os.IsNotExist(err) {
			return []string{}, nil
		}
		return nil, fmt.Errorf("open runtime log file: %w", err)
	}
	defer file.Close()

	lines := make([]string, 0, 256)
	scanner := bufio.NewScanner(file)
	scanner.Buffer(make([]byte, 0, 64*1024), runtimeLogScannerBuffer)
	for scanner.Scan() {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		default:
		}
		lines = append(lines, scanner.Text())
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("scan runtime log file: %w", err)
	}
	return lines, nil
}

func (r *RuntimeLogFileReader) ReadTailLines(ctx context.Context, lastN int) ([]string, error) {
	lines, err := r.ReadActiveLines(ctx)
	if err != nil {
		return nil, err
	}
	if lastN <= 0 || len(lines) <= lastN {
		return lines, nil
	}
	return lines[len(lines)-lastN:], nil
}

func normalizePageValue(value, fallback int64) int64 {
	if value <= 0 {
		return fallback
	}
	return value
}

func matchRuntimeLogLine(item domain.RuntimeLogLine, query domain.RuntimeLogPageQuery, maxSearchWindowDays int) bool {
	if item.LogTime != nil && maxSearchWindowDays > 0 && item.LogTime.Before(time.Now().UTC().Add(-time.Duration(maxSearchWindowDays)*24*time.Hour)) {
		return false
	}
	if query.StartTime != nil && (item.LogTime == nil || item.LogTime.Before(query.StartTime.UTC())) {
		return false
	}
	if query.EndTime != nil && (item.LogTime == nil || item.LogTime.After(query.EndTime.UTC())) {
		return false
	}
	if level := strings.TrimSpace(query.Level); level != "" && !strings.EqualFold(level, item.Level) {
		return false
	}
	if loggerName := strings.TrimSpace(query.LoggerName); loggerName != "" && !matchValue(item.LoggerName, loggerName, query.UseRegex) {
		return false
	}
	if threadName := strings.TrimSpace(query.ThreadName); threadName != "" && !matchValue(item.ThreadName, threadName, query.UseRegex) {
		return false
	}
	if traceID := strings.TrimSpace(query.TraceID); traceID != "" && strings.TrimSpace(item.TraceID) != traceID {
		return false
	}
	sourceText := runtimeLogSourceText(item.Source)
	if keyword := strings.TrimSpace(query.Keyword); keyword != "" && !matchValue(strings.Join([]string{item.Message, item.LoggerName, item.ThreadName, item.FileName, item.TraceID, sourceText}, " "), keyword, query.UseRegex) {
		return false
	}
	if contentKeyword := strings.TrimSpace(query.ContentKeyword); contentKeyword != "" && !matchValue(strings.Join([]string{item.Message, sourceText}, " "), contentKeyword, query.UseRegex) {
		return false
	}
	return true
}

func runtimeLogSourceText(source map[string]any) string {
	if len(source) == 0 {
		return ""
	}
	encoded, err := json.Marshal(source)
	if err != nil {
		return fmt.Sprint(source)
	}
	return string(encoded)
}

func matchRuntimeStreamLine(item domain.RuntimeLogLine, query domain.RuntimeLogStreamQuery, maxSearchWindowDays int) bool {
	return matchRuntimeLogLine(item, domain.RuntimeLogPageQuery{
		Keyword:        query.Keyword,
		ContentKeyword: query.ContentKeyword,
		Level:          query.Level,
		LoggerName:     query.LoggerName,
		ThreadName:     query.ThreadName,
		TraceID:        query.TraceID,
		UseRegex:       query.UseRegex,
	}, maxSearchWindowDays)
}

func matchValue(candidate, expected string, useRegex bool) bool {
	candidate = strings.TrimSpace(candidate)
	expected = strings.TrimSpace(expected)
	if expected == "" {
		return true
	}
	if useRegex {
		pattern, err := regexp.Compile(expected)
		if err != nil {
			return false
		}
		return pattern.MatchString(candidate)
	}
	return strings.Contains(strings.ToLower(candidate), strings.ToLower(expected))
}

func runtimeLogTime(item domain.RuntimeLogLine) time.Time {
	if item.LogTime != nil {
		return item.LogTime.UTC()
	}
	return time.Time{}
}
