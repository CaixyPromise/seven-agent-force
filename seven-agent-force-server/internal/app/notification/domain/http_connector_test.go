package domain

import "testing"

func TestNormalizeHTTPConnectorConfigFreezesBoundedStaticContract(t *testing.T) {
	config, err := NormalizeHTTPConnectorConfig(HTTPConnectorConfig{
		EndpointURL:     "https://receiver.example/v1/notify",
		EgressPolicyRef: "corp-orders",
		Method:          HTTPConnectorMethodPOST,
		Authentication: HTTPConnectorAuthentication{
			Mode:      HTTPConnectorAuthBearer,
			SecretRef: HTTPConnectorSecretRefConnection,
		},
		FieldMappings: []HTTPConnectorFieldMapping{
			{Source: HTTPConnectorFieldSubject, Target: "title"},
			{Source: HTTPConnectorFieldText, Target: "message"},
		},
		HeaderAllowlist:     []string{"X-Notification-Source"},
		IdempotencyHeader:   HTTPConnectorIdempotencyHeader,
		TimeoutMilliseconds: 5000,
		SuccessStatusCodes:  []int{202, 204},
	})
	if err != nil {
		t.Fatalf("NormalizeHTTPConnectorConfig() error = %v", err)
	}
	if config.Method != HTTPConnectorMethodPOST || config.IdempotencyHeader != HTTPConnectorIdempotencyHeader {
		t.Fatalf("normalized fixed request contract = %#v", config)
	}
	if config.Authentication.SecretRef != HTTPConnectorSecretRefConnection || len(config.FieldMappings) != 2 || config.FieldMappings[0].Source != HTTPConnectorFieldSubject {
		t.Fatalf("normalized bounded fields = %#v", config)
	}
	if config.EndpointURL != "https://receiver.example/v1/notify" || config.EgressPolicyRef != "corp-orders" {
		t.Fatalf("normalized endpoint = %#v", config)
	}
}

func TestHTTPConnectorConfigRejectsUnboundedOrProtectedInputs(t *testing.T) {
	base := validHTTPConnectorConfig()
	tests := []struct {
		name   string
		mutate func(*HTTPConnectorConfig)
	}{
		{name: "non POST method", mutate: func(config *HTTPConnectorConfig) { config.Method = "PUT" }},
		{name: "non HTTPS endpoint", mutate: func(config *HTTPConnectorConfig) { config.EndpointURL = "http://receiver.example/notify" }},
		{name: "query secret endpoint", mutate: func(config *HTTPConnectorConfig) { config.EndpointURL = "https://receiver.example/notify?token=secret" }},
		{name: "protected header", mutate: func(config *HTTPConnectorConfig) { config.HeaderAllowlist = []string{"Authorization"} }},
		{name: "raw body source", mutate: func(config *HTTPConnectorConfig) { config.FieldMappings[0].Source = "RAW_BODY" }},
		{name: "path expression target", mutate: func(config *HTTPConnectorConfig) { config.FieldMappings[0].Target = "body.message" }},
		{name: "inline secret reference", mutate: func(config *HTTPConnectorConfig) { config.Authentication.SecretRef = "Bearer secret-value" }},
		{name: "mTLS unavailable", mutate: func(config *HTTPConnectorConfig) {
			config.Authentication.Mode = HTTPConnectorAuthMTLS
			config.Authentication.SecretRef = HTTPConnectorSecretRefClientCertificate
		}},
		{name: "wrong idempotency header", mutate: func(config *HTTPConnectorConfig) { config.IdempotencyHeader = "X-Idempotency" }},
		{name: "unsafe success code", mutate: func(config *HTTPConnectorConfig) { config.SuccessStatusCodes = []int{500} }},
		{name: "zero timeout", mutate: func(config *HTTPConnectorConfig) { config.TimeoutMilliseconds = 0 }},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			candidate := base
			candidate.FieldMappings = append([]HTTPConnectorFieldMapping(nil), base.FieldMappings...)
			tt.mutate(&candidate)
			if _, err := NormalizeHTTPConnectorConfig(candidate); err == nil {
				t.Fatalf("NormalizeHTTPConnectorConfig(%#v) unexpectedly succeeded", candidate)
			}
		})
	}
	if _, err := ParseHTTPConnectorConfig(`{"endpointUrl":"https://receiver.example/notify","fieldMappings":[{"source":"TEXT","target":"message"}],"script":"curl attacker"}`); err == nil {
		t.Fatal("ParseHTTPConnectorConfig() accepted executable connector content")
	}
}

func validHTTPConnectorConfig() HTTPConnectorConfig {
	return HTTPConnectorConfig{
		EndpointURL: "https://receiver.example/notify",
		Authentication: HTTPConnectorAuthentication{
			Mode: HTTPConnectorAuthNone,
		},
		FieldMappings:       []HTTPConnectorFieldMapping{{Source: HTTPConnectorFieldText, Target: "message"}},
		TimeoutMilliseconds: 5000,
	}
}
