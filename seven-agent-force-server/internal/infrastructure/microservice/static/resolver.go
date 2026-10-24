package static

import (
	"context"
	"fmt"

	"github.com/CaixyPromise/seven-agent/force/internal/infrastructure/microservice"
)

type Resolver struct {
	services map[string][]microservice.ServiceInstance
}

func NewResolver(configured map[string][]string) (*Resolver, error) {
	services := make(map[string][]microservice.ServiceInstance, len(configured))
	for serviceName, urls := range configured {
		for _, rawURL := range urls {
			instance, err := microservice.ParseServiceURL(serviceName, rawURL)
			if err != nil {
				return nil, fmt.Errorf("static service %q: %w", serviceName, err)
			}
			services[serviceName] = append(services[serviceName], instance)
		}
	}
	return &Resolver{services: services}, nil
}

func (r *Resolver) Resolve(ctx context.Context, serviceName string) ([]microservice.ServiceInstance, error) {
	if r == nil {
		return nil, microservice.ErrInvalidDependency
	}
	if ctx == nil {
		return nil, microservice.ErrInvalidContext
	}
	if serviceName == "" {
		return nil, microservice.ErrInvalidRequest
	}
	instances := r.services[serviceName]
	if len(instances) == 0 {
		return nil, microservice.ErrNoHealthyInstance
	}
	result := make([]microservice.ServiceInstance, len(instances))
	copy(result, instances)
	return result, nil
}
