package imageadmission

import (
	"encoding/json"
	"testing"
)

func TestIndependentReviewCapabilitiesContainWorkerProbeEvidence(t *testing.T) {
	required := []string{"review.provider.gateway-client", "review.fixed-provider-identity"}
	tests := []struct {
		name string
		raw  string
		want bool
	}{
		{
			name: "legacy direct capability list",
			raw:  `["review.provider.gateway-client","review.fixed-provider-identity"]`,
			want: true,
		},
		{
			name: "seven worker image probe records supported capabilities",
			raw: `{"harnessType":"codex","capabilities":[
				{"name":"review.provider.gateway-client","status":"supported"},
				{"name":"review.fixed-provider-identity","status":"SUPPORTED"}
			]}`,
			want: true,
		},
		{
			name: "unsupported probe capability cannot satisfy independent review",
			raw: `{"capabilities":[
				{"name":"review.provider.gateway-client","status":"supported"},
				{"name":"review.fixed-provider-identity","status":"unsupported"}
			]}`,
			want: false,
		},
		{
			name: "missing probe capability is rejected",
			raw:  `{"capabilities":[{"name":"review.provider.gateway-client","status":"supported"}]}`,
			want: false,
		},
		{
			name: "malformed capability evidence is rejected",
			raw:  `{"capabilities":"not-an-array"}`,
			want: false,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := independentReviewCapabilitiesContain(json.RawMessage(test.raw), required); got != test.want {
				t.Fatalf("independentReviewCapabilitiesContain()=%v, want %v", got, test.want)
			}
		})
	}
}
