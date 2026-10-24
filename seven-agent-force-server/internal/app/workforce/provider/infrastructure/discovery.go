package infrastructure

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/CaixyPromise/seven-agent/force/internal/app/workforce/provider/domain"
	"github.com/CaixyPromise/seven-agent/force/internal/app/workforce/provider/egress"
	workforceconfig "github.com/CaixyPromise/seven-agent/force/internal/app/workforce/runtimeconfig"
	"github.com/CaixyPromise/seven-agent/force/internal/infrastructure/security/outboundurl"
)

var (
	ErrDiscoveryUnavailable    = errors.New("MODEL_DISCOVERY_UNAVAILABLE")
	ErrDiscoveryAuthentication = errors.New("MODEL_DISCOVERY_AUTHENTICATION_FAILED")
)

type ModelDiscovery struct {
	client   *http.Client
	guard    *outboundurl.OutboundURLGuard
	resolver *egress.Resolver
	config   workforceconfig.ProviderSSRFConfig
}

func NewModelDiscovery(config workforceconfig.ProviderSSRFConfig) (*ModelDiscovery, error) {
	resolver, err := egress.NewResolver(config)
	if err != nil {
		return nil, err
	}
	return &ModelDiscovery{guard: outboundurl.NewOutboundURLGuard(outboundurl.Options{}), resolver: resolver, config: config}, nil
}

func NewModelDiscoveryWithClient(client *http.Client) *ModelDiscovery {
	return &ModelDiscovery{client: client}
}

func (d *ModelDiscovery) Discover(ctx context.Context, credential domain.RuntimeCredential) ([]string, error) {
	client := d.client
	if client == nil {
		policy, err := d.resolver.Resolve(credential.BaseURL)
		if err != nil {
			return nil, ErrDiscoveryUnavailable
		}
		client = d.guard.HTTPClientWithOptions(policy, outboundurl.HTTPClientOptions{TLSHandshakeTimeout: d.config.TLSHandshakeTimeout, ResponseHeaderTimeout: d.config.ResponseHeaderTimeout, MaxResponseHeaderBytes: d.config.MaxResponseHeaderBytes})
	}
	discoveryCtx, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()
	next := strings.TrimRight(credential.BaseURL, "/") + "/models"
	models := make([]string, 0)
	for next != "" {
		request, err := http.NewRequestWithContext(discoveryCtx, http.MethodGet, next, nil)
		if err != nil {
			return nil, ErrDiscoveryUnavailable
		}
		if credential.Protocol == domain.ProtocolAnthropic {
			request.Header.Set("x-api-key", credential.APIKey)
			request.Header.Set("anthropic-version", "2023-06-01")
		} else {
			request.Header.Set("Authorization", "Bearer "+credential.APIKey)
		}
		response, err := client.Do(request)
		if err != nil {
			return nil, ErrDiscoveryUnavailable
		}
		body, readErr := io.ReadAll(io.LimitReader(response.Body, (1<<20)+1))
		response.Body.Close()
		if readErr != nil || len(body) > 1<<20 {
			return nil, ErrDiscoveryUnavailable
		}
		if response.StatusCode == http.StatusUnauthorized || response.StatusCode == http.StatusForbidden {
			return nil, ErrDiscoveryAuthentication
		}
		if response.StatusCode < 200 || response.StatusCode >= 300 {
			return nil, ErrDiscoveryUnavailable
		}
		var payload struct {
			Data []struct {
				ID string `json:"id"`
			} `json:"data"`
			HasMore bool   `json:"has_more"`
			LastID  string `json:"last_id"`
		}
		if json.Unmarshal(body, &payload) != nil {
			return nil, ErrDiscoveryUnavailable
		}
		for _, item := range payload.Data {
			models = append(models, item.ID)
		}
		if len(models) > 5000 {
			return nil, ErrDiscoveryUnavailable
		}
		next = ""
		if credential.Protocol == domain.ProtocolAnthropic && payload.HasMore {
			if payload.LastID == "" {
				return nil, ErrDiscoveryUnavailable
			}
			parsed, err := url.Parse(strings.TrimRight(credential.BaseURL, "/") + "/models")
			if err != nil {
				return nil, ErrDiscoveryUnavailable
			}
			query := parsed.Query()
			query.Set("after_id", payload.LastID)
			query.Set("limit", "1000")
			parsed.RawQuery = query.Encode()
			next = parsed.String()
		}
	}
	return models, nil
}

func (d *ModelDiscovery) String() string { return fmt.Sprintf("model-discovery(%p)", d) }
