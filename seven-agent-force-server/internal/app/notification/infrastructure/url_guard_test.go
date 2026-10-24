package infrastructure

import (
	"context"
	"net"
	"testing"

	"github.com/CaixyPromise/seven-agent/force/internal/app/notification/domain"
	"github.com/CaixyPromise/seven-agent/force/internal/infrastructure/security/outboundurl"
)

func TestChannelURLValidatorAppliesSharedGuardToConfiguredEndpoints(t *testing.T) {
	guard := outboundurl.NewOutboundURLGuard(outboundurl.Options{
		Resolver: notificationStaticResolver{addresses: map[string][]net.IPAddr{
			"enterprise.example": {{IP: net.ParseIP("10.20.1.9")}},
			"fake.example":       {{IP: net.ParseIP("198.18.1.5")}},
			"proxy.example":      {{IP: net.ParseIP("93.184.216.34")}},
		}},
		InterfaceAddrs: func() ([]net.Addr, error) { return nil, nil },
	})
	policies, err := outboundurl.NewEnvironmentPolicyRegistry([]outboundurl.EnvironmentPolicy{
		{
			Name:             "corp-private",
			Mode:             outboundurl.ModePrivateAllowlist,
			AllowedHostnames: []string{"enterprise.example"},
			AllowedCIDRs:     []string{"10.20.0.0/16"},
			AllowedPorts:     []int{443},
		},
		{
			Name:             "local-fake",
			Mode:             outboundurl.ModeFakeIPProxy,
			AllowedHostnames: []string{"fake.example"},
			AllowedCIDRs:     []string{"198.18.0.0/15"},
			AllowedPorts:     []int{443},
			ProxyURL:         "https://proxy.example:8443",
		},
	}, outboundurl.EnvironmentPolicyRegistryOptions{AllowFakeIPProxy: true})
	if err != nil {
		t.Fatalf("NewEnvironmentPolicyRegistry() error = %v", err)
	}
	validator := NewChannelURLValidatorWithPolicyRegistry(guard, policies)

	privateChannel := domain.Channel{
		ChannelType: domain.ChannelTypeWebhook,
		ConfigJSON:  `{"endpointUrl":"https://enterprise.example/hook","outboundUrlPolicyRef":"corp-private"}`,
	}
	if err := validator.ValidateChannel(context.Background(), privateChannel); err != nil {
		t.Fatalf("explicit intranet URL rejected: %v", err)
	}
	httpConnector := domain.Channel{
		ChannelType: domain.ChannelTypeHTTPConnector,
		ConfigJSON:  `{"endpointUrl":"https://enterprise.example/hook","egressPolicyRef":"corp-private","method":"POST"}`,
	}
	if err := validator.ValidateChannel(context.Background(), httpConnector); err != nil {
		t.Fatalf("HTTP connector environment-governed intranet URL rejected: %v", err)
	}
	bothPolicyReferences := domain.Channel{
		ChannelType: domain.ChannelTypeHTTPConnector,
		ConfigJSON:  `{"endpointUrl":"https://enterprise.example/hook","egressPolicyRef":"corp-private","outboundUrlPolicyRef":"corp-private"}`,
	}
	if err := validator.ValidateChannel(context.Background(), bothPolicyReferences); err == nil {
		t.Fatal("channel accepted two policy-reference keys")
	}
	selfApprovedPrivateChannel := domain.Channel{
		ChannelType: domain.ChannelTypeWebhook,
		ConfigJSON:  `{"endpointUrl":"https://enterprise.example/hook","outboundUrlPolicy":{"mode":"PRIVATE_ALLOWLIST","allowedCidrs":["10.20.0.0/16"]}}`,
	}
	if err := validator.ValidateChannel(context.Background(), selfApprovedPrivateChannel); err == nil {
		t.Fatal("connection configuration self-approved a private CIDR")
	}

	unsafeFakeChannel := domain.Channel{
		ChannelType: domain.ChannelTypeWebhook,
		ConfigJSON:  `{"endpointUrl":"https://fake.example/hook","outboundUrlPolicyRef":"corp-private"}`,
	}
	if err := validator.ValidateChannel(context.Background(), unsafeFakeChannel); err == nil {
		t.Fatal("direct fake-IP channel was accepted")
	}

	proxyFakeChannel := domain.Channel{
		ChannelType: domain.ChannelTypeWebhook,
		ConfigJSON:  `{"endpointUrl":"https://fake.example/hook","outboundUrlPolicyRef":"local-fake"}`,
	}
	if err := validator.ValidateChannel(context.Background(), proxyFakeChannel); err != nil {
		t.Fatalf("proxy-only fake-IP test channel rejected: %v", err)
	}

	proxyBypassChannel := domain.Channel{
		ChannelType: domain.ChannelTypeWebhook,
		ConfigJSON:  `{"endpointUrl":"https://proxy.example/hook","proxyUrl":"http://127.0.0.1:8080"}`,
	}
	if err := validator.ValidateChannel(context.Background(), proxyBypassChannel); err == nil {
		t.Fatal("unvalidated channel proxyUrl was accepted")
	}
	unknownPolicyChannel := domain.Channel{
		ChannelType: domain.ChannelTypeWebhook,
		ConfigJSON:  `{"endpointUrl":"https://enterprise.example/hook","outboundUrlPolicyRef":"not-configured"}`,
	}
	if err := validator.ValidateChannel(context.Background(), unknownPolicyChannel); err == nil {
		t.Fatal("channel accepted an unknown environment policy reference")
	}
}

type notificationStaticResolver struct {
	addresses map[string][]net.IPAddr
}

func (r notificationStaticResolver) LookupIPAddr(_ context.Context, host string) ([]net.IPAddr, error) {
	return append([]net.IPAddr(nil), r.addresses[host]...), nil
}
