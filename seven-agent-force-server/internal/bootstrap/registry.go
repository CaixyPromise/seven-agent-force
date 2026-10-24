package bootstrap

import (
	"sync"

	"github.com/CaixyPromise/seven-agent/force/internal/shared/core"
)

type Registry struct {
	mu      sync.RWMutex
	modules []core.ModuleDescriptor
}

func NewRegistry() *Registry {
	return &Registry{
		modules: make([]core.ModuleDescriptor, 0, 4),
	}
}

func (r *Registry) Register(module core.Module) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.modules = append(r.modules, module.Descriptor())
}

func (r *Registry) ListModules() []core.ModuleDescriptor {
	r.mu.RLock()
	defer r.mu.RUnlock()

	result := make([]core.ModuleDescriptor, len(r.modules))
	copy(result, r.modules)
	return result
}
