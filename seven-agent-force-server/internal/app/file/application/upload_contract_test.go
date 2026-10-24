package application

import (
	"encoding/json"
	"reflect"
	"testing"

	"github.com/CaixyPromise/seven-agent/force/internal/app/file/domain"
)

func TestUploadResultTerminalJSONContainsOnlyFileID(t *testing.T) {
	payload, err := json.Marshal(UploadResult{FileID: 42})
	if err != nil {
		t.Fatalf("json.Marshal() error = %v", err)
	}
	if got, want := string(payload), `{"fileId":42}`; got != want {
		t.Fatalf("terminal upload payload = %s, want %s", got, want)
	}
}

func TestUploadTaskStatusDoesNotExposeFileBeforeTerminalState(t *testing.T) {
	pending, err := json.Marshal(uploadTaskStatusResponse(&domain.UploadTask{ID: "task-1", Status: domain.UploadTaskProcessing, FileID: 42}))
	if err != nil {
		t.Fatalf("json.Marshal(pending) error = %v", err)
	}
	if got, want := string(pending), `{"taskId":"task-1","status":"PROCESSING"}`; got != want {
		t.Fatalf("pending status payload = %s, want %s", got, want)
	}

	terminal, err := json.Marshal(uploadTaskStatusResponse(&domain.UploadTask{ID: "task-1", Status: domain.UploadTaskClean, FileID: 42}))
	if err != nil {
		t.Fatalf("json.Marshal(terminal) error = %v", err)
	}
	if got, want := string(terminal), `{"taskId":"task-1","status":"CLEAN","fileId":42}`; got != want {
		t.Fatalf("terminal status payload = %s, want %s", got, want)
	}
}

func TestUploadDTOsDoNotAcceptBusinessOrScopeAuthority(t *testing.T) {
	for _, value := range []any{
		UploadRequest{},
		UploadTaskInitRequest{},
		ChunkUploadInitRequest{},
	} {
		typ := reflect.TypeOf(value)
		for _, forbidden := range []string{"BizType", "BizID", "ScopeID", "ReferenceID", "AccessScope", "VisitStrategy"} {
			if _, ok := typ.FieldByName(forbidden); ok {
				t.Errorf("%s still exposes forbidden authority field %s", typ.Name(), forbidden)
			}
		}
	}
}
