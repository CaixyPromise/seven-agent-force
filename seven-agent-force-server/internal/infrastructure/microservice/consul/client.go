package consul

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/CaixyPromise/seven-agent/force/internal/infrastructure/microservice"
)

const maxConsulResponseBytes int64 = 4 << 20

const defaultConsulTimeout = 2 * time.Second

type ClientOptions struct {
	Address    string
	Token      string
	HTTPClient *http.Client
	Timeout    time.Duration
}

type Client struct {
	baseURL    *url.URL
	token      string
	httpClient *http.Client
}

func NewClient(options ClientOptions) (*Client, error) {
	baseURL, err := url.Parse(options.Address)
	if err != nil || (baseURL.Scheme != "http" && baseURL.Scheme != "https") || baseURL.Host == "" || baseURL.User != nil || baseURL.RawQuery != "" || baseURL.Fragment != "" {
		return nil, fmt.Errorf("invalid Consul address")
	}
	baseURL.Path = strings.TrimRight(baseURL.Path, "/")
	timeout := options.Timeout
	if timeout <= 0 {
		timeout = defaultConsulTimeout
	}
	var httpClient *http.Client
	if options.HTTPClient == nil {
		httpClient = &http.Client{Timeout: timeout, Transport: &http.Transport{Proxy: nil}}
	} else {
		httpClientCopy := *options.HTTPClient
		httpClientCopy.Timeout = timeout
		httpClient = &httpClientCopy
	}
	httpClient.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
	return &Client{baseURL: baseURL, token: options.Token, httpClient: httpClient}, nil
}

func (c *Client) doJSON(ctx context.Context, method, endpoint, rawPath string, query url.Values, requestBody, responseBody any) error {
	if c == nil || c.baseURL == nil || c.httpClient == nil {
		return microservice.ErrInvalidDependency
	}
	if ctx == nil {
		return microservice.ErrInvalidContext
	}
	var body io.Reader
	if requestBody != nil {
		encoded, err := json.Marshal(requestBody)
		if err != nil {
			return fmt.Errorf("encode Consul request: %w", err)
		}
		body = bytes.NewReader(encoded)
	}
	u := *c.baseURL
	u.Path = c.baseURL.Path + endpoint
	if rawPath != "" {
		u.RawPath = c.baseURL.EscapedPath() + rawPath
	}
	u.RawQuery = query.Encode()
	request, err := http.NewRequestWithContext(ctx, method, u.String(), body)
	if err != nil {
		return fmt.Errorf("build Consul request: %w", err)
	}
	request.Header.Set("Accept", "application/json")
	if requestBody != nil {
		request.Header.Set("Content-Type", "application/json")
	}
	if c.token != "" {
		request.Header.Set("X-Consul-Token", c.token)
	}
	response, err := c.httpClient.Do(request)
	if err != nil {
		return fmt.Errorf("Consul request failed: %w", err)
	}
	defer response.Body.Close()
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		_, _ = io.Copy(io.Discard, io.LimitReader(response.Body, 32<<10))
		return fmt.Errorf("Consul request returned HTTP %d", response.StatusCode)
	}
	if responseBody == nil {
		_, _ = io.Copy(io.Discard, response.Body)
		return nil
	}
	limited := io.LimitReader(response.Body, maxConsulResponseBytes+1)
	data, err := io.ReadAll(limited)
	if err != nil {
		_, _ = io.Copy(io.Discard, response.Body)
		return fmt.Errorf("read Consul response: %w", err)
	}
	if int64(len(data)) > maxConsulResponseBytes {
		_, _ = io.Copy(io.Discard, response.Body)
		return fmt.Errorf("Consul response exceeds limit")
	}
	if err := json.Unmarshal(data, responseBody); err != nil {
		return fmt.Errorf("decode Consul response: %w", err)
	}
	return nil
}
