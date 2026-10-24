package config

import (
	"fmt"
	"net/url"
	"path/filepath"
	"strings"

	"github.com/go-viper/mapstructure/v2"
	"github.com/spf13/viper"
)

// LoadWorker reads only the Worker data-plane configuration. It deliberately
// does not decode datasource, Framework security keys, browser auth, or the
// Provider credential store into the seven-worker process.
func LoadWorker(configDir string) (Config, error) {
	loader := viper.New()
	loader.SetConfigFile(filepath.Join(configDir, "application.yaml"))
	loader.SetConfigType("yaml")
	if err := loader.ReadInConfig(); err != nil {
		return Config{}, err
	}
	var raw struct {
		RabbitMQ RabbitMQConfig `mapstructure:"rabbitmq"`
		Platform struct {
			Mode string `mapstructure:"mode"`
		} `mapstructure:"platform"`
		Workforce struct {
			Enabled       bool                `mapstructure:"enabled"`
			Artifact      ArtifactConfig      `mapstructure:"artifact"`
			Worker        WorkerConfig        `mapstructure:"worker"`
			Harness       HarnessConfig       `mapstructure:"harness"`
			ProviderProxy ProviderProxyConfig `mapstructure:"providerProxy"`
			ProviderSSRF  ProviderSSRFConfig  `mapstructure:"providerSSRF"`
			ManagedHooks  ManagedHooksConfig  `mapstructure:"managedHooks"`
			Reviewer      ReviewerConfig      `mapstructure:"reviewer"`
		} `mapstructure:"workforce"`
	}
	if err := loader.Unmarshal(&raw, viper.DecodeHook(mapstructure.StringToTimeDurationHookFunc())); err != nil {
		return Config{}, err
	}
	if !raw.Workforce.Enabled {
		return Config{}, fmt.Errorf("workforce.enabled must be true")
	}
	if raw.RabbitMQ.URL() == "" {
		return Config{}, fmt.Errorf("rabbitmq configuration is invalid")
	}
	if configuredURL := strings.TrimSpace(loader.GetString("rabbitmq.url")); configuredURL != "" {
		parsed, err := url.Parse(configuredURL)
		if err != nil {
			return Config{}, fmt.Errorf("parse rabbitmq.url: %w", err)
		}
		raw.RabbitMQ.Host = parsed.Hostname()
		if parsed.Port() != "" {
			_, _ = fmt.Sscanf(parsed.Port(), "%d", &raw.RabbitMQ.Port)
		}
		if parsed.User != nil {
			raw.RabbitMQ.Username = parsed.User.Username()
			raw.RabbitMQ.Password, _ = parsed.User.Password()
		}
		raw.RabbitMQ.VHost = parsed.Path
	}
	raw.Workforce.ProviderSSRF.LocalDevelopment = strings.EqualFold(raw.Platform.Mode, "local")
	raw.RabbitMQ.Prefetch = 1
	raw.RabbitMQ.Declare = false
	raw.RabbitMQ.TaskQueue = ""
	// Review consumption remains explicitly disabled by default, but a new
	// Worker still reports a bounded review capacity when enabled. Older local
	// configs keep their existing Agent capacity without an implicit schema
	// migration requirement.
	if raw.Workforce.Worker.MaxActiveReviews == 0 {
		raw.Workforce.Worker.MaxActiveReviews = 1
	}
	if !loader.IsSet("workforce.worker.maxTotalContainers") {
		raw.Workforce.Worker.MaxTotalContainers = RequiredContainerBudget(raw.Workforce.Worker.MaxActiveContainers, raw.Workforce.Worker.MaxActiveReviews, raw.Workforce.Worker.MaxActiveDeliveries)
	}
	return Config{
		RabbitMQ: raw.RabbitMQ, Artifact: raw.Workforce.Artifact,
		Worker: raw.Workforce.Worker, Harness: raw.Workforce.Harness,
		ProviderProxy: raw.Workforce.ProviderProxy, ProviderSSRF: raw.Workforce.ProviderSSRF,
		ManagedHooks: raw.Workforce.ManagedHooks,
		Reviewer:     raw.Workforce.Reviewer,
	}, nil
}
