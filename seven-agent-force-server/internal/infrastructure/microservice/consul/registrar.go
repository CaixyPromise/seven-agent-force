package consul

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"github.com/CaixyPromise/seven-agent/force/internal/infrastructure/microservice"
)

type Registrar struct {
	client *Client
}

func NewRegistrar(client *Client) *Registrar {
	return &Registrar{client: client}
}

type registrationPayload struct {
	ID      string            `json:"ID"`
	Name    string            `json:"Name"`
	Address string            `json:"Address,omitempty"`
	Port    int               `json:"Port"`
	Tags    []string          `json:"Tags,omitempty"`
	Meta    map[string]string `json:"Meta,omitempty"`
	Check   healthCheck       `json:"Check"`
}

type healthCheck struct {
	HTTP     string `json:"HTTP"`
	Interval string `json:"Interval"`
	Timeout  string `json:"Timeout"`
}

func (r *Registrar) Register(ctx context.Context, registration microservice.ServiceRegistration) error {
	if r == nil || r.client == nil {
		return microservice.ErrInvalidDependency
	}
	if ctx == nil {
		return microservice.ErrInvalidContext
	}
	if registration.ID == "" || registration.ServiceName == "" || registration.Port < 1 || registration.Port > 65535 {
		return fmt.Errorf("invalid service registration")
	}
	if registration.HealthInterval <= 0 || registration.HealthTimeout <= 0 {
		return fmt.Errorf("health interval and timeout must be positive")
	}
	scheme := strings.ToLower(registration.Scheme)
	if scheme == "" {
		scheme = "http"
	}
	if scheme != "http" && scheme != "https" {
		return fmt.Errorf("invalid service registration scheme")
	}
	healthPath := registration.HealthCheckPath
	if healthPath == "" {
		healthPath = "/internal/healthz"
	}
	if !strings.HasPrefix(healthPath, "/") || strings.HasPrefix(healthPath, "//") {
		return fmt.Errorf("invalid health check path")
	}
	healthHost := registration.Address
	if healthHost == "" {
		healthHost = "127.0.0.1"
	}
	payload := registrationPayload{
		ID: registration.ID, Name: registration.ServiceName, Address: registration.Address,
		Port: registration.Port, Tags: registration.Tags, Meta: registration.Metadata,
		Check: healthCheck{
			HTTP:     (&url.URL{Scheme: scheme, Host: net.JoinHostPort(healthHost, strconv.Itoa(registration.Port)), Path: healthPath}).String(),
			Interval: registration.HealthInterval.String(), Timeout: registration.HealthTimeout.String(),
		},
	}
	return r.client.doJSON(ctx, http.MethodPut, "/v1/agent/service/register", "", nil, payload, nil)
}

func (r *Registrar) Deregister(ctx context.Context, instanceID string) error {
	if r == nil || r.client == nil {
		return microservice.ErrInvalidDependency
	}
	if ctx == nil {
		return microservice.ErrInvalidContext
	}
	if instanceID == "" {
		return fmt.Errorf("instance ID is required")
	}
	escaped := url.PathEscape(instanceID)
	endpoint := "/v1/agent/service/deregister/" + instanceID
	rawPath := "/v1/agent/service/deregister/" + escaped
	return r.client.doJSON(ctx, http.MethodPut, endpoint, rawPath, nil, nil, nil)
}
