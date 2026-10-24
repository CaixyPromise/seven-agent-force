package infrastructure

import "testing"

func TestMarshalStringListKeepsEmptyProjectListsAsJSONArrays(t *testing.T) {
	if got := string(marshalStringList(nil)); got != `[]` {
		t.Fatalf("nil list persisted as %s, want []", got)
	}
	if got := string(marshalStringList([]string{"language.node"})); got != `["language.node"]` {
		t.Fatalf("non-empty list persisted as %s", got)
	}
}
