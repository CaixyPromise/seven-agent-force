package websocket

import (
	"reflect"
	"strconv"
	"strings"

	jsoncompat "github.com/CaixyPromise/seven-agent/force/internal/common/json"
	"github.com/bytedance/sonic"
	"github.com/go-viper/mapstructure/v2"
)

func marshalJSON(value any) ([]byte, error) {
	return sonic.Marshal(jsoncompat.NormalizeForJSON(value))
}

func unmarshalJSON(payload []byte, dest any) error {
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
