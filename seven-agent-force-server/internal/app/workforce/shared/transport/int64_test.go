package transport

import (
	"encoding/json"
	"reflect"
	"testing"
)

func TestInt64SliceUsesLosslessDecimalStrings(t *testing.T) {
	values := Int64Slice{1, 2065424359060983808, 9223372036854775807}
	encoded, err := json.Marshal(values)
	if err != nil {
		t.Fatal(err)
	}
	if got, want := string(encoded), `["1","2065424359060983808","9223372036854775807"]`; got != want {
		t.Fatalf("unexpected encoding %s", got)
	}
	var decoded Int64Slice
	if err := json.Unmarshal(encoded, &decoded); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(decoded, values) {
		t.Fatalf("round trip mismatch: %#v", decoded)
	}
}

func TestInt64SliceRejectsNumericAndInvalidIdentities(t *testing.T) {
	for _, payload := range []string{`[2065424359060983808]`, `["0"]`, `["01"]`, `["-1"]`, `["9223372036854775808"]`} {
		var decoded Int64Slice
		if err := json.Unmarshal([]byte(payload), &decoded); err == nil {
			t.Fatalf("expected %s to be rejected", payload)
		}
	}
}
