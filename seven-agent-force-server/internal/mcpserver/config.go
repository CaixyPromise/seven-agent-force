package mcpserver

import (
	"errors"
	"fmt"
	"os"
	"strings"
	"time"

	"go.yaml.in/yaml/v3"
)

type RuntimeConfig struct {
	Server struct {
		ListenAddress       string   `yaml:"listenAddress"`
		EndpointPath        string   `yaml:"endpointPath"`
		AllowedOrigins      []string `yaml:"allowedOrigins"`
		AllowedHosts        []string `yaml:"allowedHosts"`
		ReadTimeoutSeconds  int      `yaml:"readTimeoutSeconds"`
		WriteTimeoutSeconds int      `yaml:"writeTimeoutSeconds"`
	} `yaml:"server"`
	Protocol struct {
		Version             string `yaml:"version"`
		MaxRequestBodyBytes int64  `yaml:"maxRequestBodyBytes"`
		JSONResponseOnly    bool   `yaml:"jsonResponseOnly"`
	} `yaml:"protocol"`
	OpenAPI struct {
		BaseURL        string `yaml:"baseURL"`
		TimeoutSeconds int    `yaml:"timeoutSeconds"`
	} `yaml:"openAPI"`
	Limits struct {
		IngressConcurrency        int   `yaml:"ingressConcurrency"`
		GlobalConcurrency         int   `yaml:"globalConcurrency"`
		PerApplicationConcurrency int   `yaml:"perApplicationConcurrency"`
		OutputTextLimitBytes      int64 `yaml:"outputTextLimitBytes"`
	} `yaml:"limits"`
}

func LoadRuntimeConfig(path string) (RuntimeConfig, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return RuntimeConfig{}, fmt.Errorf("read MCP configuration: %w", err)
	}
	config := defaultRuntimeConfig()
	decoder := yaml.NewDecoder(strings.NewReader(string(raw)))
	decoder.KnownFields(true)
	if err := decoder.Decode(&config); err != nil {
		return RuntimeConfig{}, fmt.Errorf("decode MCP configuration: %w", err)
	}
	if err := config.Validate(); err != nil {
		return RuntimeConfig{}, err
	}
	return config, nil
}

func defaultRuntimeConfig() RuntimeConfig {
	var config RuntimeConfig
	config.Server.ListenAddress = "127.0.0.1:18081"
	config.Server.EndpointPath = DefaultEndpoint
	config.Server.AllowedHosts = []string{"127.0.0.1", "localhost", "::1"}
	config.Server.ReadTimeoutSeconds = 10
	config.Server.WriteTimeoutSeconds = 60
	config.Protocol.Version = ProtocolVersion
	config.Protocol.MaxRequestBodyBytes = DefaultMaxBody
	config.Protocol.JSONResponseOnly = true
	config.OpenAPI.TimeoutSeconds = 15
	config.Limits.IngressConcurrency = 64
	config.Limits.GlobalConcurrency = 64
	config.Limits.PerApplicationConcurrency = 8
	config.Limits.OutputTextLimitBytes = DefaultMaxText
	return config
}

func (c RuntimeConfig) Validate() error {
	if strings.TrimSpace(c.Server.ListenAddress) == "" {
		return errors.New("MCP listen address is required")
	}
	if strings.TrimSpace(c.OpenAPI.BaseURL) == "" {
		return errors.New("Open API base URL is required")
	}
	if c.Server.ReadTimeoutSeconds <= 0 || c.Server.ReadTimeoutSeconds > 120 {
		return errors.New("MCP read timeout must be between 1 and 120 seconds")
	}
	if c.Server.WriteTimeoutSeconds <= 0 || c.Server.WriteTimeoutSeconds > 120 {
		return errors.New("MCP write timeout must be between 1 and 120 seconds")
	}
	if !c.Protocol.JSONResponseOnly {
		return errors.New("MCP JSON-only responses must be enabled")
	}
	if c.OpenAPI.TimeoutSeconds <= 0 || c.OpenAPI.TimeoutSeconds > 30 {
		return errors.New("Open API timeout must be between 1 and 30 seconds")
	}
	minimumWriteTimeout := 3*c.OpenAPI.TimeoutSeconds + 5
	if c.Server.WriteTimeoutSeconds < minimumWriteTimeout {
		return fmt.Errorf("MCP write timeout must be at least %d seconds for the configured Open API timeout", minimumWriteTimeout)
	}
	_, err := (HandlerConfig{
		ProtocolVersion: c.Protocol.Version, EndpointPath: c.Server.EndpointPath,
		AllowedOrigins: c.Server.AllowedOrigins, AllowedHosts: c.Server.AllowedHosts, MaxRequestBodyBytes: c.Protocol.MaxRequestBodyBytes,
		IngressConcurrency: c.Limits.IngressConcurrency, GlobalConcurrency: c.Limits.GlobalConcurrency, PerApplicationLimit: c.Limits.PerApplicationConcurrency,
		OutputTextLimitBytes: c.Limits.OutputTextLimitBytes,
	}).normalized()
	return err
}

func (c RuntimeConfig) HandlerConfig() HandlerConfig {
	return HandlerConfig{
		ProtocolVersion: c.Protocol.Version, EndpointPath: c.Server.EndpointPath,
		AllowedOrigins: c.Server.AllowedOrigins, AllowedHosts: c.Server.AllowedHosts, MaxRequestBodyBytes: c.Protocol.MaxRequestBodyBytes,
		IngressConcurrency: c.Limits.IngressConcurrency, GlobalConcurrency: c.Limits.GlobalConcurrency, PerApplicationLimit: c.Limits.PerApplicationConcurrency,
		OutputTextLimitBytes: c.Limits.OutputTextLimitBytes,
	}
}

func (c RuntimeConfig) GatewayConfig() HTTPGatewayConfig {
	return HTTPGatewayConfig{BaseURL: c.OpenAPI.BaseURL, Timeout: time.Duration(c.OpenAPI.TimeoutSeconds) * time.Second}
}
