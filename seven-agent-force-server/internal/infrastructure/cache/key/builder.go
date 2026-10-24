package key

import (
	"crypto/sha1"
	"encoding/hex"
	"fmt"
	"net/url"
	"strconv"
	"strings"
)

const defaultMaxKeyLength = 256

type CacheKey interface {
	Namespace() string
	Name() string
	Build(parts ...any) string
}

type Builder struct {
	prefix       string
	maxKeyLength int
}

func NewBuilder(prefix string) *Builder {
	trimmed := strings.Trim(strings.TrimSpace(prefix), ":")
	return &Builder{
		prefix:       trimmed,
		maxKeyLength: defaultMaxKeyLength,
	}
}

func (b *Builder) Prefix() string {
	if b == nil {
		return ""
	}
	return b.prefix
}

func (b *Builder) Build(namespace, name string, parts ...any) string {
	prefix := sanitizeSegment(b.prefix)
	namespace = sanitizeSegment(namespace)
	name = sanitizeSegment(name)

	segments := make([]string, 0, len(parts)+3)
	if prefix != "" {
		segments = append(segments, prefix)
	}
	if namespace != "" {
		segments = append(segments, namespace)
	}
	if name != "" {
		segments = append(segments, name)
	}
	for _, part := range parts {
		encoded := encodePart(part)
		if encoded != "" {
			segments = append(segments, encoded)
		}
	}

	cacheKey := joinSegments(segments)
	if cacheKey == "" {
		return ""
	}
	maxLength := b.maxKeyLength
	if maxLength <= 0 {
		maxLength = defaultMaxKeyLength
	}
	if len(cacheKey) <= maxLength {
		return cacheKey
	}

	hash := sha1.Sum([]byte(cacheKey))
	fallback := append([]string(nil), segments[:min(3, len(segments))]...)
	fallback = append(fallback, "sha1", hex.EncodeToString(hash[:]))
	return joinSegments(fallback)
}

type StaticKey struct {
	builder   *Builder
	namespace string
	name      string
}

func NewStaticKey(builder *Builder, namespace, name string) StaticKey {
	return StaticKey{
		builder:   builder,
		namespace: namespace,
		name:      name,
	}
}

func (k StaticKey) Namespace() string {
	return k.namespace
}

func (k StaticKey) Name() string {
	return k.name
}

func (k StaticKey) Build(parts ...any) string {
	if k.builder == nil {
		return ""
	}
	return k.builder.Build(k.namespace, k.name, parts...)
}

func sanitizeSegment(value string) string {
	return strings.Trim(strings.TrimSpace(value), ":")
}

func encodePart(value any) string {
	trimmed := sanitizeSegment(stringifyPart(value))
	if trimmed == "" {
		return ""
	}
	if !needsEscape(trimmed) {
		return trimmed
	}
	return url.QueryEscape(trimmed)
}

func stringifyPart(value any) string {
	switch typed := value.(type) {
	case string:
		return typed
	case []byte:
		return string(typed)
	case int:
		return strconv.Itoa(typed)
	case int8:
		return strconv.FormatInt(int64(typed), 10)
	case int16:
		return strconv.FormatInt(int64(typed), 10)
	case int32:
		return strconv.FormatInt(int64(typed), 10)
	case int64:
		return strconv.FormatInt(typed, 10)
	case uint:
		return strconv.FormatUint(uint64(typed), 10)
	case uint8:
		return strconv.FormatUint(uint64(typed), 10)
	case uint16:
		return strconv.FormatUint(uint64(typed), 10)
	case uint32:
		return strconv.FormatUint(uint64(typed), 10)
	case uint64:
		return strconv.FormatUint(typed, 10)
	case bool:
		return strconv.FormatBool(typed)
	case float32:
		return strconv.FormatFloat(float64(typed), 'f', -1, 32)
	case float64:
		return strconv.FormatFloat(typed, 'f', -1, 64)
	case nil:
		return ""
	case interface{ String() string }:
		return typed.String()
	default:
		return fmt.Sprint(value)
	}
}

func needsEscape(value string) bool {
	for i := 0; i < len(value); i++ {
		ch := value[i]
		switch {
		case ch >= 'a' && ch <= 'z':
		case ch >= 'A' && ch <= 'Z':
		case ch >= '0' && ch <= '9':
		case ch == '-', ch == '_', ch == '.', ch == '~':
		default:
			return true
		}
	}
	return false
}

func joinSegments(segments []string) string {
	if len(segments) == 0 {
		return ""
	}
	total := len(segments) - 1
	for _, segment := range segments {
		total += len(segment)
	}
	var builder strings.Builder
	builder.Grow(total)
	for idx, segment := range segments {
		if idx > 0 {
			builder.WriteByte(':')
		}
		builder.WriteString(segment)
	}
	return builder.String()
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
