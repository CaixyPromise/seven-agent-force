// Package config loads Seven Worker runtime configuration.
package config

import (
	"fmt"
	"net/url"
	"path/filepath"
	"strings"
	"time"
)

// Config is the narrow runtime contract consumed by the Worker and the
// Workforce module's infrastructure adapters. HTTP server ownership remains
// with the enclosing Framework process.
type Config struct {
	Database      DatabaseConfig      `mapstructure:"database"`
	RabbitMQ      RabbitMQConfig      `mapstructure:"rabbitmq"`
	Artifact      ArtifactConfig      `mapstructure:"artifact"`
	Worker        WorkerConfig        `mapstructure:"worker"`
	Harness       HarnessConfig       `mapstructure:"harness"`
	ProviderProxy ProviderProxyConfig `mapstructure:"providerProxy"`
	ProviderSSRF  ProviderSSRFConfig  `mapstructure:"providerSSRF"`
	ManagedHooks  ManagedHooksConfig  `mapstructure:"managedHooks"`
	Reviewer      ReviewerConfig      `mapstructure:"reviewer"`
}

type ManagedHooksConfig struct {
	Enabled   bool
	CacheRoot string
}

type DatabaseConfig struct {
	DSN            string        `mapstructure:"dsn"`
	MigrationsDir  string        `mapstructure:"migrationsDir"`
	MaxConns       int32         `mapstructure:"maxConns"`
	MinConns       int32         `mapstructure:"minConns"`
	ConnectTimeout time.Duration `mapstructure:"connectTimeout"`
}

type RabbitMQConfig struct {
	Enabled      bool          `mapstructure:"enabled"`
	Host         string        `mapstructure:"host"`
	Port         int           `mapstructure:"port"`
	Username     string        `mapstructure:"username"`
	Password     string        `mapstructure:"password"`
	VHost        string        `mapstructure:"vhost"`
	Prefetch     int           `mapstructure:"prefetch"`
	ReconnectMin time.Duration `mapstructure:"reconnectMin"`
	ReconnectMax time.Duration `mapstructure:"reconnectMax"`
	Declare      bool          `mapstructure:"declare"`
	TaskQueue    string        `mapstructure:"taskQueue"`
}

func (c RabbitMQConfig) URL() string {
	vhost := strings.TrimPrefix(strings.TrimSpace(c.VHost), "/")
	u := &url.URL{
		Scheme: "amqp",
		User:   url.UserPassword(c.Username, c.Password),
		Host:   fmt.Sprintf("%s:%d", c.Host, c.Port),
		Path:   "/" + vhost,
	}
	return u.String()
}

type ArtifactConfig struct {
	Endpoint  string `mapstructure:"endpoint"`
	AccessKey string `mapstructure:"accessKey"`
	SecretKey string `mapstructure:"secretKey"`
	Bucket    string `mapstructure:"bucket"`
	UseTLS    bool   `mapstructure:"useTLS"`
}

type WorkerConfig struct {
	ID                       int64         `mapstructure:"id"`
	ServerBaseURL            string        `mapstructure:"serverBaseURL"`
	TokenFile                string        `mapstructure:"tokenFile"`
	MaxActiveContainers      int           `mapstructure:"maxActiveContainers"`
	MaxActiveReviews         int           `mapstructure:"maxActiveReviews"`
	MaxTotalContainers       int           `mapstructure:"maxTotalContainers"`
	MaxActiveDeliveries      int           `mapstructure:"maxActiveDeliveries"`
	DockerHost               string        `mapstructure:"dockerHost"`
	WorkspaceRoot            string        `mapstructure:"workspaceRoot"`
	OperationDNSOverHTTPSURL string        `mapstructure:"operationDNSOverHTTPSURL"`
	PollInterval             time.Duration `mapstructure:"pollInterval"`
	ManagedImageDigests      []string      `mapstructure:"managedImageDigests"`
}

type HarnessConfig struct {
	Alias           string `mapstructure:"alias"`
	Image           string `mapstructure:"image"`
	Contract        string `mapstructure:"contract"`
	AllowLocalImage bool   `mapstructure:"allowLocalImage"`
}

type ProviderProxyConfig struct {
	Image       string `mapstructure:"image"`
	ImageDigest string `mapstructure:"imageDigest"`
	Version     string `mapstructure:"version"`
	ControlRoot string `mapstructure:"controlRoot"`
}

type ProviderSSRFConfig struct {
	Enabled                bool
	AllowedList            []ProviderSSRFAllowEntry
	LocalDevelopment       bool
	MaxRequestBytes        int64
	MaxResponseHeaderBytes int64
	RequestTimeout         time.Duration
	ProbeTimeout           time.Duration
	ResponseHeaderTimeout  time.Duration
	TLSHandshakeTimeout    time.Duration
}

type ProviderSSRFAllowEntry struct {
	Hostname      string
	CIDRs         []string
	Ports         []int
	AllowFakeIP   bool
	AllowLoopback bool
}

type ReviewerConfig struct {
	Enabled bool `mapstructure:"enabled"`
}

func (c Config) Validate() error {
	checks := []struct {
		ok      bool
		message string
	}{
		{c.Worker.ID > 0, "worker.id is required"},
		{strings.TrimSpace(c.Worker.ServerBaseURL) != "", "worker.serverBaseURL is required"},
		{strings.TrimSpace(c.Worker.TokenFile) != "", "worker.tokenFile is required"},
		{c.Worker.MaxActiveContainers >= 1, "worker.maxActiveContainers must be at least 1"},
		{c.Worker.MaxActiveReviews >= 1 && c.Worker.MaxActiveReviews <= 64, "worker.maxActiveReviews must be between 1 and 64"},
		{c.Worker.MaxTotalContainers >= RequiredContainerBudget(c.Worker.MaxActiveContainers, c.Worker.MaxActiveReviews, c.Worker.MaxActiveDeliveries) && c.Worker.MaxTotalContainers <= 128, fmt.Sprintf("worker.maxTotalContainers=%d is insufficient or exceeds 128: require at least %d (3*agents + 2*reviews + 2*effectiveDeliveries; delivery 0 means 1), with maximum 128", c.Worker.MaxTotalContainers, RequiredContainerBudget(c.Worker.MaxActiveContainers, c.Worker.MaxActiveReviews, c.Worker.MaxActiveDeliveries))},
		{c.Worker.MaxActiveDeliveries >= 0 && c.Worker.MaxActiveDeliveries <= 8, "worker.maxActiveDeliveries must be between 0 and 8"},
		{validWorkspaceRoot(c.Worker.WorkspaceRoot), "worker.workspaceRoot must be an absolute non-root path"},
		{validOptionalHTTPSURL(c.Worker.OperationDNSOverHTTPSURL), "worker.operationDNSOverHTTPSURL must be an absolute https URL without credentials or fragment"},
		{strings.TrimSpace(c.Harness.Alias) != "", "harness.alias is required"},
		{strings.TrimSpace(c.Harness.Image) != "", "harness.image is required"},
		{c.Harness.Contract == "seven.worker-image/v1", "harness.contract must be seven.worker-image/v1"},
		{!c.ManagedHooks.Enabled || strings.TrimSpace(c.ManagedHooks.CacheRoot) != "", "managedHooks.cacheRoot is required when managed Hooks are enabled"},
	}
	if strings.TrimSpace(c.ProviderProxy.Image) != "" && strings.TrimSpace(c.ProviderProxy.ControlRoot) == "" {
		return fmt.Errorf("providerProxy.controlRoot is required when providerProxy.image is configured")
	}
	if strings.TrimSpace(c.ProviderProxy.Image) != "" && (strings.TrimSpace(c.ProviderProxy.ImageDigest) == "" || strings.TrimSpace(c.ProviderProxy.Version) == "") {
		return fmt.Errorf("providerProxy.imageDigest and providerProxy.version are required when providerProxy.image is configured")
	}
	if strings.TrimSpace(c.ProviderProxy.Image) != "" && !exactImageReference(c.ProviderProxy.Image, c.ProviderProxy.ImageDigest) {
		return fmt.Errorf("providerProxy.image must reference the configured exact sha256 digest")
	}
	if c.ProviderSSRF.MaxRequestBytes < 1 || c.ProviderSSRF.MaxResponseHeaderBytes < 1 || c.ProviderSSRF.RequestTimeout <= 0 || c.ProviderSSRF.ProbeTimeout <= 0 || c.ProviderSSRF.ResponseHeaderTimeout <= 0 || c.ProviderSSRF.TLSHandshakeTimeout <= 0 {
		return fmt.Errorf("providerSSRF limits and timeouts must be positive")
	}
	for _, entry := range c.ProviderSSRF.AllowedList {
		if entry.AllowFakeIP && !c.ProviderSSRF.LocalDevelopment {
			return fmt.Errorf("providerSSRF allowFakeIP for %q requires local development mode", entry.Hostname)
		}
		if entry.AllowLoopback && !c.ProviderSSRF.LocalDevelopment {
			return fmt.Errorf("providerSSRF allowLoopback for %q requires local development mode", entry.Hostname)
		}
	}
	for _, check := range checks {
		if !check.ok {
			return fmt.Errorf("%s", check.message)
		}
	}
	if c.RabbitMQ.Enabled {
		if c.RabbitMQ.Host == "" || c.RabbitMQ.Port < 1 || c.RabbitMQ.Username == "" {
			return fmt.Errorf("rabbitmq host, port, and username are required when enabled")
		}
		if c.RabbitMQ.Prefetch < 1 {
			return fmt.Errorf("rabbitmq.prefetch must be at least 1")
		}
	}
	return nil
}

func validOptionalHTTPSURL(raw string) bool {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return true
	}
	parsed, err := url.Parse(raw)
	return err == nil && parsed.Scheme == "https" && parsed.Hostname() != "" && parsed.User == nil && parsed.Fragment == ""
}

// Filesystem ownership, writability and free-space checks run in the Worker
// process, which is the only process authorized to use this host directory.
// Configuration validation still rejects ambiguous relative/root paths before
// the daemon can acquire work.
func validWorkspaceRoot(value string) bool {
	path := strings.TrimSpace(value)
	if path == "" || !filepath.IsAbs(path) {
		return false
	}
	path = filepath.Clean(path)
	volume := filepath.VolumeName(path)
	if volume != "" {
		return path != filepath.Clean(volume+string(filepath.Separator))
	}
	return path != string(filepath.Separator)
}

func exactImageReference(image, digest string) bool {
	image = strings.TrimSpace(image)
	digest = strings.TrimSpace(digest)
	if len(digest) != len("sha256:")+64 || !strings.HasPrefix(digest, "sha256:") {
		return false
	}
	for _, character := range digest[len("sha256:"):] {
		if !((character >= '0' && character <= '9') || (character >= 'a' && character <= 'f')) {
			return false
		}
	}
	return image != "" && strings.HasSuffix(image, "@"+digest)
}
