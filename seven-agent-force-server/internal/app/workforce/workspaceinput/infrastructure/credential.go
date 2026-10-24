package infrastructure

import (
	"context"

	deliveryapp "github.com/CaixyPromise/seven-agent/force/internal/app/workforce/delivery/application"
	"github.com/CaixyPromise/seven-agent/force/internal/app/workforce/workspaceinput/application"
)

type deliveryCredentialResolver interface {
	ResolveDeliveryCredential(context.Context, string, string) (deliveryapp.DeliveryCredential, error)
}

type DeliveryCredentialAdapter struct{ Resolver deliveryCredentialResolver }

func (a DeliveryCredentialAdapter) ResolveSourceCredential(ctx context.Context, ref, version string) (application.SourceCredential, error) {
	value, err := a.Resolver.ResolveDeliveryCredential(ctx, ref, version)
	return application.SourceCredential{Username: value.Username, Password: value.Password, PrivateKey: value.PrivateKey, KnownHosts: value.KnownHosts, ExpiresAt: value.ExpiresAt}, err
}
