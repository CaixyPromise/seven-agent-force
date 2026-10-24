package infrastructure

import (
	"reflect"
	"strings"
	"testing"
)

func TestDeliveryDiagnosticScopePredicateIncludesLegacyNullOnlyForLocal(t *testing.T) {
	localPredicate, localArgs := deliveryDiagnosticScopePredicate("local")
	if !strings.Contains(localPredicate, ".scopeId IS NULL") {
		t.Fatalf("local diagnostic predicate=%q, want legacy NULL compatibility", localPredicate)
	}
	if !reflect.DeepEqual(localArgs, []any{"local", "local", "local"}) {
		t.Fatalf("local diagnostic args=%#v", localArgs)
	}

	nodePredicate, nodeArgs := deliveryDiagnosticScopePredicate("node:gray-b")
	if strings.Contains(nodePredicate, "IS NULL") {
		t.Fatalf("node diagnostic predicate=%q, must not include legacy NULL rows", nodePredicate)
	}
	if !reflect.DeepEqual(nodeArgs, []any{"node:gray-b", "node:gray-b", "node:gray-b"}) {
		t.Fatalf("node diagnostic args=%#v", nodeArgs)
	}

	emptyPredicate, emptyArgs := deliveryDiagnosticScopePredicate(" ")
	if emptyPredicate != "1=0" || len(emptyArgs) != 0 {
		t.Fatalf("empty scope predicate=%q args=%#v, want fail closed", emptyPredicate, emptyArgs)
	}
}
