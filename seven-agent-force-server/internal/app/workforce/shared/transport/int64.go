package transport

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strconv"
)

// Int64Slice keeps database identities as int64 inside Go while encoding every
// value as a decimal JSON string at HTTP, RabbitMQ, and persisted-contract
// boundaries. encoding/json's `,string` option does not apply to slice items.
type Int64Slice []int64

func (values Int64Slice) MarshalJSON() ([]byte, error) {
	encoded := make([]string, len(values))
	for index, value := range values {
		if value <= 0 {
			return nil, fmt.Errorf("int64 identity at index %d must be positive", index)
		}
		encoded[index] = strconv.FormatInt(value, 10)
	}
	return json.Marshal(encoded)
}

func (values *Int64Slice) UnmarshalJSON(data []byte) error {
	if bytes.Equal(bytes.TrimSpace(data), []byte("null")) {
		*values = nil
		return nil
	}
	var encoded []string
	if err := json.Unmarshal(data, &encoded); err != nil {
		return fmt.Errorf("int64 identities must be decimal JSON strings: %w", err)
	}
	decoded := make(Int64Slice, len(encoded))
	for index, value := range encoded {
		parsed, err := strconv.ParseInt(value, 10, 64)
		if err != nil || parsed <= 0 || strconv.FormatInt(parsed, 10) != value {
			return fmt.Errorf("invalid positive decimal int64 identity at index %d", index)
		}
		decoded[index] = parsed
	}
	*values = decoded
	return nil
}
