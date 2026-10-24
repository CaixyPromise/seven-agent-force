package facade

import (
	"testing"

	"github.com/bytedance/sonic"
)

func TestScalarValidationRejectsUnknownHTTPFields(t *testing.T) {
	var request ConfigAddRequest
	err := sonic.Unmarshal([]byte(`{
		"groupId": 1,
		"configKey": "runtime.title",
		"configValue": "Seven",
		"valueType": "STRING",
		"validation": {
			"required": true,
			"javascript": "alert(1)"
		}
	}`), &request)
	if err == nil {
		t.Fatal("unknown validation field was silently accepted")
	}
}

func TestScalarValidationAcceptsDeclaredHTTPFields(t *testing.T) {
	var request ConfigAddRequest
	err := sonic.Unmarshal([]byte(`{
		"groupId": 1,
		"configKey": "runtime.title",
		"configValue": "Seven",
		"valueType": "STRING",
		"validation": {
			"required": true,
			"minLength": 1,
			"maxLength": 20
		}
	}`), &request)
	if err != nil {
		t.Fatalf("declared validation fields rejected: %v", err)
	}
	if request.Validation == nil || !request.Validation.Required {
		t.Fatalf("validation not decoded: %#v", request.Validation)
	}
}
