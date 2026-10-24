package consul

import (
	"context"
	"net/http"
	"net/url"
	"strings"

	"github.com/CaixyPromise/seven-agent/force/internal/infrastructure/microservice"
)

type ResolverOptions struct {
	Datacenter string
	Tags       []string
	Fallback   microservice.ServiceResolver
}

type Resolver struct {
	client  *Client
	options ResolverOptions
}

func NewResolver(client *Client, options ResolverOptions) *Resolver {
	return &Resolver{client: client, options: options}
}

type healthServiceEntry struct {
	Node struct {
		Address string `json:"Address"`
	} `json:"Node"`
	Service struct {
		ID      string            `json:"ID"`
		Service string            `json:"Service"`
		Address string            `json:"Address"`
		Port    int               `json:"Port"`
		Tags    []string          `json:"Tags"`
		Meta    map[string]string `json:"Meta"`
	} `json:"Service"`
	Checks []struct {
		Status string `json:"Status"`
	} `json:"Checks"`
}

func (r *Resolver) Resolve(ctx context.Context, serviceName string) ([]microservice.ServiceInstance, error) {
	if r == nil || r.client == nil {
		return nil, microservice.ErrInvalidDependency
	}
	if ctx == nil {
		return nil, microservice.ErrInvalidContext
	}
	if serviceName == "" {
		return nil, microservice.ErrInvalidRequest
	}
	query := url.Values{"passing": {"true"}}
	if r.options.Datacenter != "" {
		query.Set("dc", r.options.Datacenter)
	}
	for _, tag := range r.options.Tags {
		query.Add("tag", tag)
	}
	var entries []healthServiceEntry
	endpoint := "/v1/health/service/" + serviceName
	rawPath := "/v1/health/service/" + url.PathEscape(serviceName)
	err := r.client.doJSON(ctx, http.MethodGet, endpoint, rawPath, query, nil, &entries)
	if err != nil {
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		if r.options.Fallback != nil {
			return r.options.Fallback.Resolve(ctx, serviceName)
		}
		return nil, err
	}
	instances := make([]microservice.ServiceInstance, 0, len(entries))
	for _, entry := range entries {
		if !allChecksPassing(entry.Checks) || entry.Service.Port < 1 || entry.Service.Port > 65535 {
			continue
		}
		host := entry.Service.Address
		if host == "" {
			host = entry.Node.Address
		}
		if host == "" || entry.Service.ID == "" {
			continue
		}
		scheme := strings.ToLower(entry.Service.Meta["protocol"])
		if scheme == "" {
			scheme = "http"
		}
		if scheme != "http" && scheme != "https" {
			continue
		}
		name := entry.Service.Service
		if name == "" {
			name = serviceName
		}
		instances = append(instances, microservice.ServiceInstance{
			ID: entry.Service.ID, ServiceName: name, Host: host, Port: entry.Service.Port,
			Scheme: scheme, Healthy: true, Tags: entry.Service.Tags, Metadata: entry.Service.Meta,
		})
	}
	if len(instances) == 0 {
		return nil, microservice.ErrNoHealthyInstance
	}
	return instances, nil
}

func allChecksPassing(checks []struct {
	Status string `json:"Status"`
}) bool {
	if len(checks) == 0 {
		return false
	}
	for _, check := range checks {
		if check.Status != "passing" {
			return false
		}
	}
	return true
}
