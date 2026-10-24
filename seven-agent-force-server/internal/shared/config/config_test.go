package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/go-viper/mapstructure/v2"
	"github.com/spf13/viper"
)

func TestLoadMergesProfileAndEnvironment(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "application.yaml"), `
seven:
  name: base-name
server:
  host: 127.0.0.1
  port: 7777
datasource:
  driver: postgres
  bootstrap:
    enabled: true
    mode: both
    migrationsDir: migrations/custom
    versionTable: goose_custom
    changeOwner: goose
    baselineVersion: "20260422000000"
    allowLegacySync: true
  mysql:
    enabled: true
    dsn: user:pass@tcp(127.0.0.1:3306)/seven?parseTime=true
    maxOpenConns: 30
    maxIdleConns: 7
    connMaxLifetime: 45m
    connMaxIdleTime: 15m
  postgres:
    enabled: true
    dsn: postgres://postgres:postgres@127.0.0.1:5432/seven?sslmode=disable
    maxOpenConns: 22
    maxIdleConns: 6
    connMaxLifetime: 50m
    connMaxIdleTime: 20m
logging:
  level: info
  format: json
  request:
    maxBodyBytes: 1024
    maxFieldLength: 64
  file:
    enabled: true
    path: logs/test.log
    maxSizeMB: 16
    maxBackups: 5
    maxAgeDays: 14
    compress: false
cache:
  enabled: true
  prefix: app
  codec: sonic
  l1:
    enabled: true
    maxCost: 2048
    numCounters: 200
    bufferItems: 16
    defaultTTL: 45s
  redis:
    enabled: true
    mode: cluster
    keyPrefix: cache
    database: 3
    clientName: seven-cache
    dialTimeout: 4s
    readTimeout: 5s
    writeTimeout: 6s
    poolSize: 8
    minIdleConns: 1
    cluster:
      addrs:
        - 127.0.0.1:7001
        - 127.0.0.1:7002
id:
  node: 8
`)
	writeFile(t, filepath.Join(dir, "application-test.yaml"), `
logging:
  level: debug
  request:
    includeQuery: false
`)

	t.Setenv("SEVEN_PROFILE", "test")
	t.Setenv("SERVER_PORT", "9999")
	t.Setenv("SERVER_CONTEXT_PATH", "/api")

	cfg, err := Load(dir)
	if err != nil {
		t.Fatalf("load config: %v", err)
	}

	if cfg.Profile != "test" {
		t.Fatalf("unexpected profile: %s", cfg.Profile)
	}
	if cfg.Server.Port != 9999 {
		t.Fatalf("unexpected port: %d", cfg.Server.Port)
	}
	if cfg.Server.ContextPath != "/api" {
		t.Fatalf("unexpected context path: %s", cfg.Server.ContextPath)
	}
	if cfg.Datasource.Driver != "postgres" {
		t.Fatalf("unexpected datasource driver: %s", cfg.Datasource.Driver)
	}
	if !cfg.Datasource.Bootstrap.Enabled {
		t.Fatal("expected datasource bootstrap enabled")
	}
	if cfg.Datasource.Bootstrap.Mode != BootstrapModeBoth {
		t.Fatalf("unexpected bootstrap mode: %s", cfg.Datasource.Bootstrap.Mode)
	}
	if cfg.Datasource.Bootstrap.MigrationsDir != "migrations/custom" {
		t.Fatalf("unexpected bootstrap migrationsDir: %s", cfg.Datasource.Bootstrap.MigrationsDir)
	}
	if cfg.Datasource.Bootstrap.VersionTable != "goose_custom" {
		t.Fatalf("unexpected bootstrap version table: %s", cfg.Datasource.Bootstrap.VersionTable)
	}
	if cfg.Datasource.Bootstrap.BaselineVersion != "20260422000000" {
		t.Fatalf("unexpected bootstrap baseline version: %s", cfg.Datasource.Bootstrap.BaselineVersion)
	}
	if !cfg.Datasource.Bootstrap.AllowLegacySync {
		t.Fatal("expected allowLegacySync enabled")
	}
	if cfg.Logging.Level != "debug" {
		t.Fatalf("unexpected logging level: %s", cfg.Logging.Level)
	}
	if cfg.Logging.Request.MaxBodyBytes != 1024 {
		t.Fatalf("unexpected max body bytes: %d", cfg.Logging.Request.MaxBodyBytes)
	}
	if cfg.Logging.Request.MaxFieldLength != 64 {
		t.Fatalf("unexpected max field length: %d", cfg.Logging.Request.MaxFieldLength)
	}
	if cfg.Logging.Request.IncludeQuery {
		t.Fatal("expected includeQuery override to be false")
	}
	if cfg.Challenge.OTPAllowedDriftWindows != 1 {
		t.Fatalf("unexpected challenge otp allowed drift windows: %d", cfg.Challenge.OTPAllowedDriftWindows)
	}
	if cfg.Challenge.RecoveryBatchSize != 10 {
		t.Fatalf("unexpected challenge recovery batch size: %d", cfg.Challenge.RecoveryBatchSize)
	}
	if cfg.Challenge.TriggerMaxAttempts != 10 {
		t.Fatalf("unexpected challenge trigger max attempts: %d", cfg.Challenge.TriggerMaxAttempts)
	}
	if cfg.SSO.RateLimit.TokenLimit != 60 || cfg.SSO.RateLimit.TokenWindow.String() != "1m0s" {
		t.Fatalf("unexpected sso token rate limit defaults: %+v", cfg.SSO.RateLimit)
	}
	if cfg.SSO.RateLimit.UserInfoLimit != 120 || cfg.SSO.RateLimit.UserInfoWindow.String() != "1m0s" {
		t.Fatalf("unexpected sso userinfo rate limit defaults: %+v", cfg.SSO.RateLimit)
	}
	if !cfg.Logging.File.Enabled {
		t.Fatal("expected file logging enabled")
	}
	if cfg.Logging.File.Path != "logs/test.log" {
		t.Fatalf("unexpected log file path: %s", cfg.Logging.File.Path)
	}
	if cfg.Logging.File.MaxSizeMB != 16 {
		t.Fatalf("unexpected file maxSizeMB: %d", cfg.Logging.File.MaxSizeMB)
	}
	if !cfg.Datasource.MySQL.Enabled {
		t.Fatal("expected mysql datasource enabled")
	}
	if cfg.Datasource.MySQL.DSN != "user:pass@tcp(127.0.0.1:3306)/seven?parseTime=true" {
		t.Fatalf("unexpected mysql dsn: %s", cfg.Datasource.MySQL.DSN)
	}
	if cfg.Datasource.MySQL.MaxOpenConns != 30 {
		t.Fatalf("unexpected mysql maxOpenConns: %d", cfg.Datasource.MySQL.MaxOpenConns)
	}
	if cfg.Datasource.MySQL.MaxIdleConns != 7 {
		t.Fatalf("unexpected mysql maxIdleConns: %d", cfg.Datasource.MySQL.MaxIdleConns)
	}
	if !cfg.Datasource.Postgres.Enabled {
		t.Fatal("expected postgres datasource enabled")
	}
	if cfg.Datasource.Postgres.DSN != "postgres://postgres:postgres@127.0.0.1:5432/seven?sslmode=disable" {
		t.Fatalf("unexpected postgres dsn: %s", cfg.Datasource.Postgres.DSN)
	}
	if cfg.Datasource.Postgres.MaxOpenConns != 22 {
		t.Fatalf("unexpected postgres maxOpenConns: %d", cfg.Datasource.Postgres.MaxOpenConns)
	}
	if cfg.Datasource.Postgres.MaxIdleConns != 6 {
		t.Fatalf("unexpected postgres maxIdleConns: %d", cfg.Datasource.Postgres.MaxIdleConns)
	}
	if !cfg.Cache.Enabled {
		t.Fatal("expected cache enabled")
	}
	if cfg.Cache.Prefix != "app" {
		t.Fatalf("unexpected cache prefix: %s", cfg.Cache.Prefix)
	}
	if cfg.Cache.Codec != "sonic" {
		t.Fatalf("unexpected cache codec: %s", cfg.Cache.Codec)
	}
	if !cfg.Cache.L1Enabled() {
		t.Fatal("expected l1 cache enabled")
	}
	if cfg.Cache.L1.DefaultTTL.String() != "45s" {
		t.Fatalf("unexpected l1 default ttl: %s", cfg.Cache.L1.DefaultTTL)
	}
	if cfg.Cache.Redis.Mode != RedisCacheModeCluster {
		t.Fatalf("unexpected redis mode: %s", cfg.Cache.Redis.Mode)
	}
	if !cfg.Cache.Redis.Configured() {
		t.Fatal("expected redis configured")
	}
	if len(cfg.Cache.Redis.Cluster.Addrs) != 2 {
		t.Fatalf("unexpected cluster addrs: %+v", cfg.Cache.Redis.Cluster.Addrs)
	}
	if !cfg.Datasource.Enabled() {
		t.Fatal("expected active datasource enabled")
	}
	if !cfg.Datasource.Configured() {
		t.Fatal("expected active datasource configured")
	}
	if len(cfg.LoadedFiles) != 2 {
		t.Fatalf("expected 2 loaded files, got %d", len(cfg.LoadedFiles))
	}
}

func TestLoadDefaultsPlatformModeLocal(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "application.yaml"), `
seven:
  name: platform-default-test
`)
	t.Setenv("SEVEN_PROFILE", "test")

	cfg, err := Load(dir)
	if err != nil {
		t.Fatalf("load config: %v", err)
	}

	if cfg.Platform.Mode != PlatformModeLocal {
		t.Fatalf("platform mode=%q want %q", cfg.Platform.Mode, PlatformModeLocal)
	}
	if cfg.Platform.Node.InternalListener.Enabled {
		t.Fatalf("platform node internal listener must default disabled: %+v", cfg.Platform.Node.InternalListener)
	}
	if cfg.Platform.Node.InternalListener.Listen != "127.0.0.1:9777" {
		t.Fatalf("platform node internal listener=%q", cfg.Platform.Node.InternalListener.Listen)
	}
	if cfg.Docker.Enabled {
		t.Fatal("docker must default to disabled")
	}
	if cfg.Docker.FailFast {
		t.Fatal("docker fail-fast must default to disabled")
	}
	if !cfg.Workforce.ProviderSSRF.Enabled {
		t.Fatal("workforce.providerSSRF.enabled must default true")
	}
	if cfg.Workforce.ProviderSSRF.MaxRequestBytes != 128<<20 || cfg.Workforce.ProviderSSRF.MaxResponseHeaderBytes != 256<<10 {
		t.Fatalf("unexpected provider SSRF limits: %+v", cfg.Workforce.ProviderSSRF)
	}
}

func TestLoadWorkforceProviderSSRFStructuredAllowList(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "application.yaml"), `
platform:
  mode: local
workforce:
  providerSSRF:
    enabled: true
    allowedList:
      - hostname: provider.example.test
        cidrs: [198.18.0.0/15]
        ports: [443]
        allowFakeIP: true
`)
	t.Setenv("SEVEN_PROFILE", "test")
	cfg, err := Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	entries := cfg.Workforce.ProviderSSRF.AllowedList
	if len(entries) != 1 || entries[0].Hostname != "provider.example.test" || !entries[0].AllowFakeIP || entries[0].Ports[0] != 443 {
		t.Fatalf("allowedList=%+v", entries)
	}
}

func TestLoadNotificationOutboundEnvironmentPolicies(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "application.yaml"), `
seven:
  env: dev
notification:
  outbound:
    policies:
      - name: corp-orders
        mode: PRIVATE_ALLOWLIST
        allowedHostnames: [orders.corp.example]
        allowedCidrs: [10.20.0.0/16]
        allowedPorts: [8443]
      - name: local-fake
        mode: FAKE_IP_PROXY
        allowedHostnames: [receiver.example]
        allowedCidrs: [198.18.0.0/15]
        allowedPorts: [443]
        proxyUrl: https://proxy.example:8443
`)
	t.Setenv("SEVEN_PROFILE", "test")

	cfg, err := Load(dir)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	policies := cfg.Notification.Outbound.Policies
	if len(policies) != 2 {
		t.Fatalf("outbound policies = %#v, want two entries", policies)
	}
	if policies[0].Name != "corp-orders" || policies[0].Mode != "PRIVATE_ALLOWLIST" || policies[0].AllowedHostnames[0] != "orders.corp.example" || policies[0].AllowedCIDRs[0] != "10.20.0.0/16" || policies[0].AllowedPorts[0] != 8443 {
		t.Fatalf("private policy = %#v", policies[0])
	}
	if policies[1].ProxyURL != "https://proxy.example:8443" {
		t.Fatalf("fake-IP proxy URL = %q", policies[1].ProxyURL)
	}
}

func TestLoadPlatformNodeInternalListenerConfiguration(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "application.yaml"), `
platform:
  mode: node
  node:
    code: order-admin
    managementBearer: node-bearer
    internalListener:
      enabled: true
      listen: 127.0.0.1:9788
`)
	t.Setenv("SEVEN_PROFILE", "test")

	cfg, err := Load(dir)
	if err != nil {
		t.Fatalf("load config: %v", err)
	}
	if !cfg.Platform.Node.InternalListener.Enabled || cfg.Platform.Node.InternalListener.Listen != "127.0.0.1:9788" {
		t.Fatalf("platform node internal listener=%+v", cfg.Platform.Node.InternalListener)
	}
}

func TestLoadDefaultsMicroserviceRuntime(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "application.yaml"), "seven:\n  name: microservice-default-test\n")
	t.Setenv("SEVEN_PROFILE", "test")

	cfg, err := Load(dir)
	if err != nil {
		t.Fatalf("load config: %v", err)
	}
	microservice := cfg.Microservice
	if microservice.Enabled || microservice.InternalServer.Enabled {
		t.Fatalf("microservice must default disabled: %+v", microservice)
	}
	if microservice.InternalServer.Listen != "127.0.0.1:9377" {
		t.Fatalf("internal listen=%q", microservice.InternalServer.Listen)
	}
	if microservice.Service.Name != "seven-hub" || microservice.Service.Scheme != "http" {
		t.Fatalf("service defaults=%+v", microservice.Service)
	}
	if microservice.Registry.Type != "consul" || microservice.Registry.Address != "http://127.0.0.1:8500" || !microservice.Registry.Enabled {
		t.Fatalf("registry defaults=%+v", microservice.Registry)
	}
	if microservice.Registry.RegisterTimeout != 3*time.Second || microservice.Registry.DeregisterTimeout != 3*time.Second {
		t.Fatalf("registry timeout defaults=%+v", microservice.Registry)
	}
	if microservice.Health.Interval != 10*time.Second || microservice.Health.Timeout != 2*time.Second {
		t.Fatalf("health defaults=%+v", microservice.Health)
	}
	if microservice.Discovery.Type != "consul" || microservice.Discovery.CacheTTL != 10*time.Second || microservice.Discovery.EmptyResultTTL != time.Second || microservice.Discovery.ResolveTimeout != 2*time.Second || !microservice.Discovery.StaticFallbackEnabled {
		t.Fatalf("discovery defaults=%+v", microservice.Discovery)
	}
	if microservice.Client.ConnectTimeout != time.Second || microservice.Client.RequestTimeout != 3*time.Second || microservice.Client.MaxIdleConns != 100 || microservice.Client.MaxIdleConnsPerHost != 20 || microservice.Client.IdleConnTimeout != 90*time.Second {
		t.Fatalf("client defaults=%+v", microservice.Client)
	}
	if microservice.Client.MaxRequestBytes != 1<<20 || microservice.Client.MaxResponseBytes != 4<<20 {
		t.Fatalf("client payload defaults=%+v", microservice.Client)
	}
	if len(microservice.Outbound.TrustedHosts) != 0 || len(microservice.Outbound.TrustedCIDRs) != 0 || len(microservice.Outbound.RegistryTrustedHosts) != 0 || len(microservice.Outbound.RegistryTrustedCIDRs) != 0 {
		t.Fatalf("production defaults must not implicitly trust private destinations: %+v", microservice.Outbound)
	}
}

func TestLoadMicroserviceRuntimeConfiguration(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "application.yaml"), `
microservice:
  enabled: true
  internalServer:
    enabled: true
    listen: 127.0.0.1:9477
  service:
    name: seven-node
    instanceId: node-a
    advertisedHost: 10.0.0.12
    advertisedPort: 9477
    scheme: https
    tags: [seven, node]
    metadata:
      version: 2.0.0
  registry:
    enabled: true
    type: consul
    address: http://consul.internal:8500
    datacenter: dc2
    token: yaml-token
    registrationRequired: true
    registerTimeout: 4s
    deregisterTimeout: 5s
  health:
    interval: 15s
    timeout: 3s
  discovery:
    type: static
    cacheTtl: 20s
    emptyResultTtl: 2s
    resolveTimeout: 4s
    datacenter: dc2
    tags: [node]
    staticFallbackEnabled: false
  static:
    services:
      seven-hub:
        - url: https://hub-a.internal:9443
        - url: http://hub-b.internal:9080
  client:
    connectTimeout: 2s
    requestTimeout: 6s
    maxIdleConns: 200
    maxIdleConnsPerHost: 40
    idleConnTimeout: 2m
    maxRequestBytes: 2048
    maxResponseBytes: 8192
`)
	t.Setenv("SEVEN_PROFILE", "test")

	cfg, err := Load(dir)
	if err != nil {
		t.Fatalf("load config: %v", err)
	}
	microservice := cfg.Microservice
	if !microservice.Enabled || !microservice.InternalServer.Enabled || microservice.InternalServer.Listen != "127.0.0.1:9477" {
		t.Fatalf("internal server=%+v", microservice.InternalServer)
	}
	if microservice.Service.InstanceID != "node-a" || microservice.Service.AdvertisedHost != "10.0.0.12" || microservice.Service.AdvertisedPort != 9477 || microservice.Service.Scheme != "https" {
		t.Fatalf("service=%+v", microservice.Service)
	}
	if microservice.Registry.Token != "yaml-token" || !microservice.Registry.RegistrationRequired || microservice.Registry.Datacenter != "dc2" {
		t.Fatalf("registry=%+v", microservice.Registry)
	}
	if microservice.Discovery.Type != "static" || microservice.Discovery.StaticFallbackEnabled {
		t.Fatalf("discovery=%+v", microservice.Discovery)
	}
	if microservice.Client.MaxRequestBytes != 2048 || microservice.Client.MaxResponseBytes != 8192 {
		t.Fatalf("client payload limits=%+v", microservice.Client)
	}
	urls := microservice.Static.Services["seven-hub"]
	if len(urls) != 2 || urls[0].URL != "https://hub-a.internal:9443" || urls[1].URL != "http://hub-b.internal:9080" {
		t.Fatalf("static services=%+v", microservice.Static.Services)
	}
}

func TestLoadMicroserviceOutboundTrustConfiguration(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "application.yaml"), `
microservice:
  outbound:
    trustedHosts: [" node.internal "]
    trustedCidrs: ["10.20.0.0/16"]
    registryTrustedHosts: [" registry-node.internal "]
    registryTrustedCidrs: ["fd00:30::/48"]
`)
	t.Setenv("SEVEN_PROFILE", "test")
	cfg, err := Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	outbound := cfg.Microservice.Outbound
	if len(outbound.TrustedHosts) != 1 || outbound.TrustedHosts[0] != "node.internal" || len(outbound.RegistryTrustedHosts) != 1 || outbound.RegistryTrustedHosts[0] != "registry-node.internal" {
		t.Fatalf("outbound hosts=%+v", outbound)
	}
	if len(outbound.TrustedCIDRs) != 1 || len(outbound.RegistryTrustedCIDRs) != 1 {
		t.Fatalf("outbound CIDRs=%+v", outbound)
	}
}

func TestLoadRejectsInvalidMicroserviceOutboundCIDR(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "application.yaml"), "microservice:\n  outbound:\n    trustedCidrs: [not-a-cidr]\n")
	t.Setenv("SEVEN_PROFILE", "test")
	if _, err := Load(dir); err == nil || !strings.Contains(err.Error(), "trustedCidrs") {
		t.Fatalf("Load() error=%v", err)
	}
}

func TestLoadRejectsOutOfRangeStaticServicePorts(t *testing.T) {
	for _, rawURL := range []string{"http://hub.internal:0", "http://hub.internal:65536"} {
		t.Run(rawURL, func(t *testing.T) {
			dir := t.TempDir()
			writeFile(t, filepath.Join(dir, "application.yaml"), "microservice:\n  static:\n    services:\n      seven-hub:\n        - url: "+rawURL+"\n")
			t.Setenv("SEVEN_PROFILE", "test")

			_, err := Load(dir)
			if err == nil || !strings.Contains(err.Error(), "port") {
				t.Fatalf("Load() error=%v, want invalid static port", err)
			}
		})
	}
}

func TestLoadNormalizesAndValidatesMicroservicePayloadLimits(t *testing.T) {
	t.Run("zero uses bounded defaults", func(t *testing.T) {
		dir := t.TempDir()
		writeFile(t, filepath.Join(dir, "application.yaml"), "microservice:\n  client:\n    maxRequestBytes: 0\n    maxResponseBytes: 0\n")
		t.Setenv("SEVEN_PROFILE", "test")

		cfg, err := Load(dir)
		if err != nil {
			t.Fatalf("Load() error=%v", err)
		}
		if cfg.Microservice.Client.MaxRequestBytes != 1<<20 || cfg.Microservice.Client.MaxResponseBytes != 4<<20 {
			t.Fatalf("payload limits=%+v", cfg.Microservice.Client)
		}
	})

	for _, field := range []string{"maxRequestBytes", "maxResponseBytes"} {
		t.Run("negative "+field, func(t *testing.T) {
			dir := t.TempDir()
			writeFile(t, filepath.Join(dir, "application.yaml"), "microservice:\n  client:\n    "+field+": -1\n")
			t.Setenv("SEVEN_PROFILE", "test")

			_, err := Load(dir)
			if err == nil || !strings.Contains(err.Error(), "payload limits") {
				t.Fatalf("Load() error=%v, want invalid payload limit", err)
			}
		})
	}
}

func TestValidateOpenIntegrationDeliveryLeaseExceedsHTTPTimeout(t *testing.T) {
	for _, tc := range []struct {
		name         string
		leaseSeconds int
		timeout      int
		wantErr      bool
	}{
		{name: "valid margin", leaseSeconds: 25, timeout: 10},
		{name: "equal", leaseSeconds: 10, timeout: 10, wantErr: true},
		{name: "insufficient margin", leaseSeconds: 24, timeout: 10, wantErr: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			loader := viper.New()
			setDefaults(loader)
			var cfg Config
			if err := loader.Unmarshal(&cfg, viper.DecodeHook(mapstructure.StringToTimeDurationHookFunc())); err != nil {
				t.Fatalf("unmarshal default config: %v", err)
			}
			cfg.Datasource.Driver = "postgres"
			cfg.Datasource.Postgres.Enabled = true
			cfg.Datasource.Postgres.DSN = "postgres://acceptance.invalid/seven"
			cfg.RabbitMQ.Enabled = true
			cfg.Workforce.Enabled = true
			cfg.Workforce.Artifact.Endpoint = "https://artifact.invalid"
			cfg.Workforce.Artifact.AccessKey = "fixture-access"
			cfg.Workforce.Artifact.SecretKey = "fixture-secret"
			cfg.Workforce.Artifact.Bucket = "fixture-bucket"
			cfg.Workforce.Worker.ID = "worker-fixture"
			cfg.Workforce.Worker.MaxActiveContainers = 1
			cfg.Workforce.Worker.WorkspaceRoot = t.TempDir()
			cfg.Workforce.Harness.Alias = "fixture"
			cfg.Workforce.Harness.Image = "fixture@sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
			cfg.Workforce.Harness.Contract = "seven.worker-image/v1"
			cfg.Workforce.OpenIntegration.Enabled = true
			cfg.Workforce.OpenIntegration.DeliveryLeaseSeconds = tc.leaseSeconds
			cfg.Workforce.OpenIntegration.HTTPTimeoutSeconds = tc.timeout

			err := validateWorkforceConfig(cfg)
			if tc.wantErr {
				if err == nil || !strings.Contains(err.Error(), "deliveryLeaseSeconds must be at least 15 seconds greater") {
					t.Fatalf("validateWorkforceConfig() error=%v, want lease/timeout error", err)
				}
				return
			}
			if err != nil {
				t.Fatalf("validateWorkforceConfig() error=%v", err)
			}
		})
	}
}

func TestLoadBindsConsulAddressAndTokenFromEnvironment(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "application.yaml"), `
microservice:
  registry:
    address: http://yaml-consul:8500
    token: yaml-token
`)
	t.Setenv("SEVEN_PROFILE", "test")
	t.Setenv("CONSUL_HTTP_ADDR", "http://127.0.0.1:18500")
	t.Setenv("CONSUL_HTTP_TOKEN", "environment-token")

	cfg, err := Load(dir)
	if err != nil {
		t.Fatalf("load config: %v", err)
	}
	if cfg.Microservice.Registry.Address != "http://127.0.0.1:18500" {
		t.Fatalf("registry address=%q", cfg.Microservice.Registry.Address)
	}
	if cfg.Microservice.Registry.Token != "environment-token" {
		t.Fatalf("registry token=%q", cfg.Microservice.Registry.Token)
	}
}

func TestLoadValidatesPlatformModes(t *testing.T) {
	for _, tc := range []struct {
		name     string
		mode     string
		bearer   string
		wantErr  bool
		wantMode PlatformMode
	}{
		{name: "local", mode: "local", wantMode: PlatformMode("local")},
		{name: "hub", mode: "hub", wantMode: PlatformMode("hub")},
		{name: "node", mode: "node", bearer: "node-bearer", wantMode: PlatformMode("node")},
		{name: "remote", mode: "remote", wantErr: true},
		{name: "disabled", mode: "disabled", wantErr: true},
		{name: "empty", mode: "", wantErr: true},
		{name: "typo", mode: "typo", wantErr: true},
		{name: "uppercase", mode: "HUB", wantErr: true},
		{name: "whitespace-padded", mode: " node ", bearer: "node-bearer", wantErr: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			content := "platform:\n  mode: \"" + tc.mode + "\"\n"
			if tc.bearer != "" {
				content += "  node:\n    code: order-admin\n    managementBearer: \"" + tc.bearer + "\"\n"
			}
			writeFile(t, filepath.Join(dir, "application.yaml"), content)
			t.Setenv("SEVEN_PROFILE", "test")

			cfg, err := Load(dir)
			if (err != nil) != tc.wantErr {
				t.Fatalf("Load() error=%v wantErr=%v", err, tc.wantErr)
			}
			if tc.wantErr {
				if !strings.Contains(err.Error(), "local, hub, node") {
					t.Fatalf("validation error=%q must list accepted modes", err)
				}
				return
			}
			if cfg.Platform.Mode != tc.wantMode {
				t.Fatalf("platform mode=%q want %q", cfg.Platform.Mode, tc.wantMode)
			}
		})
	}
}

func TestLoadRejectsNodeCodeThatCannotRepresentManagedProviderOwner(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "application.yaml"), "platform:\n  mode: node\n  node:\n    code: \""+strings.Repeat("a", 61)+"\"\n    managementBearer: node-bearer\n")
	t.Setenv("SEVEN_PROFILE", "test")
	_, err := Load(dir)
	if err == nil || !strings.Contains(err.Error(), "platform.node.code") {
		t.Fatalf("expected managed owner length validation, got %v", err)
	}
}

func TestLoadRequiresNodeManagementBearer(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "application.yaml"), "platform:\n  mode: node\n  node:\n    code: order-admin\n    managementBearer: \"\"\n")
	t.Setenv("SEVEN_PROFILE", "test")

	_, err := Load(dir)
	if err == nil || !strings.Contains(err.Error(), "platform.node.managementBearer") {
		t.Fatalf("Load() error=%v want missing node management bearer validation", err)
	}
}

func TestLoadBindsNodeManagementBearerFromEnvironment(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "application.yaml"), "platform:\n  mode: node\n  node:\n    code: order-admin\n    managementBearer: \"${NODE_MANAGEMENT_BEARER}\"\n")
	t.Setenv("SEVEN_PROFILE", "test")
	t.Setenv("NODE_MANAGEMENT_BEARER", "node-bearer-from-env")

	cfg, err := Load(dir)
	if err != nil {
		t.Fatalf("load config: %v", err)
	}
	if cfg.Platform.Mode != PlatformMode("node") {
		t.Fatalf("platform mode=%q want node", cfg.Platform.Mode)
	}
	if got := cfg.Platform.Node.ManagementBearer; got != "node-bearer-from-env" {
		t.Fatalf("management bearer=%q want environment value", got)
	}
}

func TestLoadEnvironmentOverridesConfiguredNodeManagementBearer(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "application.yaml"), "platform:\n  mode: node\n  node:\n    code: order-admin\n    managementBearer: base-bearer\n")
	writeFile(t, filepath.Join(dir, "application-test.yaml"), "platform:\n  node:\n    managementBearer: profile-bearer\n")
	t.Setenv("SEVEN_PROFILE", "test")
	t.Setenv("NODE_MANAGEMENT_BEARER", "environment-bearer")

	cfg, err := Load(dir)
	if err != nil {
		t.Fatalf("load config: %v", err)
	}
	if got := cfg.Platform.Node.ManagementBearer; got != "environment-bearer" {
		t.Fatalf("management bearer=%q want environment override", got)
	}
}

func TestLoadRequiresNormalizedNodeCode(t *testing.T) {
	for _, tc := range []struct {
		name    string
		code    string
		wantErr bool
		want    string
	}{
		{name: "valid", code: " order-admin ", want: "order-admin"},
		{name: "empty", code: "", wantErr: true},
		{name: "invalid characters", code: "Order Admin", wantErr: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			writeFile(t, filepath.Join(dir, "application.yaml"), "platform:\n  mode: node\n  node:\n    code: \""+tc.code+"\"\n    managementBearer: node-bearer\n")
			t.Setenv("SEVEN_PROFILE", "test")

			cfg, err := Load(dir)
			if (err != nil) != tc.wantErr {
				t.Fatalf("Load() error=%v wantErr=%v", err, tc.wantErr)
			}
			if tc.wantErr {
				if !strings.Contains(err.Error(), "platform.node.code") {
					t.Fatalf("validation error=%q must identify platform.node.code", err)
				}
				return
			}
			if cfg.Platform.Node.Code != tc.want {
				t.Fatalf("node code=%q want %q", cfg.Platform.Node.Code, tc.want)
			}
		})
	}
}

func TestLoadAllowsMissingProfileFile(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "application.yaml"), `
seven:
  name: only-base
`)

	t.Setenv("SEVEN_PROFILE", "staging")

	cfg, err := Load(dir)
	if err != nil {
		t.Fatalf("load config without profile file: %v", err)
	}

	if cfg.Seven.Name != "only-base" {
		t.Fatalf("unexpected app name: %s", cfg.Seven.Name)
	}
	if len(cfg.LoadedFiles) != 1 {
		t.Fatalf("expected 1 loaded file, got %d", len(cfg.LoadedFiles))
	}
	if !cfg.Logging.Request.Enabled {
		t.Fatal("expected request logging enabled by default")
	}
	if cfg.Logging.Request.MaxBodyBytes != 4096 {
		t.Fatalf("unexpected default maxBodyBytes: %d", cfg.Logging.Request.MaxBodyBytes)
	}
	if !cfg.Logging.File.Enabled {
		t.Fatal("expected file logging enabled by default")
	}
	if cfg.Logging.File.Path != "logs/seven-agent-force-server.log" {
		t.Fatalf("unexpected default log path: %s", cfg.Logging.File.Path)
	}
	if cfg.Challenge.OTPAllowedDriftWindows != 1 {
		t.Fatalf("unexpected default challenge otp allowed drift windows: %d", cfg.Challenge.OTPAllowedDriftWindows)
	}
	if cfg.Challenge.RecoveryBatchSize != 10 {
		t.Fatalf("unexpected default challenge recovery batch size: %d", cfg.Challenge.RecoveryBatchSize)
	}
	if cfg.Challenge.TriggerMaxAttempts != 10 {
		t.Fatalf("unexpected default challenge trigger max attempts: %d", cfg.Challenge.TriggerMaxAttempts)
	}
	if cfg.Datasource.Driver != "mysql" {
		t.Fatalf("unexpected default datasource driver: %s", cfg.Datasource.Driver)
	}
	if cfg.Datasource.Bootstrap.Enabled {
		t.Fatal("expected datasource bootstrap disabled by default")
	}
	if cfg.Datasource.Bootstrap.Mode != BootstrapModeManual {
		t.Fatalf("unexpected default bootstrap mode: %s", cfg.Datasource.Bootstrap.Mode)
	}
	if cfg.Datasource.Bootstrap.VersionTable != "goose_db_version" {
		t.Fatalf("unexpected default bootstrap version table: %s", cfg.Datasource.Bootstrap.VersionTable)
	}
	if cfg.Datasource.Bootstrap.ChangeOwner != "goose" {
		t.Fatalf("unexpected default bootstrap change owner: %s", cfg.Datasource.Bootstrap.ChangeOwner)
	}
	if cfg.Datasource.MySQL.Enabled {
		t.Fatal("expected mysql datasource disabled by default")
	}
	if cfg.Datasource.MySQL.DSN != "" {
		t.Fatalf("unexpected default mysql dsn: %s", cfg.Datasource.MySQL.DSN)
	}
	if cfg.Datasource.MySQL.MaxOpenConns != 20 {
		t.Fatalf("unexpected default mysql maxOpenConns: %d", cfg.Datasource.MySQL.MaxOpenConns)
	}
	if cfg.Datasource.Postgres.Enabled {
		t.Fatal("expected postgres datasource disabled by default")
	}
	if cfg.Cache.Enabled {
		t.Fatal("expected cache disabled by default")
	}
	if cfg.Cache.Prefix != "seven" {
		t.Fatalf("unexpected default cache prefix: %s", cfg.Cache.Prefix)
	}
	if cfg.Cache.Codec != "sonic" {
		t.Fatalf("unexpected default cache codec: %s", cfg.Cache.Codec)
	}
	if !cfg.Cache.L1.Enabled {
		t.Fatal("expected l1 cache enabled by default")
	}
	if cfg.Cache.Redis.Mode != RedisCacheModeSingle {
		t.Fatalf("unexpected default redis mode: %s", cfg.Cache.Redis.Mode)
	}
	if !cfg.Cache.Redis.Enabled {
		t.Fatal("expected redis cache enabled by default")
	}
	if cfg.Cache.Redis.Single.Addr != "127.0.0.1:6379" {
		t.Fatalf("unexpected default redis addr: %s", cfg.Cache.Redis.Single.Addr)
	}
	if cfg.Datasource.Enabled() {
		t.Fatal("expected active datasource disabled by default")
	}
	if cfg.Datasource.Configured() {
		t.Fatal("expected active datasource unconfigured by default")
	}
	for _, route := range []string{"/sso/oauth2/userinfo", "/sso/oauth2/revoke", "/sso/oauth2/introspect", "/platform/admin/**", "/external-login/admin/**", "GET /external-identities/oauth/callback/*", "GET /config-assets/:id", "GET /open/workforce/v1/capabilities", "POST /open/workforce/v1/projects/:projectId/tasks", "GET /open/workforce/v1/projects/:projectId/model-options", "GET /open/workforce/v1/projects/:projectId/runtime-options", "GET /open/workforce/v1/tasks/**"} {
		if !containsString(cfg.Authorization.AnonymousURLs, route) {
			t.Fatalf("expected oauth handler-owned route %s to be anonymous by default, got %+v", route, cfg.Authorization.AnonymousURLs)
		}
	}
}

func TestLoadNormalizesGlobalOriginPatterns(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "application.yaml"), `
security:
  originPatterns:
    - " https://console.example.com "
    - "   "
`)

	cfg, err := Load(dir)
	if err != nil {
		t.Fatalf("load config: %v", err)
	}
	if got := strings.Join(cfg.Security.OriginPatterns, ","); got != "https://console.example.com" {
		t.Fatalf("unexpected global Origin patterns: %q", got)
	}
}

func TestLoadRejectsInsecureProductionAuthConfig(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "application.yaml"), `
seven:
  name: prod-auth
  env: prod
`)

	t.Setenv("SEVEN_PROFILE", "prod")

	_, err := Load(dir)
	if err == nil {
		t.Fatal("expected insecure production auth config to be rejected")
	}
	message := err.Error()
	for _, want := range []string{
		"sso.issuer must use https",
		"sso.sessionCookie.secure must be true",
		"authorization.internal.token must be production-strength",
		"authorization.internal.signatureEnabled must be true",
		"challenge.webauthnRpId must be production domain",
		"challenge.webauthnAllowedOrigins must use https",
	} {
		if !strings.Contains(message, want) {
			t.Fatalf("expected production gate error to include %q, got %s", want, message)
		}
	}
}

func TestLoadRejectsInsecureProductionWebAuthnConfig(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "application.yaml"), `
seven:
  name: prod-auth
  env: prod
sso:
  issuer: https://auth.example.com/api/sso
  baseUrl: https://auth.example.com/api/sso
  frontendLoginUrl: https://auth.example.com/login
  sessionCookie:
    secure: true
  refreshCookie:
    secure: true
    httpOnly: true
authorization:
  internal:
    enabled: true
    token: pA9qZ7mR4vL2nX8cT6bY3wK5
    signatureEnabled: true
    signatureSecret: aR8mV2qL9zX5cN7pT4wK6yB3dH1s
challenge:
  webauthnRpId: localhost
  webauthnAllowedOrigins:
    - http://localhost:5177
    - https://evil.example.net
`)

	t.Setenv("SEVEN_PROFILE", "prod")

	_, err := Load(dir)
	if err == nil {
		t.Fatal("expected insecure production WebAuthn config to be rejected")
	}
	message := err.Error()
	for _, want := range []string{
		"challenge.webauthnRpId must be production domain",
		"challenge.webauthnAllowedOrigins must use https",
		"challenge.webauthnAllowedOrigins must match challenge.webauthnRpId",
	} {
		if !strings.Contains(message, want) {
			t.Fatalf("expected production gate error to include %q, got %s", want, message)
		}
	}
}

func TestLoadRejectsCrossSiteProductionRefreshCookie(t *testing.T) {
	dir := t.TempDir()
	content := strings.Replace(secureProductionConfigWithWebAuthn("auth.example.com", "https://auth.example.com"), "    sameSite: Lax\n"+secureProductionSetupOriginBlock(), "    sameSite: None\n"+secureProductionSetupOriginBlock(), 1)
	writeFile(t, filepath.Join(dir, "application.yaml"), content)

	t.Setenv("SEVEN_PROFILE", "prod")

	_, err := Load(dir)
	if err == nil {
		t.Fatal("expected cross-site production refresh cookie to be rejected")
	}
	if !strings.Contains(err.Error(), "sso.refreshCookie.sameSite must be Lax or Strict in production") {
		t.Fatalf("expected refresh cookie sameSite production violation, got %v", err)
	}
}

func TestLoadRejectsCrossSiteProductionSessionCookie(t *testing.T) {
	dir := t.TempDir()
	content := strings.Replace(secureProductionConfigWithWebAuthn("auth.example.com", "https://auth.example.com"), "  sessionCookie:\n    secure: true\n    sameSite: Lax\n", "  sessionCookie:\n    secure: true\n    sameSite: None\n", 1)
	writeFile(t, filepath.Join(dir, "application.yaml"), content)

	t.Setenv("SEVEN_PROFILE", "prod")

	_, err := Load(dir)
	if err == nil {
		t.Fatal("expected cross-site production session cookie to be rejected")
	}
	if !strings.Contains(err.Error(), "sso.sessionCookie.sameSite must be Lax or Strict in production") {
		t.Fatalf("expected session cookie sameSite production violation, got %v", err)
	}
}

func TestLoadRejectsInsecureProductionOriginPatterns(t *testing.T) {
	dir := t.TempDir()
	content := secureProductionConfigWithWebAuthn("auth.example.com", "https://auth.example.com") + `
security:
  originPatterns:
    - https://*.example.com
`
	writeFile(t, filepath.Join(dir, "application.yaml"), content)

	t.Setenv("SEVEN_PROFILE", "prod")

	_, err := Load(dir)
	if err == nil {
		t.Fatal("expected wildcard global Origin config to be rejected")
	}
	if !strings.Contains(err.Error(), "security.originPatterns must not contain wildcards in production") {
		t.Fatalf("expected global Origin production violation, got %v", err)
	}
}

func TestLoadRejectsProductionSSORateLimitFailOpen(t *testing.T) {
	dir := t.TempDir()
	content := strings.Replace(secureProductionConfigWithWebAuthn("auth.example.com", "https://auth.example.com"), "    failClosedOnError: true\n  sessionCookie:", "    failClosedOnError: false\n  sessionCookie:", 1)
	writeFile(t, filepath.Join(dir, "application.yaml"), content)

	t.Setenv("SEVEN_PROFILE", "prod")

	_, err := Load(dir)
	if err == nil {
		t.Fatal("expected production SSO rate-limit fail-open config to be rejected")
	}
	if !strings.Contains(err.Error(), "sso.rateLimit.failClosedOnError must be true in production") {
		t.Fatalf("expected SSO rate-limit fail-closed production violation, got %v", err)
	}
}

func TestLoadRejectsInsecureProductionSetupOriginConfig(t *testing.T) {
	tests := []struct {
		name    string
		replace string
		want    string
	}{
		{
			name: "missing required origin header",
			replace: `setup:
  requireOriginHeader: false
  allowedOriginPatterns:
    - https://auth.example.com
`,
			want: "setup.requireOriginHeader must be true in production",
		},
		{
			name: "wildcard origin",
			replace: `setup:
  requireOriginHeader: true
  allowedOriginPatterns:
    - https://*.example.com
`,
			want: "setup.allowedOriginPatterns must not contain wildcards in production",
		},
		{
			name: "http origin",
			replace: `setup:
  requireOriginHeader: true
  allowedOriginPatterns:
    - http://auth.example.com
`,
			want: "setup.allowedOriginPatterns must use https production origins",
		},
		{
			name: "localhost origin",
			replace: `setup:
  requireOriginHeader: true
  allowedOriginPatterns:
    - https://localhost:5177
`,
			want: "setup.allowedOriginPatterns must be production domains",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := t.TempDir()
			content := strings.Replace(secureProductionConfigWithWebAuthn("auth.example.com", "https://auth.example.com"), secureProductionSetupOriginBlock(), tt.replace, 1)
			writeFile(t, filepath.Join(dir, "application.yaml"), content)

			t.Setenv("SEVEN_PROFILE", "prod")

			_, err := Load(dir)
			if err == nil {
				t.Fatalf("expected insecure production setup origin config to be rejected")
			}
			if !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("expected setup origin production violation %q, got %v", tt.want, err)
			}
		})
	}
}

func TestLoadRejectsReservedOrIPLikeProductionWebAuthnRPID(t *testing.T) {
	tests := []struct {
		name string
		rpID string
	}{
		{name: "reserved test domain", rpID: "login.test"},
		{name: "reserved example tld", rpID: "auth.example"},
		{name: "reserved invalid domain", rpID: "auth.invalid"},
		{name: "mdns local domain", rpID: "login.local"},
		{name: "trailing dot loopback ip", rpID: "127.0.0.1."},
		{name: "trailing dot localhost", rpID: "localhost."},
		{name: "single label", rpID: "auth"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := t.TempDir()
			writeFile(t, filepath.Join(dir, "application.yaml"), secureProductionConfigWithWebAuthn(tt.rpID, "https://"+tt.rpID))
			t.Setenv("SEVEN_PROFILE", "prod")

			_, err := Load(dir)
			if err == nil {
				t.Fatalf("expected %s to be rejected as production WebAuthn RP ID", tt.rpID)
			}
			if !strings.Contains(err.Error(), "challenge.webauthnRpId must be production domain") {
				t.Fatalf("expected RP ID production-domain violation, got %v", err)
			}
		})
	}
}

func TestLoadAcceptsSecureProductionAuthConfig(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "application.yaml"), secureProductionConfigWithWebAuthn("auth.example.com", "https://auth.example.com"))

	t.Setenv("SEVEN_PROFILE", "prod")

	cfg, err := Load(dir)
	if err != nil {
		t.Fatalf("expected secure production auth config to load: %v", err)
	}
	if cfg.SSO.Issuer != "https://auth.example.com/api/sso" {
		t.Fatalf("unexpected issuer: %s", cfg.SSO.Issuer)
	}
}

func secureProductionConfigWithWebAuthn(rpID, origin string) string {
	return `
seven:
  name: prod-auth
  env: prod
server:
  context-path: /api
sso:
  issuer: https://auth.example.com/api/sso
  baseUrl: https://auth.example.com/api/sso
  frontendLoginUrl: https://auth.example.com/login
  rateLimit:
    failClosedOnError: true
  sessionCookie:
    secure: true
    sameSite: Lax
  refreshCookie:
    secure: true
    httpOnly: true
    sameSite: Lax
` + secureProductionSetupOriginBlock() + `
authorization:
  internal:
    enabled: true
    token: pA9qZ7mR4vL2nX8cT6bY3wK5
    signatureEnabled: true
    signatureSecret: aR8mV2qL9zX5cN7pT4wK6yB3dH1s
challenge:
  webauthnRpId: ` + rpID + `
  webauthnAllowedOrigins:
    - ` + origin + `
`
}

func secureProductionSetupOriginBlock() string {
	return `setup:
  requireOriginHeader: true
  allowedOriginPatterns:
    - https://auth.example.com`
}

func TestLoadDefaultsSSOBaseURLToRuntimeServerPort(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "application.yaml"), `
server:
  host: 0.0.0.0
  port: 9319
`)

	cfg, err := Load(dir)
	if err != nil {
		t.Fatalf("load config: %v", err)
	}
	if cfg.SSO.BaseURL != "http://127.0.0.1:9319/sso" {
		t.Fatalf("unexpected sso base url: %s", cfg.SSO.BaseURL)
	}
	if cfg.SSO.Issuer != "http://127.0.0.1:9319/sso" {
		t.Fatalf("unexpected sso issuer: %s", cfg.SSO.Issuer)
	}
}

func TestLoadIncludesContextPathInDefaultSSOBaseURL(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "application.yaml"), `
server:
  host: 0.0.0.0
  port: 9319
  context-path: /api/
`)

	cfg, err := Load(dir)
	if err != nil {
		t.Fatalf("load config: %v", err)
	}
	if cfg.Server.ContextPath != "/api" {
		t.Fatalf("unexpected context path: %s", cfg.Server.ContextPath)
	}
	if cfg.SSO.BaseURL != "http://127.0.0.1:9319/api/sso" {
		t.Fatalf("unexpected sso base url: %s", cfg.SSO.BaseURL)
	}
	if cfg.SSO.Issuer != "http://127.0.0.1:9319/api/sso" {
		t.Fatalf("unexpected sso issuer: %s", cfg.SSO.Issuer)
	}
}

func TestLoadFallsBackChallengeJWTKeysToCurrentSSOKey(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "application.yaml"), `
sso:
  jwt:
    currentKid: sso-kid-test
    privateKeysByKid:
      sso-kid-test: file:/tmp/sso-private.pem
    publicKeysByKid:
      sso-kid-test: file:/tmp/sso-public.pem
`)

	cfg, err := Load(dir)
	if err != nil {
		t.Fatalf("load config: %v", err)
	}
	if cfg.Security.Keys.JWT.Active.KID != "sso-kid-test" {
		t.Fatalf("unexpected fallback jwt kid: %s", cfg.Security.Keys.JWT.Active.KID)
	}
	if cfg.Security.Keys.JWT.Active.PrivateKeySource != "file:/tmp/sso-private.pem" {
		t.Fatalf("unexpected fallback jwt private key: %s", cfg.Security.Keys.JWT.Active.PrivateKeySource)
	}
	if cfg.Security.Keys.JWT.Active.PublicKeySource != "file:/tmp/sso-public.pem" {
		t.Fatalf("unexpected fallback jwt public key: %s", cfg.Security.Keys.JWT.Active.PublicKeySource)
	}
	if !cfg.Security.Keys.JWT.Configured() {
		t.Fatal("expected fallback jwt config to be usable")
	}
}

func TestLoadKeepsExplicitChallengeJWTKeysWhenConfigured(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "application.yaml"), `
security:
  keys:
    jwt:
      active:
        kid: challenge-kid
        privateKeySource: file:/tmp/challenge-private.pem
        publicKeySource: file:/tmp/challenge-public.pem
sso:
  jwt:
    currentKid: sso-kid-test
    privateKeysByKid:
      sso-kid-test: file:/tmp/sso-private.pem
    publicKeysByKid:
      sso-kid-test: file:/tmp/sso-public.pem
`)

	cfg, err := Load(dir)
	if err != nil {
		t.Fatalf("load config: %v", err)
	}
	if cfg.Security.Keys.JWT.Active.KID != "challenge-kid" {
		t.Fatalf("unexpected explicit jwt kid: %s", cfg.Security.Keys.JWT.Active.KID)
	}
	if cfg.Security.Keys.JWT.Active.PrivateKeySource != "file:/tmp/challenge-private.pem" {
		t.Fatalf("unexpected explicit jwt private key: %s", cfg.Security.Keys.JWT.Active.PrivateKeySource)
	}
	if cfg.Security.Keys.JWT.Active.PublicKeySource != "file:/tmp/challenge-public.pem" {
		t.Fatalf("unexpected explicit jwt public key: %s", cfg.Security.Keys.JWT.Active.PublicKeySource)
	}
}

func writeFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("write file %s: %v", path, err)
	}
}

func containsString(values []string, target string) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}
