package resourcejournal

import (
	"context"
	"fmt"
	"testing"

	config "github.com/CaixyPromise/seven-agent/force/internal/app/workforce/runtimeconfig"
)

func TestConfiguredBudgetAdmitsLiveAgentProxyAndCurrentCheckButRejectsAboveCap(t *testing.T) {
	j := openTest(t)
	budget := config.RequiredContainerBudget(1, 1, 0)
	j.SetContainerLimit(budget)
	creates := 0
	run := func(context.Context, string, ...string) (string, error) {
		creates++
		return fmt.Sprintf("%064x", creates), nil
	}
	for _, name := range []string{"agent", "provider-proxy", "current-check", "reviewer", "review-proxy", "delivery-runner", "delivery-proxy"} {
		if _, err := j.Execute(context.Background(), run, "docker", "create", "--name", name, "image"); err != nil {
			t.Fatalf("normal peak %s refused: %v", name, err)
		}
	}
	if creates != budget {
		t.Fatalf("created %d expected %d", creates, budget)
	}
	if _, err := j.Execute(context.Background(), run, "docker", "create", "--name", "above-cap", "image"); err == nil {
		t.Fatal("above-cap creation accepted")
	}
	if creates != budget {
		t.Fatal("above-cap request reached Docker")
	}
}
