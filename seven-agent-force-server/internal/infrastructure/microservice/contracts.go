package microservice

import "context"

// Registrar registers and deregisters service instances.
type Registrar interface {
	Register(context.Context, ServiceRegistration) error
	Deregister(context.Context, string) error
}

// ServiceResolver discovers healthy instances for a configured service name.
type ServiceResolver interface {
	Resolve(context.Context, string) ([]ServiceInstance, error)
}

// LoadBalancer selects a healthy instance that has not been excluded by the current call.
type LoadBalancer interface {
	Select(serviceKey string, instances []ServiceInstance, excluded map[string]struct{}) (ServiceInstance, error)
}

// ServiceClient sends bounded HTTP requests to discovered service instances.
type ServiceClient interface {
	Do(context.Context, ServiceRequest) (*ServiceResponse, error)
}
