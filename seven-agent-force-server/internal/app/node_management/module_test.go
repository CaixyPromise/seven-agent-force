package node_management

import (
	"context"
	"testing"

	externalfacade "github.com/CaixyPromise/seven-agent/force/internal/app/external_login/facade"
	nodefacade "github.com/CaixyPromise/seven-agent/force/internal/app/node_management/facade"
)

func TestManagedOIDCPortProjectsNodeOwnerApplyAndDisable(t *testing.T) {
	managed := &fakeManagedOIDCProvider{}
	port := newManagedOIDCPort("order-admin", managed)
	command := nodefacade.ManagedHubConnectionCommand{
		ConnectionVersion: "v8", TargetRevision: 8, Enabled: true, DisplayName: "Hub", Issuer: "https://hub.example.com",
		ClientID: "hub-node-order-admin", ClientSecret: "secret", RedirectURI: "https://node.example.com/callback",
	}
	if err := port.ApplyHubConnection(context.Background(), command); err != nil {
		t.Fatalf("apply managed oidc: %v", err)
	}
	if managed.applied.OwnerNodeCode != "order-admin" || managed.applied.ConnectionVersion != "v8" || managed.applied.TargetRevision != 8 || managed.applied.Issuer != command.Issuer ||
		managed.applied.ClientID != command.ClientID || managed.applied.ClientSecret != command.ClientSecret || managed.applied.RedirectURI != command.RedirectURI {
		t.Fatalf("managed apply=%+v", managed.applied)
	}
	command.Enabled = false
	command.ConnectionVersion = "v9"
	if err := port.ApplyHubConnection(context.Background(), command); err != nil {
		t.Fatalf("disable managed oidc: %v", err)
	}
	if managed.disabledOwner != "order-admin" || managed.disabledVersion != "v9" || managed.disabledRevision != 8 {
		t.Fatalf("managed disable owner=%q version=%q", managed.disabledOwner, managed.disabledVersion)
	}
}

type fakeManagedOIDCProvider struct {
	applied          externalfacade.ManagedOIDCProviderCommand
	disabledOwner    string
	disabledVersion  string
	disabledRevision int64
}

func (f *fakeManagedOIDCProvider) ApplyManagedOIDCProvider(_ context.Context, command externalfacade.ManagedOIDCProviderCommand) error {
	f.applied = command
	return nil
}

func (f *fakeManagedOIDCProvider) DisableManagedOIDCProvider(_ context.Context, ownerNodeCode, connectionVersion string, targetRevision int64) error {
	f.disabledOwner = ownerNodeCode
	f.disabledVersion = connectionVersion
	f.disabledRevision = targetRevision
	return nil
}
