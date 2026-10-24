package facade

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestValidInstructionCountsUnicodeCharacters(t *testing.T) {
	if !ValidInstruction(strings.Repeat("修", 10000)) {
		t.Fatal("a contract-valid multilingual instruction was rejected")
	}
	if ValidInstruction("") || ValidInstruction(strings.Repeat("修", MaxInstructionCharacters+1)) {
		t.Fatal("instruction boundary was not enforced")
	}
}

func TestProjectTaskRequestKeepsOnlyUserSelectedExecutionImage(t *testing.T) {
	request := ProjectTaskRequest{Instruction: "fix pagination", DeliveryPreference: "PROJECT_DEFAULT", RuntimeReleaseID: 9007199254740993}
	additions := request.Additions()
	if additions.Instruction != request.Instruction || additions.DeliveryPreference != request.DeliveryPreference {
		t.Fatalf("project Task additions changed business input: %#v", additions)
	}
	body, err := json.Marshal(request)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(body), `"runtimeReleaseId":"9007199254740993"`) {
		t.Fatalf("runtime release identity was not transported as a decimal string: %s", body)
	}
}
