package application

import (
	"context"
	"strings"
	"testing"

	"github.com/CaixyPromise/seven-agent/force/internal/app/notification/domain"
	"github.com/CaixyPromise/seven-agent/force/internal/app/notification/facade"
)

func TestUpsertHTTPConnectorRequiresTypedStaticConfigurationAndCanEnableInG52(t *testing.T) {
	repo := newExternalTestRepository()
	service := newExternalTestService(t, repo, nil)
	service.urls = allowAllChannelURLs{}

	_, err := service.UpsertChannel(context.Background(), facade.ChannelUpsertRequest{
		ChannelCode:         "orders-http",
		ChannelName:         "Orders HTTP",
		ChannelType:         domain.ChannelTypeHTTPConnector,
		Status:              domain.ChannelStatusDisabled,
		ConfigJSON:          `{"endpointUrl":"https://attacker.example/ignored"}`,
		HTTPConnectorConfig: validFacadeHTTPConnectorConfig(),
	}, 1)
	if err == nil || repo.channels["orders-http"] != nil {
		t.Fatalf("raw HTTP connector configuration err=%v saved=%#v, want rejection before persistence", err, repo.channels["orders-http"])
	}

	record, err := service.UpsertChannel(context.Background(), facade.ChannelUpsertRequest{
		ChannelCode:         "orders-http",
		ChannelName:         "Orders HTTP",
		ChannelType:         domain.ChannelTypeHTTPConnector,
		Status:              domain.ChannelStatusDisabled,
		SecretPlain:         "connector-secret",
		HTTPConnectorConfig: validFacadeHTTPConnectorConfig(),
	}, 1)
	if err != nil {
		t.Fatalf("upsert typed HTTP connector: %v", err)
	}
	if record.HTTPConnectorConfig == nil || record.ConfigJSON != "" || record.MetadataJSON != "" || record.RateLimitJSON != "" || !record.SecretConfigured {
		t.Fatalf("public HTTP connector record=%#v", record)
	}
	if record.HTTPConnectorConfig.Method != domain.HTTPConnectorMethodPOST || record.HTTPConnectorConfig.AuthenticationMode != domain.HTTPConnectorAuthBearer || record.HTTPConnectorConfig.IdempotencyHeader != domain.HTTPConnectorIdempotencyHeader {
		t.Fatalf("static HTTP connector record=%#v", record.HTTPConnectorConfig)
	}

	saved := repo.channels["orders-http"]
	if saved == nil || saved.Status != domain.ChannelStatusDisabled || saved.ScopeID != "local" || strings.Contains(saved.ConfigJSON, "attacker") || strings.Contains(saved.ConfigJSON, "connector-secret") || strings.Contains(saved.ConfigJSON, "secretRef") == false {
		t.Fatalf("persisted HTTP connector=%#v", saved)
	}
	if strings.TrimSpace(saved.SecretCiphertext) == "" || strings.TrimSpace(saved.SecretEDEK) == "" || strings.TrimSpace(saved.SecretWrapKeyRef) == "" {
		t.Fatalf("HTTP connector secret was not encrypted=%#v", saved)
	}

	enabled, err := service.UpsertChannel(context.Background(), facade.ChannelUpsertRequest{
		ChannelCode: "orders-http",
		ChannelName: "Orders HTTP",
		ChannelType: domain.ChannelTypeHTTPConnector,
		Status:      domain.ChannelStatusEnabled,
	}, 1)
	if err != nil {
		t.Fatalf("enable G5.2 HTTP connector: %v", err)
	}
	if enabled.Status != domain.ChannelStatusEnabled || repo.channels["orders-http"].Status != domain.ChannelStatusEnabled {
		t.Fatalf("enabled HTTP connector=%#v saved=%#v", enabled, repo.channels["orders-http"])
	}
}

func TestUpsertHTTPConnectorRejectsSecretWhenAuthenticationIsDisabled(t *testing.T) {
	repo := newExternalTestRepository()
	service := newExternalTestService(t, repo, nil)
	service.urls = allowAllChannelURLs{}
	config := validFacadeHTTPConnectorConfig()
	config.AuthenticationMode = domain.HTTPConnectorAuthNone

	_, err := service.UpsertChannel(context.Background(), facade.ChannelUpsertRequest{
		ChannelCode:         "http-no-auth",
		ChannelName:         "HTTP no auth",
		ChannelType:         domain.ChannelTypeHTTPConnector,
		Status:              domain.ChannelStatusDisabled,
		SecretPlain:         "must-not-be-stored",
		HTTPConnectorConfig: config,
	}, 1)
	if err == nil || repo.channels["http-no-auth"] != nil {
		t.Fatalf("unauthenticated HTTP connector secret err=%v saved=%#v", err, repo.channels["http-no-auth"])
	}
}

func TestUpsertHTTPConnectorRequiresConnectionSecretForSelectedAuthentication(t *testing.T) {
	repo := newExternalTestRepository()
	service := newExternalTestService(t, repo, nil)
	service.urls = allowAllChannelURLs{}

	_, err := service.UpsertChannel(context.Background(), facade.ChannelUpsertRequest{
		ChannelCode:         "http-auth-without-secret",
		ChannelName:         "HTTP auth without secret",
		ChannelType:         domain.ChannelTypeHTTPConnector,
		Status:              domain.ChannelStatusDisabled,
		HTTPConnectorConfig: validFacadeHTTPConnectorConfig(),
	}, 1)
	if err == nil || repo.channels["http-auth-without-secret"] != nil {
		t.Fatalf("authenticated HTTP connector without secret err=%v saved=%#v", err, repo.channels["http-auth-without-secret"])
	}
}

func TestUpsertHTTPConnectorRejectsMTLSBeforeAnyCertificateCanBeStored(t *testing.T) {
	repo := newExternalTestRepository()
	service := newExternalTestService(t, repo, nil)
	service.urls = allowAllChannelURLs{}
	config := validFacadeHTTPConnectorConfig()
	config.AuthenticationMode = domain.HTTPConnectorAuthMTLS

	_, err := service.UpsertChannel(context.Background(), facade.ChannelUpsertRequest{
		ChannelCode:         "http-mtls",
		ChannelName:         "HTTP mTLS",
		ChannelType:         domain.ChannelTypeHTTPConnector,
		Status:              domain.ChannelStatusDisabled,
		SecretPlain:         "-----BEGIN PRIVATE KEY-----",
		HTTPConnectorConfig: config,
	}, 1)
	if err == nil || repo.channels["http-mtls"] != nil {
		t.Fatalf("G5.2 mTLS err=%v saved=%#v, want rejection before secret persistence", err, repo.channels["http-mtls"])
	}
}

func validFacadeHTTPConnectorConfig() *facade.HTTPConnectorConfig {
	return &facade.HTTPConnectorConfig{
		EndpointURL:        "https://orders.example/notifications",
		EgressPolicyRef:    "corp-orders",
		Method:             domain.HTTPConnectorMethodPOST,
		AuthenticationMode: domain.HTTPConnectorAuthBearer,
		FieldMappings: []facade.HTTPConnectorFieldMapping{
			{Source: domain.HTTPConnectorFieldSubject, Target: "title"},
			{Source: domain.HTTPConnectorFieldText, Target: "message"},
		},
		HeaderAllowlist:     []string{"X-Notification-Source"},
		IdempotencyHeader:   domain.HTTPConnectorIdempotencyHeader,
		TimeoutMilliseconds: 5000,
		SuccessStatusCodes:  []int{202},
	}
}
