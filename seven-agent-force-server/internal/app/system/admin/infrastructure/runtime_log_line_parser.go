package infrastructure

import (
	stdjson "encoding/json"
	"fmt"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/CaixyPromise/seven-agent/force/internal/app/system/admin/domain"
	jsoncompat "github.com/CaixyPromise/seven-agent/force/internal/common/json"
)

var javaTextLogPattern = regexp.MustCompile(`^(\d{4}-\d{2}-\d{2}[ T]\d{2}:\d{2}:\d{2}(?:[.,]\d{3})?)\s+([A-Z]+)\s+\[([^\]]*)\]\s+(.+?)\s+-\s+(.*)$`)

type RuntimeLogLineParser struct {
	masking *RuntimeLogMaskingSupport
}

func NewRuntimeLogLineParser(masking *RuntimeLogMaskingSupport) *RuntimeLogLineParser {
	return &RuntimeLogLineParser{masking: masking}
}

func (p *RuntimeLogLineParser) Parse(line string, lineID string) (domain.RuntimeLogLine, bool) {
	trimmed := strings.TrimSpace(line)
	if trimmed == "" {
		return domain.RuntimeLogLine{}, false
	}
	if item, ok := p.parseJSONLine(trimmed, lineID); ok {
		return item, true
	}
	if item, ok := p.parseTextLine(trimmed, lineID); ok {
		return item, true
	}
	return domain.RuntimeLogLine{
		LineID:  lineID,
		Message: p.mask(trimmed),
	}, true
}

func (p *RuntimeLogLineParser) parseJSONLine(line string, lineID string) (domain.RuntimeLogLine, bool) {
	var payload map[string]any
	if err := stdjson.Unmarshal([]byte(line), &payload); err != nil {
		return domain.RuntimeLogLine{}, false
	}
	item := domain.RuntimeLogLine{
		LineID:     lineID,
		LogTime:    parseAnyTime(payload["timestamp"], payload["@timestamp"], payload["time"], payload["ts"]),
		Level:      strings.ToUpper(strings.TrimSpace(anyString(payload["level"]))),
		ThreadName: strings.TrimSpace(anyString(payload["threadName"])),
		LoggerName: strings.TrimSpace(anyString(payload["loggerName"])),
		TraceID:    strings.TrimSpace(firstNonEmpty(anyString(payload["traceId"]), anyString(payload["trace_id"]), anyString(payload["trace-id"]))),
		Message:    p.mask(strings.TrimSpace(firstNonEmpty(anyString(payload["message"]), anyString(payload["msg"])))),
		Source:     p.maskSource(runtimeLogSourceFields(payload)),
	}
	if item.ThreadName == "" {
		item.ThreadName = strings.TrimSpace(anyString(payload["thread"]))
	}
	caller := strings.TrimSpace(firstNonEmpty(anyString(payload["caller"]), anyString(payload["source"])))
	if item.LoggerName == "" {
		item.LoggerName = caller
	}
	if caller != "" {
		item.FileName, item.LineNumber = parseCaller(caller)
	}
	if item.Message == "" {
		item.Message = p.mask(line)
	}
	if item.Level == "" {
		item.Level = "INFO"
	}
	return item, true
}

func (p *RuntimeLogLineParser) parseTextLine(line string, lineID string) (domain.RuntimeLogLine, bool) {
	matches := javaTextLogPattern.FindStringSubmatch(line)
	if len(matches) != 6 {
		return domain.RuntimeLogLine{}, false
	}
	return domain.RuntimeLogLine{
		LineID:     lineID,
		LogTime:    parseTimeValue(matches[1]),
		Level:      strings.ToUpper(strings.TrimSpace(matches[2])),
		ThreadName: strings.TrimSpace(matches[3]),
		LoggerName: strings.TrimSpace(matches[4]),
		Message:    p.mask(strings.TrimSpace(matches[5])),
	}, true
}

func (p *RuntimeLogLineParser) mask(value string) string {
	if p == nil || p.masking == nil {
		return value
	}
	return p.masking.MaskMessage(value)
}

func (p *RuntimeLogLineParser) maskSource(source map[string]any) map[string]any {
	if len(source) == 0 {
		return nil
	}
	masked, ok := jsoncompat.MaskSensitiveFields(source, p.maskedFields(), p.maxFieldLength()).(map[string]any)
	if ok {
		return masked
	}
	return map[string]any{"raw": p.mask(fmt.Sprint(source))}
}

func (p *RuntimeLogLineParser) maskedFields() []string {
	if p == nil || p.masking == nil {
		return nil
	}
	return p.masking.maskedFields
}

func (p *RuntimeLogLineParser) maxFieldLength() int {
	if p == nil || p.masking == nil {
		return 0
	}
	return p.masking.maxFieldLength
}

func runtimeLogSourceFields(payload map[string]any) map[string]any {
	source := make(map[string]any, len(payload))
	for key, value := range payload {
		if isRuntimeLogMetadataKey(key) {
			continue
		}
		source[key] = value
	}
	if len(source) == 0 {
		return nil
	}
	return source
}

func isRuntimeLogMetadataKey(key string) bool {
	normalized := strings.ToLower(strings.TrimSpace(strings.ReplaceAll(key, "-", "_")))
	switch normalized {
	case "timestamp", "@timestamp", "time", "ts",
		"level", "threadname", "thread_name", "thread",
		"loggername", "logger_name",
		"traceid", "trace_id",
		"message", "msg",
		"caller", "source":
		return true
	default:
		return false
	}
}

func parseAnyTime(values ...any) *time.Time {
	for _, value := range values {
		switch typed := value.(type) {
		case string:
			if parsed := parseTimeValue(typed); parsed != nil {
				return parsed
			}
		case float64:
			millis := int64(typed)
			if millis > 0 {
				result := time.UnixMilli(millis).UTC()
				return &result
			}
		}
	}
	return nil
}

func parseTimeValue(value string) *time.Time {
	raw := strings.TrimSpace(strings.ReplaceAll(value, ",", "."))
	if raw == "" {
		return nil
	}
	layouts := []string{
		time.RFC3339Nano,
		time.RFC3339,
		"2006-01-02T15:04:05.000-0700",
		"2006-01-02T15:04:05-0700",
		"2006-01-02 15:04:05.000",
		"2006-01-02 15:04:05",
		"2006-01-02T15:04:05.000",
		"2006-01-02T15:04:05",
	}
	for _, layout := range layouts {
		if parsed, err := time.ParseInLocation(layout, raw, time.Local); err == nil {
			result := parsed.UTC()
			return &result
		}
	}
	return nil
}

func anyString(value any) string {
	switch typed := value.(type) {
	case string:
		return typed
	case nil:
		return ""
	default:
		return fmt.Sprint(typed)
	}
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}

func parseCaller(value string) (string, int) {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return "", 0
	}
	lastColon := strings.LastIndex(trimmed, ":")
	if lastColon < 0 || lastColon == len(trimmed)-1 {
		return filepath.Base(trimmed), 0
	}
	lineNumber, err := strconv.Atoi(trimmed[lastColon+1:])
	if err != nil {
		return filepath.Base(trimmed), 0
	}
	return filepath.Base(trimmed[:lastColon]), lineNumber
}
