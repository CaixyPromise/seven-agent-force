package build

import (
	"context"
	"os"
	"strings"
	"testing"

	externalfacade "github.com/CaixyPromise/seven-agent/force/internal/app/external_login/facade"
	bootstrapruntime "github.com/CaixyPromise/seven-agent/force/internal/bootstrap/runtime"
	"github.com/CaixyPromise/seven-agent/force/internal/shared/config"
	"github.com/CaixyPromise/seven-agent/force/internal/shared/core"
	"github.com/CaixyPromise/seven-agent/force/internal/shared/features"
	"github.com/cloudwego/hertz/pkg/route"
)

func TestRuntimeDepsRemainAppAgnostic(t *testing.T) {
	depsSource, err := os.ReadFile("../runtime/deps.go")
	if err != nil {
		t.Fatalf("read runtime deps: %v", err)
	}
	if strings.Contains(string(depsSource), "internal/app/") {
		t.Fatal("bootstrap runtime dependencies must not import application packages")
	}
}

func TestLocalAndNodeInstallPolicyCoreWithoutControlPlane(t *testing.T) {
	policyCore := fakePlatformModule{name: "platform-policy-core"}
	controlPlane := fakePlatformModule{name: "platform-control-plane"}
	platformModules := platformModules{
		PolicyCore:   policyCore,
		ControlPlane: controlPlane,
	}

	for _, mode := range []config.PlatformMode{config.PlatformModeLocal, config.PlatformModeNode} {
		t.Run(string(mode), func(t *testing.T) {
			modules := appendPlatformModules(nil, features.Resolve(config.Config{Platform: config.PlatformConfig{Mode: mode}}), platformModules)
			if len(modules) != 1 || modules[0].Descriptor().Name != policyCore.name {
				t.Fatalf("mode %s modules = %#v, want policy core only", mode, moduleNames(modules))
			}
		})
	}
}

func TestHubInstallsPlatformControlPlane(t *testing.T) {
	policyCore := fakePlatformModule{name: "platform-policy-core"}
	controlPlane := fakePlatformModule{name: "platform-control-plane"}
	modules := appendPlatformModules(nil, features.Resolve(config.Config{Platform: config.PlatformConfig{Mode: config.PlatformModeHub}}), platformModules{
		PolicyCore:   policyCore,
		ControlPlane: controlPlane,
	})
	if got := moduleNames(modules); len(got) != 2 || got[0] != policyCore.name || got[1] != controlPlane.name {
		t.Fatalf("hub modules = %#v, want [%q %q]", got, policyCore.name, controlPlane.name)
	}
}

func TestHubControlModuleIsInstalledOnlyInHubMode(t *testing.T) {
	hubControl := fakePlatformModule{name: "hub-control"}
	for _, mode := range []config.PlatformMode{config.PlatformModeLocal, config.PlatformModeHub, config.PlatformModeNode} {
		modules := appendHubControlModule(nil, features.Resolve(config.Config{Platform: config.PlatformConfig{Mode: mode}}), hubControl)
		if mode == config.PlatformModeHub {
			if got := moduleNames(modules); len(got) != 1 || got[0] != "hub-control" {
				t.Fatalf("hub modules=%v", got)
			}
		} else if len(modules) != 0 {
			t.Fatalf("mode %s installed hub control: %v", mode, moduleNames(modules))
		}
	}
}

func TestManagedHubOIDCBindingExistsOnlyInNodeMode(t *testing.T) {
	for _, mode := range []config.PlatformMode{config.PlatformModeLocal, config.PlatformModeHub, config.PlatformModeNode} {
		t.Run(string(mode), func(t *testing.T) {
			binder := &fakeManagedOIDCBinder{}
			bound := bindManagedOIDCProvider(features.Resolve(config.Config{Platform: config.PlatformConfig{Mode: mode}}), binder, fakeManagedOIDCFacade{})
			want := mode == config.PlatformModeNode
			if bound != want || binder.calls != boolToInt(want) {
				t.Fatalf("mode=%s bound=%v calls=%d want=%v", mode, bound, binder.calls, want)
			}
		})
	}
}

func TestModulesRejectsNodeModeWhenManagedExternalLoginIsUnavailable(t *testing.T) {
	_, err := Modules(bootstrapruntime.ModuleDeps{Config: config.Config{
		Platform:      config.PlatformConfig{Mode: config.PlatformModeNode, Node: config.PlatformNodeConfig{Code: "order-admin", ManagementBearer: "node-bearer"}},
		ExternalLogin: config.ExternalLoginConfig{Enabled: false},
	}})
	if err == nil || !strings.Contains(err.Error(), "node federation requires external login") {
		t.Fatalf("Modules() error=%v, want managed external-login composition failure", err)
	}
}

type fakeManagedOIDCBinder struct{ calls int }

func (f *fakeManagedOIDCBinder) BindManagedOIDCProvider(externalfacade.ManagedOIDCProviderFacade) {
	f.calls++
}

type fakeManagedOIDCFacade struct{}

func (fakeManagedOIDCFacade) ApplyManagedOIDCProvider(context.Context, externalfacade.ManagedOIDCProviderCommand) error {
	return nil
}
func (fakeManagedOIDCFacade) DisableManagedOIDCProvider(context.Context, string, string, int64) error {
	return nil
}

func boolToInt(value bool) int {
	if value {
		return 1
	}
	return 0
}

type fakePlatformModule struct {
	name string
}

func (m fakePlatformModule) Descriptor() core.ModuleDescriptor {
	return core.ModuleDescriptor{Name: m.name}
}

func (fakePlatformModule) Mount(route.IRouter) {}

func moduleNames(modules []core.Module) []string {
	names := make([]string, 0, len(modules))
	for _, module := range modules {
		names = append(names, module.Descriptor().Name)
	}
	return names
}
