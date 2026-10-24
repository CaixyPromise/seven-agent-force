package infrastructure

import (
	"bytes"
	"context"
	"net/http"
	"net/url"
	"strings"

	providerdomain "github.com/CaixyPromise/seven-agent/force/internal/app/workforce/provider/domain"
	"github.com/CaixyPromise/seven-agent/force/internal/app/workforce/provider/egress"
	invocationapp "github.com/CaixyPromise/seven-agent/force/internal/app/workforce/providerinvocation/application"
	invocationdomain "github.com/CaixyPromise/seven-agent/force/internal/app/workforce/providerinvocation/domain"
	workforceconfig "github.com/CaixyPromise/seven-agent/force/internal/app/workforce/runtimeconfig"
	"github.com/CaixyPromise/seven-agent/force/internal/infrastructure/security/outboundurl"
)

type ResponsesAdapter struct {
	client   *http.Client
	guard    *outboundurl.OutboundURLGuard
	resolver *egress.Resolver
	config   workforceconfig.ProviderSSRFConfig
}

func NewResponsesAdapter(config workforceconfig.ProviderSSRFConfig) (*ResponsesAdapter, error) {
	resolver, err := egress.NewResolver(config)
	if err != nil {
		return nil, err
	}
	return &ResponsesAdapter{guard: outboundurl.NewOutboundURLGuard(outboundurl.Options{}), resolver: resolver, config: config}, nil
}

func NewResponsesAdapterWithClient(client *http.Client) *ResponsesAdapter {
	return &ResponsesAdapter{client: client}
}

func (a *ResponsesAdapter) Invoke(ctx context.Context, credential providerdomain.InvocationCredential, operation invocationdomain.Operation, invocationID string, body []byte, sourceHeader http.Header, stream bool) (invocationapp.ProviderResponse, error) {
	path := "/responses"
	switch operation {
	case invocationdomain.OperationResponsesCompact:
		path = "/responses/compact"
	case invocationdomain.OperationMessagesCreate:
		path = "/messages"
	case invocationdomain.OperationMessagesCountTokens:
		path = "/messages/count_tokens"
	}
	base, err := url.Parse(strings.TrimRight(strings.TrimSpace(credential.BaseURL), "/"))
	if err != nil || base.Scheme == "" || base.Host == "" {
		return invocationapp.ProviderResponse{}, invocationError("PROVIDER_URL_INVALID", http.StatusBadGateway, "Provider base URL is invalid", err)
	}
	base.Path = strings.TrimRight(base.Path, "/") + path
	base.RawQuery, base.Fragment = "", ""
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, base.String(), bytes.NewReader(body))
	if err != nil {
		return invocationapp.ProviderResponse{}, invocationError("PROVIDER_REQUEST_BUILD_FAILED", http.StatusBadGateway, "Provider request could not be built", err)
	}
	// The runtime's placeholder Authorization/x-api-key headers never reach the
	// upstream. Only the task-bound credential is injected here.
	if credential.Protocol == providerdomain.ProtocolAnthropic {
		request.Header.Set("x-api-key", credential.APIKey)
		version := strings.TrimSpace(sourceHeader.Get("anthropic-version"))
		if version == "" {
			version = "2023-06-01"
		}
		request.Header.Set("anthropic-version", version)
		if beta := strings.TrimSpace(sourceHeader.Get("anthropic-beta")); beta != "" {
			request.Header.Set("anthropic-beta", beta)
		}
	} else {
		request.Header.Set("Authorization", "Bearer "+credential.APIKey)
	}
	request.Header.Set("Content-Type", "application/json")
	if stream {
		request.Header.Set("Accept", "text/event-stream")
	} else {
		request.Header.Set("Accept", "application/json")
	}
	request.Header.Set("Idempotency-Key", invocationID)
	client := a.client
	if client == nil {
		if a.guard == nil || a.resolver == nil {
			return invocationapp.ProviderResponse{}, invocationError("PROVIDER_ADAPTER_NOT_CONFIGURED", http.StatusServiceUnavailable, "Provider Adapter is not configured", nil)
		}
		policy, resolveErr := a.resolver.Resolve(credential.BaseURL)
		if resolveErr != nil {
			return invocationapp.ProviderResponse{}, invocationError("PROVIDER_EGRESS_DENIED", http.StatusForbidden, "Provider destination is denied by egress policy", resolveErr)
		}
		client = a.guard.HTTPClientWithOptions(policy, outboundurl.HTTPClientOptions{
			TLSHandshakeTimeout:    a.config.TLSHandshakeTimeout,
			ResponseHeaderTimeout:  a.config.ResponseHeaderTimeout,
			MaxResponseHeaderBytes: a.config.MaxResponseHeaderBytes,
		})
	}
	response, err := client.Do(request)
	if err != nil {
		return invocationapp.ProviderResponse{}, invocationError("PROVIDER_UPSTREAM_UNAVAILABLE", http.StatusBadGateway, "Provider upstream request failed", err)
	}
	return invocationapp.ProviderResponse{
		StatusCode: response.StatusCode, Header: allowResponseHeaders(response.Header), Body: response.Body,
		UpstreamRequestID: firstHeader(response.Header, "X-Request-Id", "Request-Id", "OpenAI-Request-Id"),
	}, nil
}

func allowResponseHeaders(source http.Header) http.Header {
	result := make(http.Header)
	for _, name := range []string{"Content-Type", "Cache-Control", "X-Request-Id", "Request-Id", "OpenAI-Request-Id", "Retry-After"} {
		for _, value := range source.Values(name) {
			result.Add(name, value)
		}
	}
	return result
}

func firstHeader(header http.Header, names ...string) string {
	for _, name := range names {
		if value := strings.TrimSpace(header.Get(name)); value != "" {
			return value
		}
	}
	return ""
}
