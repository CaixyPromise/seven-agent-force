package registry

import (
	"context"
	"fmt"
	"sync"
)

type Job interface {
	Name() string
	Spec() string
	Run(ctx context.Context) error
}

type Registry interface {
	Register(job Job) error
	List() []Job
}

type MemoryRegistry struct {
	mu   sync.RWMutex
	jobs []Job
}

func New() *MemoryRegistry {
	return &MemoryRegistry{}
}

func (r *MemoryRegistry) Register(job Job) error {
	if job == nil {
		return fmt.Errorf("job must not be nil")
	}
	if job.Name() == "" {
		return fmt.Errorf("job name must not be empty")
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, existing := range r.jobs {
		if existing.Name() == job.Name() {
			return fmt.Errorf("job %s already registered", job.Name())
		}
	}
	r.jobs = append(r.jobs, job)
	return nil
}

func (r *MemoryRegistry) List() []Job {
	r.mu.RLock()
	defer r.mu.RUnlock()
	result := make([]Job, len(r.jobs))
	copy(result, r.jobs)
	return result
}
