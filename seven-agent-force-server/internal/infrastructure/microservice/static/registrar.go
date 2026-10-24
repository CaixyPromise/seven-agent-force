package static

import (
	"context"

	"github.com/CaixyPromise/seven-agent/force/internal/infrastructure/microservice"
)

// Registrar explicitly reports that static discovery has no registration protocol.
type Registrar struct{}

func (Registrar) Register(context.Context, microservice.ServiceRegistration) error {
	return microservice.ErrRegistrationUnsupported
}

func (Registrar) Deregister(context.Context, string) error {
	return microservice.ErrRegistrationUnsupported
}
