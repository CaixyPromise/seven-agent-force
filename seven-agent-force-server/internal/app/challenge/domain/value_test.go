package domain

import (
	"os"
	"regexp"
	"runtime"
	"testing"
)

func TestAllChallengeTypesCoversDeclaredConstants(t *testing.T) {
	declared := declaredChallengeTypeValues(t)
	listed := make(map[ChallengeType]struct{}, len(AllChallengeTypes()))
	for _, item := range AllChallengeTypes() {
		if _, duplicate := listed[item]; duplicate {
			t.Fatalf("AllChallengeTypes contains duplicate challenge type: %s", item)
		}
		listed[item] = struct{}{}
	}
	if len(listed) != len(declared) {
		t.Fatalf("AllChallengeTypes size = %d, declared constants = %d", len(listed), len(declared))
	}
	for item := range declared {
		if _, ok := listed[item]; !ok {
			t.Fatalf("declared challenge type %s is missing from AllChallengeTypes", item)
		}
	}
	for item := range listed {
		if _, ok := declared[item]; !ok {
			t.Fatalf("AllChallengeTypes contains undeclared challenge type: %s", item)
		}
	}
}

func declaredChallengeTypeValues(t *testing.T) map[ChallengeType]struct{} {
	t.Helper()
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("resolve value_test.go path")
	}
	source, err := os.ReadFile(file[:len(file)-len("_test.go")] + ".go")
	if err != nil {
		t.Fatalf("read challenge value source: %v", err)
	}
	matches := regexp.MustCompile(`ChallengeType[A-Za-z0-9_]+\s+ChallengeType\s+=\s+"([^"]+)"`).FindAllSubmatch(source, -1)
	if len(matches) == 0 {
		t.Fatal("no ChallengeType constants found")
	}
	declared := make(map[ChallengeType]struct{}, len(matches))
	for _, match := range matches {
		declared[ChallengeType(string(match[1]))] = struct{}{}
	}
	return declared
}
