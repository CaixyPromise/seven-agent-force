package workforce

import (
	workforceconfig "github.com/CaixyPromise/seven-agent/force/internal/app/workforce/runtimeconfig"
	sharedconfig "github.com/CaixyPromise/seven-agent/force/internal/shared/config"
	"testing"
)

func TestRuntimeConfigUsesSharedContainerBudget(t *testing.T) {
	var cfg sharedconfig.Config
	cfg.Workforce.Worker.MaxActiveContainers = 1
	actual := RuntimeConfig(cfg, "").Worker
	if actual.MaxTotalContainers != 7 || actual.MaxTotalContainers != workforceconfig.RequiredContainerBudget(actual.MaxActiveContainers, actual.MaxActiveReviews, actual.MaxActiveDeliveries) {
		t.Fatalf("mapping budget drift: %+v", actual)
	}
	if actual.MaxActiveContainers != 1 || actual.MaxActiveReviews != 1 || actual.MaxActiveDeliveries != 0 {
		t.Fatalf("logical concurrency changed: %+v", actual)
	}
}
