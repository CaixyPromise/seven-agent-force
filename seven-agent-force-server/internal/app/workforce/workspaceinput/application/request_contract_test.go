package application

import (
	"encoding/json"
	"testing"
)

func TestWorkforceUploadRequestsPreserveInt64FileIdentity(t *testing.T) {
	const fileID = int64(8000000000000000001)
	tests := []struct {
		name   string
		decode func([]byte) (int64, error)
	}{
		{
			name: "workspace source",
			decode: func(body []byte) (int64, error) {
				var request UploadSourceRequest
				err := json.Unmarshal(body, &request)
				return request.FileID, err
			},
		},
		{
			name: "reference asset",
			decode: func(body []byte) (int64, error) {
				var request ReferenceRequest
				err := json.Unmarshal(body, &request)
				return request.FileID, err
			},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got, err := test.decode([]byte(`{"fileId":"8000000000000000001"}`))
			if err != nil {
				t.Fatalf("decode string fileId: %v", err)
			}
			if got != fileID {
				t.Fatalf("fileId = %d, want %d", got, fileID)
			}
			if _, err = test.decode([]byte(`{"fileId":8000000000000000001}`)); err == nil {
				t.Fatal("numeric fileId must be rejected before JavaScript can silently round it")
			}
		})
	}
}
