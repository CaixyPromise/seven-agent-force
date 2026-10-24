package cache_governance

import (
	"context"
	"strings"
	"testing"

	bootstrapruntime "github.com/CaixyPromise/seven-agent/force/internal/bootstrap/runtime"
	jobscheduler "github.com/CaixyPromise/seven-agent/force/internal/infrastructure/job/scheduler"
)

func TestInstallRefusesEnabledGovernanceWithoutOutboxScheduler(t *testing.T) {
	deps := bootstrapruntime.ModuleDeps{}
	deps.Config.Cache.Governance.Enabled = true

	module, registrar, err := Install(deps)
	if err == nil || !strings.Contains(err.Error(), "scheduler") {
		t.Fatalf("enabled DG5 without scheduler must fail closed, module=%#v registrar=%#v err=%v", module, registrar, err)
	}
}

func TestInstallRefusesEnabledGovernanceWithoutDistributedOutboxID(t *testing.T) {
	deps := bootstrapruntime.ModuleDeps{}
	deps.Config.Cache.Governance.Enabled = true
	deps.Infra.Jobs = moduleTestScheduler{}

	module, registrar, err := Install(deps)
	if err == nil || !strings.Contains(err.Error(), "id generator") {
		t.Fatalf("enabled DG5 without distributed ID must fail closed, module=%#v registrar=%#v err=%v", module, registrar, err)
	}
}

type moduleTestScheduler struct{}

func (moduleTestScheduler) Register(jobscheduler.Job) error { return nil }
func (moduleTestScheduler) Start(context.Context) error     { return nil }
func (moduleTestScheduler) Stop(context.Context) error      { return nil }
func (moduleTestScheduler) Running() bool                   { return false }
