package workforce

import (
	"fmt"
	"net/url"
	"strconv"
	"strings"
	"time"

	workforceconfig "github.com/CaixyPromise/seven-agent/force/internal/app/workforce/runtimeconfig"
	sharedconfig "github.com/CaixyPromise/seven-agent/force/internal/shared/config"
)

// RuntimeConfig projects the Framework configuration into the smaller host
// Worker interface. It is the single mapping seam shared by the in-process
// control plane and the separate seven-worker daemon.
func RuntimeConfig(cfg sharedconfig.Config, migrationsDir string) workforceconfig.Config {
	rabbit := cfg.RabbitMQ
	if strings.TrimSpace(rabbit.URL) != "" {
		if parsed, err := url.Parse(rabbit.URL); err == nil {
			if parsed.Hostname() != "" {
				rabbit.Host = parsed.Hostname()
			}
			if parsed.Port() != "" {
				_, _ = fmt.Sscanf(parsed.Port(), "%d", &rabbit.Port)
			}
			if parsed.User != nil {
				rabbit.Username = parsed.User.Username()
				rabbit.Password, _ = parsed.User.Password()
			}
			if parsed.Path != "" {
				rabbit.VHost = parsed.Path
			}
		}
	}
	providerProxyControlRoot := strings.TrimSpace(cfg.Workforce.ProviderProxy.ControlRoot)
	if providerProxyControlRoot == "" {
		providerProxyControlRoot = "/tmp/seven-workforce/provider-proxy"
	}
	return workforceconfig.Config{
		Database: workforceconfig.DatabaseConfig{
			DSN: cfg.Datasource.Postgres.DSN, MigrationsDir: migrationsDir,
			MaxConns: int32(cfg.Datasource.Postgres.MaxOpenConns), MinConns: 1, ConnectTimeout: 5 * time.Second,
		},
		RabbitMQ: workforceconfig.RabbitMQConfig{
			Enabled: rabbit.Enabled, Host: rabbit.Host, Port: rabbit.Port, Username: rabbit.Username, Password: rabbit.Password,
			VHost: rabbit.VHost, Prefetch: rabbit.Prefetch, ReconnectMin: rabbit.ReconnectMin, ReconnectMax: rabbit.ReconnectMax,
			Declare: rabbit.Declare,
		},
		Artifact: workforceconfig.ArtifactConfig{
			Endpoint: cfg.Workforce.Artifact.Endpoint, AccessKey: cfg.Workforce.Artifact.AccessKey,
			SecretKey: cfg.Workforce.Artifact.SecretKey, Bucket: cfg.Workforce.Artifact.Bucket, UseTLS: cfg.Workforce.Artifact.UseTLS,
		},
		Worker: workforceconfig.WorkerConfig{
			ID: parseConfiguredWorkerID(cfg.Workforce.Worker.ID), MaxActiveContainers: cfg.Workforce.Worker.MaxActiveContainers,
			MaxActiveReviews: 1, MaxTotalContainers: workforceconfig.RequiredContainerBudget(cfg.Workforce.Worker.MaxActiveContainers, 1, 0),
			ServerBaseURL: cfg.Workforce.Worker.ServerBaseURL, TokenFile: cfg.Workforce.Worker.TokenFile,
			DockerHost: cfg.Workforce.Worker.DockerHost, WorkspaceRoot: cfg.Workforce.Worker.WorkspaceRoot,
			PollInterval: cfg.Workforce.Worker.PollInterval,
		},
		Harness: workforceconfig.HarnessConfig{
			Alias: cfg.Workforce.Harness.Alias, Image: cfg.Workforce.Harness.Image,
			Contract: cfg.Workforce.Harness.Contract, AllowLocalImage: cfg.Workforce.Harness.AllowLocalImage,
		},
		ProviderProxy: workforceconfig.ProviderProxyConfig{
			Image: cfg.Workforce.ProviderProxy.Image, ImageDigest: cfg.Workforce.ProviderProxy.ImageDigest,
			Version: cfg.Workforce.ProviderProxy.Version, ControlRoot: providerProxyControlRoot,
		},
		ProviderSSRF: providerSSRFConfig(cfg),
		ManagedHooks: workforceconfig.ManagedHooksConfig{Enabled: cfg.Workforce.ManagedHooks.Enabled, CacheRoot: cfg.Workforce.ManagedHooks.CacheRoot},
		Reviewer:     workforceconfig.ReviewerConfig{Enabled: cfg.Workforce.Reviewer.Enabled},
	}
}

func parseConfiguredWorkerID(raw string) int64 {
	id, err := strconv.ParseInt(strings.TrimSpace(raw), 10, 64)
	if err != nil || id <= 0 {
		return 0
	}
	return id
}

func providerSSRFConfig(cfg sharedconfig.Config) workforceconfig.ProviderSSRFConfig {
	entries := make([]workforceconfig.ProviderSSRFAllowEntry, 0, len(cfg.Workforce.ProviderSSRF.AllowedList))
	for _, entry := range cfg.Workforce.ProviderSSRF.AllowedList {
		entries = append(entries, workforceconfig.ProviderSSRFAllowEntry{
			Hostname: entry.Hostname, CIDRs: append([]string(nil), entry.CIDRs...), Ports: append([]int(nil), entry.Ports...), AllowFakeIP: entry.AllowFakeIP, AllowLoopback: entry.AllowLoopback,
		})
	}
	return workforceconfig.ProviderSSRFConfig{
		Enabled: cfg.Workforce.ProviderSSRF.Enabled, AllowedList: entries,
		LocalDevelopment: cfg.Platform.Mode == sharedconfig.PlatformModeLocal,
		MaxRequestBytes:  cfg.Workforce.ProviderSSRF.MaxRequestBytes, MaxResponseHeaderBytes: cfg.Workforce.ProviderSSRF.MaxResponseHeaderBytes,
		RequestTimeout: cfg.Workforce.ProviderSSRF.RequestTimeout, ProbeTimeout: cfg.Workforce.ProviderSSRF.ProbeTimeout, ResponseHeaderTimeout: cfg.Workforce.ProviderSSRF.ResponseHeaderTimeout,
		TLSHandshakeTimeout: cfg.Workforce.ProviderSSRF.TLSHandshakeTimeout,
	}
}
