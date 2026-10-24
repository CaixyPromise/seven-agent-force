package cache

import (
	"fmt"
	"reflect"
	"strconv"
	"strings"
	"time"

	jsoncompat "github.com/CaixyPromise/seven-agent/force/internal/common/json"
	"github.com/bytedance/sonic"
	"github.com/go-viper/mapstructure/v2"
)

type Codec interface {
	Name() string
	Marshal(value any) ([]byte, error)
	Unmarshal(payload []byte, dest any) error
}

type sonicCodec struct{}

var timeType = reflect.TypeOf(time.Time{})

func NewCodec(name string) (Codec, error) {
	switch strings.ToLower(strings.TrimSpace(name)) {
	case "", "sonic":
		return sonicCodec{}, nil
	default:
		return nil, fmt.Errorf("unsupported cache codec: %s", name)
	}
}

func (sonicCodec) Name() string {
	return "sonic"
}

func (sonicCodec) Marshal(value any) ([]byte, error) {
	return sonic.Marshal(jsoncompat.NormalizeForJSON(value))
}

func (sonicCodec) Unmarshal(payload []byte, dest any) error {
	if len(payload) == 0 {
		return nil
	}
	if err := sonic.Unmarshal(payload, dest); err == nil {
		return nil
	}

	var raw any
	if err := sonic.Unmarshal(payload, &raw); err != nil {
		return err
	}

	decoder, err := mapstructure.NewDecoder(&mapstructure.DecoderConfig{
		TagName: "json",
		Result:  dest,
		DecodeHook: mapstructure.ComposeDecodeHookFunc(
			stringToSignedHook,
			stringToUnsignedHook,
			stringToFloatHook,
			stringToTimeHook,
		),
		MatchName: func(mapKey, fieldName string) bool {
			return strings.EqualFold(mapKey, fieldName)
		},
	})
	if err != nil {
		return err
	}
	return decoder.Decode(raw)
}

func stringToSignedHook(from, to reflect.Type, value any) (any, error) {
	if from.Kind() != reflect.String {
		return value, nil
	}
	switch to.Kind() {
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		parsed, err := strconv.ParseInt(value.(string), 10, 64)
		if err != nil {
			return value, nil
		}
		return parsed, nil
	default:
		return value, nil
	}
}

func stringToUnsignedHook(from, to reflect.Type, value any) (any, error) {
	if from.Kind() != reflect.String {
		return value, nil
	}
	switch to.Kind() {
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		parsed, err := strconv.ParseUint(value.(string), 10, 64)
		if err != nil {
			return value, nil
		}
		return parsed, nil
	default:
		return value, nil
	}
}

func stringToFloatHook(from, to reflect.Type, value any) (any, error) {
	if from.Kind() != reflect.String {
		return value, nil
	}
	switch to.Kind() {
	case reflect.Float32, reflect.Float64:
		parsed, err := strconv.ParseFloat(value.(string), 64)
		if err != nil {
			return value, nil
		}
		return parsed, nil
	default:
		return value, nil
	}
}

func stringToTimeHook(from, to reflect.Type, value any) (any, error) {
	if from.Kind() != reflect.String {
		return value, nil
	}
	raw := strings.TrimSpace(value.(string))
	if raw == "" {
		if to == timeType {
			return time.Time{}, nil
		}
		if to.Kind() == reflect.Ptr && to.Elem() == timeType {
			return (*time.Time)(nil), nil
		}
		return value, nil
	}
	parsed, err := parseCacheTime(raw)
	if err != nil {
		return value, nil
	}
	switch {
	case to == timeType:
		return parsed, nil
	case to.Kind() == reflect.Ptr && to.Elem() == timeType:
		return &parsed, nil
	default:
		return value, nil
	}
}

func parseCacheTime(raw string) (time.Time, error) {
	layouts := []string{
		time.RFC3339Nano,
		time.RFC3339,
		"2006-01-02 15:04:05",
	}
	var lastErr error
	for _, layout := range layouts {
		parsed, err := time.Parse(layout, raw)
		if err == nil {
			return parsed, nil
		}
		lastErr = err
	}
	return time.Time{}, lastErr
}
