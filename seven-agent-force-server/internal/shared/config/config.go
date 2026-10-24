package config

import (
	"errors"
	"fmt"
	"net"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/CaixyPromise/seven-agent/force/internal/shared/federation"
	"github.com/go-viper/mapstructure/v2"
	"github.com/spf13/viper"
)

type Config struct {
	Seven         SevenConfig         `mapstructure:"seven"`
	Server        ServerConfig        `mapstructure:"server"`
	Logging       LoggingConfig       `mapstructure:"logging"`
	Datasource    DatasourceConfig    `mapstructure:"datasource"`
	Cache         CacheConfig         `mapstructure:"cache"`
	Security      SecurityConfig      `mapstructure:"security"`
	Challenge     ChallengeConfig     `mapstructure:"challenge"`
	Login         LoginConfig         `mapstructure:"login"`
	SSO           SSOConfig           `mapstructure:"sso"`
	ExternalLogin ExternalLoginConfig `mapstructure:"externalLogin"`
	Authorization AuthorizationConfig `mapstructure:"authorization"`
	Setup         SetupConfig         `mapstructure:"setup"`
	Admin         AdminConfig         `mapstructure:"admin"`
	File          FileConfig          `mapstructure:"file"`
	Email         EmailConfig         `mapstructure:"email"`
	Notification  NotificationConfig  `mapstructure:"notification"`
	Storage       StorageConfig       `mapstructure:"storage"`
	RabbitMQ      RabbitMQConfig      `mapstructure:"rabbitmq"`
	Microservice  MicroserviceConfig  `mapstructure:"microservice"`
	Platform      PlatformConfig      `mapstructure:"platform"`
	Docker        DockerConfig        `mapstructure:"docker"`
	Limiter       LimiterConfig       `mapstructure:"limiter"`
	Scheduler     SchedulerConfig     `mapstructure:"scheduler"`
	Observability ObservabilityConfig `mapstructure:"observability"`
	Workforce     WorkforceConfig     `mapstructure:"workforce"`
	ID            IDConfig            `mapstructure:"id"`
	Profile       string              `mapstructure:"-"`
	LoadedFiles   []string            `mapstructure:"-"`
}

type SevenConfig struct {
	Name string `mapstructure:"name"`
	Env  string `mapstructure:"env"`
}

type ServerConfig struct {
	Host                string        `mapstructure:"host"`
	Port                int           `mapstructure:"port"`
	ContextPath         string        `mapstructure:"contextPath"`
	ContextPathKebab    string        `mapstructure:"context-path"`
	MaxRequestBodyBytes int           `mapstructure:"maxRequestBodyBytes"`
	ReadTimeout         time.Duration `mapstructure:"readTimeout"`
	WriteTimeout        time.Duration `mapstructure:"writeTimeout"`
	IdleTimeout         time.Duration `mapstructure:"idleTimeout"`
}

type LoggingConfig struct {
	Level   string               `mapstructure:"level"`
	Format  string               `mapstructure:"format"`
	Request RequestLoggingConfig `mapstructure:"request"`
	File    FileLoggingConfig    `mapstructure:"file"`
}

type RequestLoggingConfig struct {
	Enabled          bool     `mapstructure:"enabled"`
	MaxBodyBytes     int      `mapstructure:"maxBodyBytes"`
	MaxFieldLength   int      `mapstructure:"maxFieldLength"`
	IncludeQuery     bool     `mapstructure:"includeQuery"`
	MaskedFields     []string `mapstructure:"maskedFields"`
	SkipContentTypes []string `mapstructure:"skipContentTypes"`
}

type FileLoggingConfig struct {
	Enabled    bool   `mapstructure:"enabled"`
	Path       string `mapstructure:"path"`
	MaxSizeMB  int    `mapstructure:"maxSizeMB"`
	MaxBackups int    `mapstructure:"maxBackups"`
	MaxAgeDays int    `mapstructure:"maxAgeDays"`
	Compress   bool   `mapstructure:"compress"`
	LocalTime  bool   `mapstructure:"localTime"`
}

type DatasourceConfig struct {
	Driver    string                    `mapstructure:"driver"`
	Bootstrap DatasourceBootstrapConfig `mapstructure:"bootstrap"`
	MySQL     MySQLConfig               `mapstructure:"mysql"`
	Postgres  PostgresConfig            `mapstructure:"postgres"`
}

type DatasourceBootstrapMode string

const (
	BootstrapModeManual  DatasourceBootstrapMode = "manual"
	BootstrapModeStartup DatasourceBootstrapMode = "startup"
	BootstrapModeBoth    DatasourceBootstrapMode = "both"
)

type DatasourceBootstrapConfig struct {
	Enabled         bool                    `mapstructure:"enabled"`
	Mode            DatasourceBootstrapMode `mapstructure:"mode"`
	MigrationsDir   string                  `mapstructure:"migrationsDir"`
	VersionTable    string                  `mapstructure:"versionTable"`
	ChangeOwner     string                  `mapstructure:"changeOwner"`
	BaselineVersion string                  `mapstructure:"baselineVersion"`
	AllowLegacySync bool                    `mapstructure:"allowLegacySync"`
}

type MySQLConfig struct {
	Enabled         bool          `mapstructure:"enabled"`
	DSN             string        `mapstructure:"dsn"`
	MaxOpenConns    int           `mapstructure:"maxOpenConns"`
	MaxIdleConns    int           `mapstructure:"maxIdleConns"`
	ConnMaxLifetime time.Duration `mapstructure:"connMaxLifetime"`
	ConnMaxIdleTime time.Duration `mapstructure:"connMaxIdleTime"`
}

type PostgresConfig struct {
	Enabled         bool          `mapstructure:"enabled"`
	DSN             string        `mapstructure:"dsn"`
	MaxOpenConns    int           `mapstructure:"maxOpenConns"`
	MaxIdleConns    int           `mapstructure:"maxIdleConns"`
	ConnMaxLifetime time.Duration `mapstructure:"connMaxLifetime"`
	ConnMaxIdleTime time.Duration `mapstructure:"connMaxIdleTime"`
}

type CacheConfig struct {
	Enabled    bool                  `mapstructure:"enabled"`
	Prefix     string                `mapstructure:"prefix"`
	Codec      string                `mapstructure:"codec"`
	L1         CacheL1Config         `mapstructure:"l1"`
	Redis      RedisCacheConfig      `mapstructure:"redis"`
	Governance CacheGovernanceConfig `mapstructure:"governance"`
}

// CacheGovernanceConfig is opt-in because the DG5 protocol requires a real
// transaction Outbox, Redis generation store, and RabbitMQ consumer. There is
// no silent local-only compatibility mode.
type CacheGovernanceConfig struct {
	Enabled              bool          `mapstructure:"enabled"`
	InstanceID           string        `mapstructure:"instanceId"`
	RelayInterval        time.Duration `mapstructure:"relayInterval"`
	RelayBatch           int           `mapstructure:"relayBatch"`
	GlobalRefreshEnabled bool          `mapstructure:"globalRefreshEnabled"`
}

type CacheL1Config struct {
	Enabled     bool          `mapstructure:"enabled"`
	MaxCost     int64         `mapstructure:"maxCost"`
	NumCounters int64         `mapstructure:"numCounters"`
	BufferItems int64         `mapstructure:"bufferItems"`
	DefaultTTL  time.Duration `mapstructure:"defaultTTL"`
}

type RedisCacheMode string

const (
	RedisCacheModeSingle   RedisCacheMode = "single"
	RedisCacheModeSentinel RedisCacheMode = "sentinel"
	RedisCacheModeCluster  RedisCacheMode = "cluster"
)

type RedisCacheConfig struct {
	Enabled      bool                `mapstructure:"enabled"`
	Mode         RedisCacheMode      `mapstructure:"mode"`
	KeyPrefix    string              `mapstructure:"keyPrefix"`
	Database     int                 `mapstructure:"database"`
	Username     string              `mapstructure:"username"`
	Password     string              `mapstructure:"password"`
	ClientName   string              `mapstructure:"clientName"`
	DialTimeout  time.Duration       `mapstructure:"dialTimeout"`
	ReadTimeout  time.Duration       `mapstructure:"readTimeout"`
	WriteTimeout time.Duration       `mapstructure:"writeTimeout"`
	PoolSize     int                 `mapstructure:"poolSize"`
	MinIdleConns int                 `mapstructure:"minIdleConns"`
	Single       RedisSingleConfig   `mapstructure:"single"`
	Sentinel     RedisSentinelConfig `mapstructure:"sentinel"`
	Cluster      RedisClusterConfig  `mapstructure:"cluster"`
}

type RedisSingleConfig struct {
	Addr string `mapstructure:"addr"`
}

type RedisSentinelConfig struct {
	MasterName string   `mapstructure:"masterName"`
	Addrs      []string `mapstructure:"addrs"`
}

type RedisClusterConfig struct {
	Addrs []string `mapstructure:"addrs"`
}

type IDConfig struct {
	Node int64 `mapstructure:"node"`
}

// WorkforceConfig enables the Seven Agent Force control-plane module inside the
// existing Seven Framework server. The host Worker remains a separate process;
// these settings are shared so both processes use the same admitted Harness
// release, task queue and artifact store without creating a second Web system.
type WorkforceConfig struct {
	Enabled         bool                           `mapstructure:"enabled"`
	Artifact        WorkforceArtifactConfig        `mapstructure:"artifact"`
	Worker          WorkforceWorkerConfig          `mapstructure:"worker"`
	Harness         WorkforceHarnessConfig         `mapstructure:"harness"`
	ProviderProxy   WorkforceProviderProxyConfig   `mapstructure:"providerProxy"`
	ProviderSSRF    WorkforceProviderSSRFConfig    `mapstructure:"providerSSRF"`
	ManagedHooks    WorkforceManagedHooksConfig    `mapstructure:"managedHooks"`
	Reviewer        WorkforceReviewerConfig        `mapstructure:"reviewer"`
	OpenIntegration WorkforceOpenIntegrationConfig `mapstructure:"openIntegration"`
}

type WorkforceOpenIntegrationConfig struct {
	Enabled                   bool          `mapstructure:"enabled"`
	AllowLocalWebhookEndpoint bool          `mapstructure:"allowLocalWebhookEndpoint"`
	ProjectorInterval         time.Duration `mapstructure:"projectorInterval"`
	DeliveryBatchSize         int           `mapstructure:"deliveryBatchSize"`
	DeliveryLeaseSeconds      int           `mapstructure:"deliveryLeaseSeconds"`
	HTTPTimeoutSeconds        int           `mapstructure:"httpTimeoutSeconds"`
}

type WorkforceManagedHooksConfig struct {
	Enabled   bool   `mapstructure:"enabled"`
	CacheRoot string `mapstructure:"cacheRoot"`
}

type WorkforceArtifactConfig struct {
	Endpoint  string `mapstructure:"endpoint"`
	AccessKey string `mapstructure:"accessKey"`
	SecretKey string `mapstructure:"secretKey"`
	Bucket    string `mapstructure:"bucket"`
	UseTLS    bool   `mapstructure:"useTLS"`
}

type WorkforceWorkerConfig struct {
	ID                  string        `mapstructure:"id"`
	ServerBaseURL       string        `mapstructure:"serverBaseURL"`
	TokenFile           string        `mapstructure:"tokenFile"`
	MaxActiveContainers int           `mapstructure:"maxActiveContainers"`
	DockerHost          string        `mapstructure:"dockerHost"`
	WorkspaceRoot       string        `mapstructure:"workspaceRoot"`
	PollInterval        time.Duration `mapstructure:"pollInterval"`
}

type WorkforceHarnessConfig struct {
	Alias           string `mapstructure:"alias"`
	Image           string `mapstructure:"image"`
	Contract        string `mapstructure:"contract"`
	AllowLocalImage bool   `mapstructure:"allowLocalImage"`
}

type WorkforceProviderProxyConfig struct {
	Image       string `mapstructure:"image"`
	ImageDigest string `mapstructure:"imageDigest"`
	Version     string `mapstructure:"version"`
	ControlRoot string `mapstructure:"controlRoot"`
}

type WorkforceProviderSSRFConfig struct {
	Enabled                bool                              `mapstructure:"enabled"`
	AllowedList            []WorkforceProviderSSRFAllowEntry `mapstructure:"allowedList"`
	MaxRequestBytes        int64                             `mapstructure:"maxRequestBytes"`
	MaxResponseHeaderBytes int64                             `mapstructure:"maxResponseHeaderBytes"`
	RequestTimeout         time.Duration                     `mapstructure:"requestTimeout"`
	ProbeTimeout           time.Duration                     `mapstructure:"probeTimeout"`
	ResponseHeaderTimeout  time.Duration                     `mapstructure:"responseHeaderTimeout"`
	TLSHandshakeTimeout    time.Duration                     `mapstructure:"tlsHandshakeTimeout"`
}

type WorkforceProviderSSRFAllowEntry struct {
	Hostname      string   `mapstructure:"hostname"`
	CIDRs         []string `mapstructure:"cidrs"`
	Ports         []int    `mapstructure:"ports"`
	AllowFakeIP   bool     `mapstructure:"allowFakeIP"`
	AllowLoopback bool     `mapstructure:"allowLoopback"`
}

type WorkforceReviewerConfig struct {
	Enabled bool `mapstructure:"enabled"`
}

type SecurityConfig struct {
	OriginPatterns []string       `mapstructure:"originPatterns"`
	Password       PasswordConfig `mapstructure:"password"`
	Random         RandomConfig   `mapstructure:"random"`
	Keys           KeysConfig     `mapstructure:"keys"`
}

type ChallengeConfig struct {
	SessionTTLSeconds               int      `mapstructure:"sessionTtlSeconds"`
	ProofTokenTTLMinSeconds         int      `mapstructure:"proofTokenTtlMinSeconds"`
	ProofTokenTTLMaxSeconds         int      `mapstructure:"proofTokenTtlMaxSeconds"`
	PasswordMaxAttempts             int      `mapstructure:"passwordMaxAttempts"`
	PasswordCooldownSeconds         int      `mapstructure:"passwordCooldownSeconds"`
	ImageMaxAttempts                int      `mapstructure:"imageMaxAttempts"`
	ImageCooldownSeconds            int      `mapstructure:"imageCooldownSeconds"`
	OTPMaxAttempts                  int      `mapstructure:"otpMaxAttempts"`
	OTPCooldownSeconds              int      `mapstructure:"otpCooldownSeconds"`
	OTPIssuerName                   string   `mapstructure:"otpIssuerName"`
	OTPAllowedDriftWindows          int      `mapstructure:"otpAllowedDriftWindows"`
	RecoveryMaxAttempts             int      `mapstructure:"recoveryMaxAttempts"`
	RecoveryCooldownSeconds         int      `mapstructure:"recoveryCooldownSeconds"`
	RecoveryBatchSize               int      `mapstructure:"recoveryBatchSize"`
	EmailMaxAttempts                int      `mapstructure:"emailMaxAttempts"`
	EmailCooldownSeconds            int      `mapstructure:"emailCooldownSeconds"`
	TriggerMaxAttempts              int      `mapstructure:"triggerMaxAttempts"`
	ThrottleMaxFailures             int      `mapstructure:"throttleMaxFailures"`
	ThrottleWindowSeconds           int      `mapstructure:"throttleWindowSeconds"`
	ThrottleLockSeconds             int      `mapstructure:"throttleLockSeconds"`
	WebAuthnRPID                    string   `mapstructure:"webauthnRpId"`
	WebAuthnRPName                  string   `mapstructure:"webauthnRpName"`
	WebAuthnAllowedOrigins          []string `mapstructure:"webauthnAllowedOrigins"`
	WebAuthnChallengeTimeoutSeconds int      `mapstructure:"webauthnChallengeTimeoutSeconds"`
}

type LoginConfig struct {
	Enabled               bool `mapstructure:"enabled"`
	InteractionTTLSeconds int  `mapstructure:"interactionTtlSeconds"`
	CaptchaThreshold      int  `mapstructure:"captchaThreshold"`
	TOTPThreshold         int  `mapstructure:"totpThreshold"`
	LockThreshold         int  `mapstructure:"lockThreshold"`
	ContextLockThreshold  int  `mapstructure:"contextLockThreshold"`
	LockDurationHours     int  `mapstructure:"lockDurationHours"`
}

type ExternalLoginConfig struct {
	Enabled               bool          `mapstructure:"enabled"`
	CallbackBaseURL       string        `mapstructure:"callbackBaseUrl"`
	EnterpriseFakeBaseURL string        `mapstructure:"enterpriseFakeBaseUrl"`
	StateTTLSeconds       int           `mapstructure:"stateTtlSeconds"`
	HTTPTimeout           time.Duration `mapstructure:"httpTimeout"`
	FailClosed            bool          `mapstructure:"failClosed"`
}

type SSOConfig struct {
	Enabled                    bool                   `mapstructure:"enabled"`
	Issuer                     string                 `mapstructure:"issuer"`
	BaseURL                    string                 `mapstructure:"baseUrl"`
	DefaultFirstPartyClientID  string                 `mapstructure:"defaultFirstPartyClientId"`
	FrontendPrimaryEnabled     bool                   `mapstructure:"frontendPrimaryEnabled"`
	FrontendLoginURL           string                 `mapstructure:"frontendLoginUrl"`
	ResourceServerEnabled      bool                   `mapstructure:"resourceServerEnabled"`
	LoginTransactionTTLSeconds int                    `mapstructure:"loginTransactionTtlSeconds"`
	SessionIdleTimeoutSeconds  int                    `mapstructure:"sessionIdleTimeoutSeconds"`
	SessionTouchThrottleSecond int                    `mapstructure:"sessionTouchThrottleSeconds"`
	UserinfoTouchThrottleSec   int                    `mapstructure:"userinfoTouchThrottleSeconds"`
	RefreshReplayClockSkewSec  int                    `mapstructure:"refreshReplayClockSkewSeconds"`
	RateLimit                  SSORateLimitConfig     `mapstructure:"rateLimit"`
	SessionCookie              SSOCookieConfig        `mapstructure:"sessionCookie"`
	RefreshCookie              SSORefreshCookieConfig `mapstructure:"refreshCookie"`
	JWT                        SSOJWTConfig           `mapstructure:"jwt"`
}

type SSORateLimitConfig struct {
	TokenLimit        int64         `mapstructure:"tokenLimit"`
	TokenWindow       time.Duration `mapstructure:"tokenWindow"`
	UserInfoLimit     int64         `mapstructure:"userinfoLimit"`
	UserInfoWindow    time.Duration `mapstructure:"userinfoWindow"`
	FailClosedOnError bool          `mapstructure:"failClosedOnError"`
}

type SSOCookieConfig struct {
	Name     string `mapstructure:"name"`
	Path     string `mapstructure:"path"`
	SameSite string `mapstructure:"sameSite"`
	Secure   bool   `mapstructure:"secure"`
}

type SSORefreshCookieConfig struct {
	Name     string `mapstructure:"name"`
	Path     string `mapstructure:"path"`
	SameSite string `mapstructure:"sameSite"`
	Secure   bool   `mapstructure:"secure"`
	HTTPOnly bool   `mapstructure:"httpOnly"`
}

type SSOJWTConfig struct {
	CurrentKID       string            `mapstructure:"currentKid"`
	NextKID          string            `mapstructure:"nextKid"`
	PrivateKeysByKID map[string]string `mapstructure:"privateKeysByKid"`
	PublicKeysByKID  map[string]string `mapstructure:"publicKeysByKid"`
	KeyStatusByKID   map[string]string `mapstructure:"keyStatusByKid"`
}

type AuthorizationMode string

const (
	AuthorizationModeLocal  AuthorizationMode = "local"
	AuthorizationModeRemote AuthorizationMode = "remote"
)

type AuthorizationConfig struct {
	Enabled       bool                        `mapstructure:"enabled"`
	Mode          AuthorizationMode           `mapstructure:"mode"`
	AnonymousURLs []string                    `mapstructure:"anonymousUrls"`
	Gateway       AuthorizationGatewayConfig  `mapstructure:"gateway"`
	Cache         AuthorizationCacheConfig    `mapstructure:"cache"`
	Remote        AuthorizationRemoteConfig   `mapstructure:"remote"`
	Internal      AuthorizationInternalConfig `mapstructure:"internal"`
	Network       AuthorizationNetworkConfig  `mapstructure:"network"`
}

type AuthorizationGatewayConfig struct {
	SignatureEnabled          bool              `mapstructure:"signatureEnabled"`
	SignatureVersion          string            `mapstructure:"signatureVersion"`
	AcceptedSignatureVersions []string          `mapstructure:"acceptedVersions"`
	SecretsByVersion          map[string]string `mapstructure:"secretsByVersion"`
	Secret                    string            `mapstructure:"secret"`
	TimestampToleranceSeconds int64             `mapstructure:"timestampToleranceSeconds"`
}

type AuthorizationCacheConfig struct {
	Enabled    bool   `mapstructure:"enabled"`
	TTLSeconds int    `mapstructure:"ttlSeconds"`
	KeyPrefix  string `mapstructure:"keyPrefix"`
}

type AuthorizationRemoteConfig struct {
	ServiceName          string `mapstructure:"serviceName"`
	ServiceURL           string `mapstructure:"serviceUrl"`
	InternalBasePath     string `mapstructure:"internalBasePath"`
	SSOIssuer            string `mapstructure:"ssoIssuer"`
	SSOJWKSURI           string `mapstructure:"ssoJwksUri"`
	SSOAudience          string `mapstructure:"ssoAudience"`
	TimeoutMilliseconds  int    `mapstructure:"timeoutMilliseconds"`
	FailClosed           bool   `mapstructure:"failClosed"`
	AcceptGatewayHeaders bool   `mapstructure:"acceptGatewayHeaders"`
}

type AuthorizationInternalConfig struct {
	Enabled              bool   `mapstructure:"enabled"`
	HeaderName           string `mapstructure:"headerName"`
	Token                string `mapstructure:"token"`
	SignatureEnabled     bool   `mapstructure:"signatureEnabled"`
	SignatureSecret      string `mapstructure:"signatureSecret"`
	TimestampToleranceMs int64  `mapstructure:"timestampToleranceMs"`
	NonceTTLSeconds      int    `mapstructure:"nonceTtlSeconds"`
	NonceMinLength       int    `mapstructure:"nonceMinLength"`
	NonceMaxLength       int    `mapstructure:"nonceMaxLength"`
}

type AuthorizationNetworkConfig struct {
	TrustForwardHeaders bool     `mapstructure:"trustForwardHeaders"`
	TrustedProxies      []string `mapstructure:"trustedProxies"`
	TrustedCIDRs        []string `mapstructure:"trustedCidrs"`
}

type SetupConfig struct {
	Enabled                   bool                 `mapstructure:"enabled"`
	TokenSecret               string               `mapstructure:"tokenSecret"`
	TokenTTLSeconds           int64                `mapstructure:"tokenTtlSeconds"`
	OwnerBootstrapLockSeconds int64                `mapstructure:"ownerBootstrapLockSeconds"`
	BootstrapClientID         string               `mapstructure:"bootstrapClientId"`
	AllowedOriginPatterns     []string             `mapstructure:"allowedOriginPatterns"`
	RequireOriginHeader       bool                 `mapstructure:"requireOriginHeader"`
	Bootstrap                 SetupBootstrapConfig `mapstructure:"bootstrap"`
}

type SetupBootstrapConfig struct {
	SuperAdminRoleCode string `mapstructure:"superAdminRoleCode"`
	SuperAdminRoleName string `mapstructure:"superAdminRoleName"`
}

type AdminConfig struct {
	RuntimeLog AdminRuntimeLogConfig `mapstructure:"runtimeLog"`
}

type AdminRuntimeLogConfig struct {
	Enabled               bool          `mapstructure:"enabled"`
	BaseDir               string        `mapstructure:"baseDir"`
	ActiveFile            string        `mapstructure:"activeFile"`
	MaxSearchWindowDays   int           `mapstructure:"maxSearchWindowDays"`
	MaxPageSize           int           `mapstructure:"maxPageSize"`
	MaxScanLines          int           `mapstructure:"maxScanLines"`
	TailPollIntervalMs    int64         `mapstructure:"tailPollIntervalMs"`
	HeartbeatIntervalMs   int64         `mapstructure:"heartbeatIntervalMs"`
	MaxGlobalConnections  int           `mapstructure:"maxGlobalConnections"`
	MaxConnectionsPerUser int           `mapstructure:"maxConnectionsPerUser"`
	DefaultLastN          int           `mapstructure:"defaultLastN"`
	MaxLastN              int           `mapstructure:"maxLastN"`
	EmitterTimeoutMs      int64         `mapstructure:"emitterTimeoutMs"`
	DefaultHistoryWindow  time.Duration `mapstructure:"defaultHistoryWindow"`
}

type StorageConfig struct {
	Location     string `mapstructure:"location"`
	ForceCreated bool   `mapstructure:"forceCreated"`
	StaticPath   string `mapstructure:"staticPath"`
}

type FileConfig struct {
	Enabled      bool                         `mapstructure:"enabled"`
	Binding      FileBindingConfig            `mapstructure:"binding"`
	ChunkUpload  FileChunkUploadConfig        `mapstructure:"chunkUpload"`
	DirectUpload FileDirectUploadConfig       `mapstructure:"directUpload"`
	Distribution FileDistributionConfig       `mapstructure:"distribution"`
	Outbox       FileOutboxConfig             `mapstructure:"outbox"`
	ProcessTask  FileProcessTaskConfig        `mapstructure:"processTask"`
	Cleanup      FileCleanupConfig            `mapstructure:"cleanup"`
	HealthCheck  FileHealthCheckConfig        `mapstructure:"healthCheck"`
	DefaultPost  FileDefaultPostProcessConfig `mapstructure:"defaultPost"`
	Rabbit       FileRabbitConfig             `mapstructure:"rabbit"`
}

type FileBindingConfig struct {
	MaxRetries          int                         `mapstructure:"maxRetries"`
	RetryDelaySeconds   int                         `mapstructure:"retryDelaySeconds"`
	FailedRetentionDays int                         `mapstructure:"failedRetentionDays"`
	RetryBatchSize      int                         `mapstructure:"retryBatchSize"`
	Routes              map[string]FileBindingRoute `mapstructure:"routes"`
}

type FileBindingRoute struct {
	Mode           string `mapstructure:"mode"`
	ServiceID      string `mapstructure:"serviceId"`
	BaseURL        string `mapstructure:"baseUrl"`
	EndpointPrefix string `mapstructure:"endpointPrefix"`
}

type FileChunkUploadConfig struct {
	ExpireHours    int    `mapstructure:"expireHours"`
	MaxChunkSize   int64  `mapstructure:"maxChunkSize"`
	MaxRequestRate int    `mapstructure:"maxRequestRate"`
	BufferSize     int    `mapstructure:"bufferSize"`
	TempDirectory  string `mapstructure:"tempDirectory"`
}

type FileDirectUploadConfig struct {
	PresignTTLSeconds   int     `mapstructure:"presignTtlSeconds"`
	DownloadTTLSeconds  int     `mapstructure:"downloadTtlSeconds"`
	TaskExpireHours     int     `mapstructure:"taskExpireHours"`
	MultipartThreshold  int64   `mapstructure:"multipartThresholdBytes"`
	PartSizeBytes       int64   `mapstructure:"partSizeBytes"`
	StagingPrefix       string  `mapstructure:"stagingPrefix"`
	CleanPrefix         string  `mapstructure:"cleanPrefix"`
	TaskMaxRetries      int     `mapstructure:"taskMaxRetries"`
	TaskRetryDelaysMS   []int64 `mapstructure:"taskRetryDelaysMs"`
	TaskDLQMaxRetries   int     `mapstructure:"taskDlqMaxRetries"`
	TaskDLQRetryDelayMS int64   `mapstructure:"taskDlqRetryDelayMs"`
}

type FileDistributionConfig struct {
	GatewayPath              string `mapstructure:"gatewayPath"`
	SignedURLSecret          string `mapstructure:"signedUrlSecret"`
	SignedURLTTLSeconds      int    `mapstructure:"signedUrlTtlSeconds"`
	AllowIPBind              bool   `mapstructure:"allowIpBind"`
	OneTimeToken             bool   `mapstructure:"oneTimeToken"`
	RangeEnabled             bool   `mapstructure:"rangeEnabled"`
	CacheControlPublic       string `mapstructure:"cacheControlPublic"`
	CacheControlPrivate      string `mapstructure:"cacheControlPrivate"`
	HotlinkProtectionEnabled bool   `mapstructure:"hotlinkProtectionEnabled"`
	HotlinkAllowedDomains    string `mapstructure:"hotlinkAllowedDomains"`
	AllowEmptyReferer        bool   `mapstructure:"allowEmptyReferer"`
}

type FileOutboxConfig struct {
	Enabled         bool `mapstructure:"enabled"`
	LegacyDirectMQ  bool `mapstructure:"legacyDirectMq"`
	RelayIntervalMS int  `mapstructure:"relayIntervalMs"`
	BatchSize       int  `mapstructure:"batchSize"`
}

type FileProcessTaskConfig struct {
	TimeoutSeconds  int    `mapstructure:"timeoutSeconds"`
	OutputDirectory string `mapstructure:"outputDirectory"`
	RetryIntervalMS int    `mapstructure:"retryIntervalMs"`
	RetryBatchSize  int    `mapstructure:"retryBatchSize"`
}

type FileCleanupConfig struct {
	DeletedFileRetentionDays    int `mapstructure:"deletedFileRetentionDays"`
	UnreferencedFileCleanupDays int `mapstructure:"unreferencedFileCleanupDays"`
	ChunkCleanupIntervalMS      int `mapstructure:"chunkCleanupIntervalMs"`
	BatchSize                   int `mapstructure:"batchSize"`
}

type FileHealthCheckConfig struct {
	MinRequestsForFailover int `mapstructure:"minRequestsForFailover"`
	IntervalMS             int `mapstructure:"intervalMs"`
}

type FileDefaultPostProcessConfig struct {
	EnableThumbnail        bool     `mapstructure:"enableThumbnail"`
	EnableCompress         bool     `mapstructure:"enableCompress"`
	CompressThresholdBytes int64    `mapstructure:"compressThresholdBytes"`
	ThumbnailSpecs         []string `mapstructure:"thumbnailSpecs"`
	CompressType           string   `mapstructure:"compressType"`
}

type FileRabbitConfig struct {
	Enabled bool `mapstructure:"enabled"`
}

type RabbitMQConfig struct {
	Enabled      bool          `mapstructure:"enabled"`
	URL          string        `mapstructure:"url"`
	Host         string        `mapstructure:"host"`
	Port         int           `mapstructure:"port"`
	Username     string        `mapstructure:"username"`
	Password     string        `mapstructure:"password"`
	VHost        string        `mapstructure:"vhost"`
	Prefetch     int           `mapstructure:"prefetch"`
	ReconnectMin time.Duration `mapstructure:"reconnectMin"`
	ReconnectMax time.Duration `mapstructure:"reconnectMax"`
	Declare      bool          `mapstructure:"declare"`
}

type MicroserviceConfig struct {
	Enabled        bool                             `mapstructure:"enabled"`
	InternalServer MicroserviceInternalServerConfig `mapstructure:"internalServer"`
	Service        MicroserviceServiceConfig        `mapstructure:"service"`
	Registry       MicroserviceRegistryConfig       `mapstructure:"registry"`
	Health         MicroserviceHealthConfig         `mapstructure:"health"`
	Discovery      MicroserviceDiscoveryConfig      `mapstructure:"discovery"`
	Static         MicroserviceStaticConfig         `mapstructure:"static"`
	Client         MicroserviceClientConfig         `mapstructure:"client"`
	Outbound       MicroserviceOutboundConfig       `mapstructure:"outbound"`
}

type MicroserviceInternalServerConfig struct {
	Enabled bool   `mapstructure:"enabled"`
	Listen  string `mapstructure:"listen"`
}

type MicroserviceServiceConfig struct {
	Name           string            `mapstructure:"name"`
	InstanceID     string            `mapstructure:"instanceId"`
	AdvertisedHost string            `mapstructure:"advertisedHost"`
	AdvertisedPort int               `mapstructure:"advertisedPort"`
	Scheme         string            `mapstructure:"scheme"`
	Tags           []string          `mapstructure:"tags"`
	Metadata       map[string]string `mapstructure:"metadata"`
}

type MicroserviceRegistryConfig struct {
	Enabled              bool          `mapstructure:"enabled"`
	Type                 string        `mapstructure:"type"`
	Address              string        `mapstructure:"address"`
	Datacenter           string        `mapstructure:"datacenter"`
	Token                string        `mapstructure:"token"`
	RegistrationRequired bool          `mapstructure:"registrationRequired"`
	RegisterTimeout      time.Duration `mapstructure:"registerTimeout"`
	DeregisterTimeout    time.Duration `mapstructure:"deregisterTimeout"`
}

type MicroserviceHealthConfig struct {
	Interval time.Duration `mapstructure:"interval"`
	Timeout  time.Duration `mapstructure:"timeout"`
}

type MicroserviceDiscoveryConfig struct {
	Type                  string        `mapstructure:"type"`
	CacheTTL              time.Duration `mapstructure:"cacheTtl"`
	EmptyResultTTL        time.Duration `mapstructure:"emptyResultTtl"`
	ResolveTimeout        time.Duration `mapstructure:"resolveTimeout"`
	Datacenter            string        `mapstructure:"datacenter"`
	Tags                  []string      `mapstructure:"tags"`
	StaticFallbackEnabled bool          `mapstructure:"staticFallbackEnabled"`
}

type MicroserviceStaticConfig struct {
	Services map[string][]MicroserviceStaticInstanceConfig `mapstructure:"services"`
}

type MicroserviceStaticInstanceConfig struct {
	URL string `mapstructure:"url"`
}

type MicroserviceClientConfig struct {
	ConnectTimeout      time.Duration `mapstructure:"connectTimeout"`
	RequestTimeout      time.Duration `mapstructure:"requestTimeout"`
	MaxIdleConns        int           `mapstructure:"maxIdleConns"`
	MaxIdleConnsPerHost int           `mapstructure:"maxIdleConnsPerHost"`
	IdleConnTimeout     time.Duration `mapstructure:"idleConnTimeout"`
	MaxRequestBytes     int64         `mapstructure:"maxRequestBytes"`
	MaxResponseBytes    int64         `mapstructure:"maxResponseBytes"`
}

type MicroserviceOutboundConfig struct {
	TrustedHosts         []string `mapstructure:"trustedHosts"`
	TrustedCIDRs         []string `mapstructure:"trustedCidrs"`
	RegistryTrustedHosts []string `mapstructure:"registryTrustedHosts"`
	RegistryTrustedCIDRs []string `mapstructure:"registryTrustedCidrs"`
}

type PlatformMode string

const (
	PlatformModeLocal PlatformMode = "local"
	PlatformModeHub   PlatformMode = "hub"
	PlatformModeNode  PlatformMode = "node"
)

type PlatformConfig struct {
	Mode             PlatformMode       `mapstructure:"mode"`
	PublicWebBaseURL string             `mapstructure:"publicWebBaseUrl"`
	Node             PlatformNodeConfig `mapstructure:"node"`
}

type PlatformNodeConfig struct {
	Code             string                             `mapstructure:"code"`
	ManagementBearer string                             `mapstructure:"managementBearer"`
	InternalListener PlatformNodeInternalListenerConfig `mapstructure:"internalListener"`
}

type PlatformNodeInternalListenerConfig struct {
	Enabled bool   `mapstructure:"enabled"`
	Listen  string `mapstructure:"listen"`
}

type PlatformCapabilities struct {
	ControlPlane      bool `json:"controlPlane"`
	FederatedHubLogin bool `json:"federatedHubLogin"`
	NodeAPI           bool `json:"nodeApi"`
}

type DockerConfig struct {
	Enabled   bool                  `mapstructure:"enabled"`
	FailFast  bool                  `mapstructure:"failFast"`
	Provider  string                `mapstructure:"provider"`
	Engine    DockerEngineConfig    `mapstructure:"engine"`
	Compose   DockerComposeConfig   `mapstructure:"compose"`
	Registry  DockerRegistryConfig  `mapstructure:"registry"`
	Operation DockerOperationConfig `mapstructure:"operation"`
	Security  DockerSecurityConfig  `mapstructure:"security"`
}

type DockerEngineConfig struct {
	Host                  string        `mapstructure:"host"`
	APIVersion            string        `mapstructure:"apiVersion"`
	Timeout               time.Duration `mapstructure:"timeout"`
	APIVersionNegotiation bool          `mapstructure:"apiVersionNegotiation"`
}

type DockerComposeConfig struct {
	Binary                     string        `mapstructure:"binary"`
	TempDir                    string        `mapstructure:"tempDir"`
	Timeout                    time.Duration `mapstructure:"timeout"`
	OutputMax                  int           `mapstructure:"outputMax"`
	WorkspaceRoots             []string      `mapstructure:"workspaceRoots"`
	DefaultFileName            string        `mapstructure:"defaultFileName"`
	DirMode                    uint32        `mapstructure:"dirMode"`
	FileMode                   uint32        `mapstructure:"fileMode"`
	MaxComposeBytes            int           `mapstructure:"maxComposeBytes"`
	MaxDockerfileBytes         int           `mapstructure:"maxDockerfileBytes"`
	MaxExtraFilesBytes         int           `mapstructure:"maxExtraFilesBytes"`
	AllowedProjectFileSuffixes []string      `mapstructure:"allowedProjectFileSuffixes"`
}

type DockerRegistryConfig struct {
	Timeout          time.Duration `mapstructure:"timeout"`
	DefaultPageSize  int64         `mapstructure:"defaultPageSize"`
	MaxPageSize      int64         `mapstructure:"maxPageSize"`
	MaxPages         int64         `mapstructure:"maxPages"`
	InsecureFallback bool          `mapstructure:"insecureFallback"`
}

type DockerOperationConfig struct {
	MaxConcurrent       int           `mapstructure:"maxConcurrent"`
	MaxQueued           int           `mapstructure:"maxQueued"`
	DefaultTimeout      time.Duration `mapstructure:"defaultTimeout"`
	EventRetentionLimit int           `mapstructure:"eventRetentionLimit"`
	SSEHeartbeat        time.Duration `mapstructure:"sseHeartbeat"`
	PollInterval        time.Duration `mapstructure:"pollInterval"`
}

type DockerSecurityConfig struct {
	StrictContainerCreate bool     `mapstructure:"strictContainerCreate"`
	PolicyProfile         string   `mapstructure:"policyProfile"`
	TrustedRegistries     []string `mapstructure:"trustedRegistries"`
	TrustedImages         []string `mapstructure:"trustedImages"`
	AllowedNetworks       []string `mapstructure:"allowedNetworks"`
	AllowedVolumes        []string `mapstructure:"allowedVolumes"`
	SensitiveKeys         []string `mapstructure:"sensitiveKeys"`
}

type EmailConfig struct {
	Enabled     bool                `mapstructure:"enabled"`
	Provider    string              `mapstructure:"provider"`
	DefaultFrom string              `mapstructure:"defaultFrom"`
	AppName     string              `mapstructure:"appName"`
	SMTP        EmailSMTPConfig     `mapstructure:"smtp"`
	Mock        EmailMockConfig     `mapstructure:"mock"`
	Templates   EmailTemplateConfig `mapstructure:"templates"`
}

// NotificationConfig contains runtime-owned notification infrastructure
// settings. Channel rows may reference these policies by name but cannot
// redefine their network access rules.
type NotificationConfig struct {
	Outbound NotificationOutboundConfig `mapstructure:"outbound"`
}

// NotificationOutboundConfig holds the environment-owned egress exceptions
// used by guarded notification URL channels.
type NotificationOutboundConfig struct {
	Policies []NotificationOutboundPolicyConfig `mapstructure:"policies"`
}

// NotificationOutboundPolicyConfig describes one exact environment policy.
// It is deliberately separate from channel configuration so a connection
// cannot add private network, port or proxy permissions for itself.
type NotificationOutboundPolicyConfig struct {
	Name             string   `mapstructure:"name"`
	Mode             string   `mapstructure:"mode"`
	AllowedHostnames []string `mapstructure:"allowedHostnames"`
	AllowedCIDRs     []string `mapstructure:"allowedCidrs"`
	AllowedPorts     []int    `mapstructure:"allowedPorts"`
	ProxyURL         string   `mapstructure:"proxyUrl"`
}

type EmailSMTPConfig struct {
	Host       string        `mapstructure:"host"`
	Port       int           `mapstructure:"port"`
	Username   string        `mapstructure:"username"`
	Password   string        `mapstructure:"password"`
	From       string        `mapstructure:"from"`
	UseTLS     bool          `mapstructure:"useTls"`
	StartTLS   bool          `mapstructure:"startTls"`
	Timeout    time.Duration `mapstructure:"timeout"`
	SkipVerify bool          `mapstructure:"skipVerify"`
}

type EmailMockConfig struct {
	CapturePrefix string        `mapstructure:"capturePrefix"`
	TTL           time.Duration `mapstructure:"ttl"`
}

type EmailTemplateConfig struct {
	ChallengeOTP EmailTemplateSpec `mapstructure:"challengeOtp"`
}

type EmailTemplateSpec struct {
	Subject string `mapstructure:"subject"`
	Text    string `mapstructure:"text"`
	HTML    string `mapstructure:"html"`
}

type PasswordConfig struct {
	Algorithm string               `mapstructure:"algorithm"`
	Bcrypt    BcryptPasswordConfig `mapstructure:"bcrypt"`
}

type BcryptPasswordConfig struct {
	Cost int `mapstructure:"cost"`
}

type RandomConfig struct {
	TokenLength int `mapstructure:"tokenLength"`
	NonceLength int `mapstructure:"nonceLength"`
	CodeLength  int `mapstructure:"codeLength"`
}

type KeysConfig struct {
	Provider string           `mapstructure:"provider"`
	Master   MasterKeysConfig `mapstructure:"master"`
	JWT      JWTKeysConfig    `mapstructure:"jwt"`
}

type MasterKeysConfig struct {
	Active  MasterKeySourceConfig   `mapstructure:"active"`
	Retired []MasterKeySourceConfig `mapstructure:"retired"`
}

type MasterKeySourceConfig struct {
	KID    string `mapstructure:"kid"`
	Source string `mapstructure:"source"`
}

type JWTKeysConfig struct {
	Algorithm string               `mapstructure:"algorithm"`
	Active    JWTKeySourceConfig   `mapstructure:"active"`
	Next      JWTKeySourceConfig   `mapstructure:"next"`
	Retired   []JWTKeySourceConfig `mapstructure:"retired"`
}

type JWTKeySourceConfig struct {
	KID              string        `mapstructure:"kid"`
	PrivateKeySource string        `mapstructure:"privateKeySource"`
	PublicKeySource  string        `mapstructure:"publicKeySource"`
	VerifyUntil      time.Duration `mapstructure:"verifyUntil"`
}

type SchedulerConfig struct {
	Enabled  bool                `mapstructure:"enabled"`
	Timezone string              `mapstructure:"timezone"`
	Lock     SchedulerLockConfig `mapstructure:"lock"`
}

type SchedulerLockConfig struct {
	Enabled bool          `mapstructure:"enabled"`
	TTL     time.Duration `mapstructure:"ttl"`
}

type LimiterConfig struct {
	Enabled       bool          `mapstructure:"enabled"`
	KeyPrefix     string        `mapstructure:"keyPrefix"`
	DefaultLimit  int64         `mapstructure:"defaultLimit"`
	DefaultWindow time.Duration `mapstructure:"defaultWindow"`
	FailOpen      bool          `mapstructure:"failOpen"`
}

type ObservabilityConfig struct {
	Enabled            bool                          `mapstructure:"enabled"`
	SnapshotIntervalMs int64                         `mapstructure:"snapshotIntervalMs"`
	Prometheus         ObservabilityPrometheusConfig `mapstructure:"prometheus"`
	Tracing            ObservabilityTracingConfig    `mapstructure:"tracing"`
	Pprof              ObservabilityPprofConfig      `mapstructure:"pprof"`
	Logs               ObservabilityLogsConfig       `mapstructure:"logs"`
}

type ObservabilityPrometheusConfig struct {
	Enabled     bool   `mapstructure:"enabled"`
	Path        string `mapstructure:"path"`
	AccessToken string `mapstructure:"accessToken"`
}

type ObservabilityTracingConfig struct {
	Enabled       bool          `mapstructure:"enabled"`
	ServiceName   string        `mapstructure:"serviceName"`
	OTLPEndpoint  string        `mapstructure:"otlpEndpoint"`
	Insecure      bool          `mapstructure:"insecure"`
	ExportTimeout time.Duration `mapstructure:"exportTimeout"`
}

type ObservabilityPprofConfig struct {
	Enabled bool   `mapstructure:"enabled"`
	Prefix  string `mapstructure:"prefix"`
}

type ObservabilityLogsConfig struct {
	Enabled            bool  `mapstructure:"enabled"`
	RecentLimit        int   `mapstructure:"recentLimit"`
	ErrorLimit         int   `mapstructure:"errorLimit"`
	HotLoggerLimit     int   `mapstructure:"hotLoggerLimit"`
	TrendBucketSeconds int64 `mapstructure:"trendBucketSeconds"`
}

func Load(configDir string) (Config, error) {
	profile := resolveProfile()
	loader := viper.New()
	loader.SetConfigType("yaml")
	loader.SetEnvKeyReplacer(strings.NewReplacer(".", "_"))
	loader.AutomaticEnv()
	setDefaults(loader)
	_ = loader.BindEnv("server.contextPath", "SERVER_CONTEXT_PATH")
	if err := loader.BindEnv("platform.node.managementBearer", "NODE_MANAGEMENT_BEARER"); err != nil {
		return Config{}, fmt.Errorf("bind platform node management bearer environment: %w", err)
	}
	if err := loader.BindEnv("microservice.registry.address", "CONSUL_HTTP_ADDR"); err != nil {
		return Config{}, fmt.Errorf("bind Consul HTTP address environment: %w", err)
	}
	if err := loader.BindEnv("microservice.registry.token", "CONSUL_HTTP_TOKEN"); err != nil {
		return Config{}, fmt.Errorf("bind Consul HTTP token environment: %w", err)
	}

	loadedFiles := make([]string, 0, 2)
	baseFile, err := resolveExistingFile(configDir, "application")
	if err != nil {
		return Config{}, err
	}
	if err := mergeFile(loader, baseFile); err != nil {
		return Config{}, err
	}
	loadedFiles = append(loadedFiles, baseFile)

	profileFile, err := resolveOptionalFile(configDir, fmt.Sprintf("application-%s", profile))
	if err != nil {
		return Config{}, err
	}
	if profileFile != "" {
		if err := mergeFile(loader, profileFile); err != nil {
			return Config{}, err
		}
		loadedFiles = append(loadedFiles, profileFile)
	}

	var cfg Config
	decoderHook := viper.DecodeHook(mapstructure.ComposeDecodeHookFunc(
		mapstructure.StringToTimeDurationHookFunc(),
	))
	if err := loader.Unmarshal(&cfg, decoderHook); err != nil {
		return Config{}, fmt.Errorf("unmarshal config: %w", err)
	}

	cfg.Profile = profile
	cfg.LoadedFiles = loadedFiles
	if strings.TrimSpace(cfg.Server.ContextPath) == "" {
		cfg.Server.ContextPath = loader.GetString("server.context-path")
	}
	normalize(&cfg)
	if err := validateMicroserviceConfig(cfg.Microservice); err != nil {
		return Config{}, err
	}
	if err := validatePlatformConfig(cfg.Platform); err != nil {
		return Config{}, err
	}
	if err := validateWorkforceConfig(cfg); err != nil {
		return Config{}, err
	}
	if err := validateProductionSecurity(&cfg); err != nil {
		return Config{}, err
	}
	return cfg, nil
}

func validateWorkforceConfig(cfg Config) error {
	if !cfg.Workforce.Enabled {
		return nil
	}
	checks := []struct {
		ok      bool
		message string
	}{
		{cfg.Datasource.Driver == "postgres" && cfg.Datasource.Postgres.Enabled && strings.TrimSpace(cfg.Datasource.Postgres.DSN) != "", "workforce requires the configured PostgreSQL datasource"},
		{cfg.RabbitMQ.Enabled, "workforce requires rabbitmq.enabled"},
		{strings.TrimSpace(cfg.Workforce.Artifact.Endpoint) != "", "workforce.artifact.endpoint is required"},
		{strings.TrimSpace(cfg.Workforce.Artifact.AccessKey) != "", "workforce.artifact.accessKey is required"},
		{strings.TrimSpace(cfg.Workforce.Artifact.SecretKey) != "", "workforce.artifact.secretKey is required"},
		{strings.TrimSpace(cfg.Workforce.Artifact.Bucket) != "", "workforce.artifact.bucket is required"},
		{strings.TrimSpace(cfg.Workforce.Worker.ID) != "", "workforce.worker.id is required"},
		{cfg.Workforce.Worker.MaxActiveContainers >= 1, "workforce.worker.maxActiveContainers must be at least 1"},
		{strings.TrimSpace(cfg.Workforce.Worker.WorkspaceRoot) != "", "workforce.worker.workspaceRoot is required"},
		{strings.TrimSpace(cfg.Workforce.Harness.Alias) != "", "workforce.harness.alias is required"},
		{strings.TrimSpace(cfg.Workforce.Harness.Image) != "", "workforce.harness.image is required"},
		{cfg.Workforce.Harness.Contract == "seven.worker-image/v1", "workforce.harness.contract must be seven.worker-image/v1"},
	}
	for _, check := range checks {
		if !check.ok {
			return errors.New(check.message)
		}
	}
	if cfg.Workforce.OpenIntegration.Enabled {
		open := cfg.Workforce.OpenIntegration
		if open.ProjectorInterval <= 0 || open.DeliveryBatchSize < 1 || open.DeliveryBatchSize > 32 || open.DeliveryLeaseSeconds < 1 || open.HTTPTimeoutSeconds < 1 {
			return errors.New("workforce.openIntegration intervals, limits, lease, and timeout must be positive and deliveryBatchSize must not exceed 32")
		}
		if open.DeliveryLeaseSeconds < open.HTTPTimeoutSeconds+15 {
			return errors.New("workforce.openIntegration.deliveryLeaseSeconds must be at least 15 seconds greater than httpTimeoutSeconds")
		}
		if open.AllowLocalWebhookEndpoint && cfg.Platform.Mode != PlatformModeLocal {
			return errors.New("workforce.openIntegration.allowLocalWebhookEndpoint is allowed only in platform.mode=local")
		}
	}
	if cfg.Workforce.ProviderSSRF.MaxRequestBytes < 1 || cfg.Workforce.ProviderSSRF.MaxResponseHeaderBytes < 1 || cfg.Workforce.ProviderSSRF.RequestTimeout <= 0 || cfg.Workforce.ProviderSSRF.ProbeTimeout <= 0 || cfg.Workforce.ProviderSSRF.ResponseHeaderTimeout <= 0 || cfg.Workforce.ProviderSSRF.TLSHandshakeTimeout <= 0 {
		return errors.New("workforce.providerSSRF limits and timeouts must be positive")
	}
	seenProviderHosts := make(map[string]struct{}, len(cfg.Workforce.ProviderSSRF.AllowedList))
	for _, entry := range cfg.Workforce.ProviderSSRF.AllowedList {
		host := strings.ToLower(strings.TrimSuffix(strings.TrimSpace(entry.Hostname), "."))
		if host == "" || strings.Contains(host, "*") || ((!entry.AllowLoopback) && (host == "localhost" || strings.HasSuffix(host, ".localhost"))) {
			return fmt.Errorf("workforce.providerSSRF.allowedList hostname %q must be exact and non-local", entry.Hostname)
		}
		if _, exists := seenProviderHosts[host]; exists {
			return fmt.Errorf("workforce.providerSSRF.allowedList contains duplicate hostname %q", entry.Hostname)
		}
		seenProviderHosts[host] = struct{}{}
		if len(entry.CIDRs) == 0 || len(entry.Ports) == 0 {
			return fmt.Errorf("workforce.providerSSRF.allowedList entry %q requires cidrs and ports", entry.Hostname)
		}
		for _, cidr := range entry.CIDRs {
			if _, _, err := net.ParseCIDR(strings.TrimSpace(cidr)); err != nil {
				return fmt.Errorf("workforce.providerSSRF.allowedList entry %q has invalid CIDR %q", entry.Hostname, cidr)
			}
		}
		for _, port := range entry.Ports {
			if port < 1 || port > 65535 {
				return fmt.Errorf("workforce.providerSSRF.allowedList entry %q has invalid port %d", entry.Hostname, port)
			}
		}
		if entry.AllowFakeIP && cfg.Platform.Mode != PlatformModeLocal {
			return fmt.Errorf("workforce.providerSSRF.allowedList entry %q may enable allowFakeIP only in platform.mode=local", entry.Hostname)
		}
		if entry.AllowLoopback {
			if cfg.Platform.Mode != PlatformModeLocal {
				return fmt.Errorf("workforce.providerSSRF.allowedList entry %q may enable allowLoopback only in platform.mode=local", entry.Hostname)
			}
			if entry.AllowFakeIP || host != "127.0.0.1" || len(entry.CIDRs) != 1 || strings.TrimSpace(entry.CIDRs[0]) != "127.0.0.1/32" {
				return fmt.Errorf("workforce.providerSSRF.allowedList entry %q allowLoopback requires exact 127.0.0.1/32 without allowFakeIP", entry.Hostname)
			}
		}
	}
	return nil
}

func resolveProfile() string {
	for _, key := range []string{"SEVEN_PROFILE", "SEVEN_ENV"} {
		value := strings.TrimSpace(os.Getenv(key))
		if value != "" {
			return value
		}
	}
	return "dev"
}

func setDefaults(loader *viper.Viper) {
	loader.SetDefault("seven.name", "seven-agent-force-server")
	loader.SetDefault("seven.env", "dev")
	loader.SetDefault("server.host", "0.0.0.0")
	loader.SetDefault("server.port", 8888)
	loader.SetDefault("server.contextPath", "")
	loader.SetDefault("server.readTimeout", "5s")
	loader.SetDefault("server.writeTimeout", "10s")
	loader.SetDefault("server.idleTimeout", "60s")
	loader.SetDefault("logging.level", "info")
	loader.SetDefault("logging.format", "json")
	loader.SetDefault("logging.request.enabled", true)
	loader.SetDefault("logging.request.maxBodyBytes", 4096)
	loader.SetDefault("logging.request.maxFieldLength", 512)
	loader.SetDefault("logging.request.includeQuery", true)
	loader.SetDefault("logging.request.maskedFields", []string{
		"password",
		"pwd",
		"token",
		"secret",
		"authorization",
		"cookie",
		"set-cookie",
		"accessToken",
		"refreshToken",
		"clientSecret",
		"secretCiphertext",
		"passwordHash",
		"apiKey",
		"accessKey",
		"privateKey",
		"configValue",
		"isSensitive",
		"captchaCode",
		"otpCode",
		"totpCode",
		"oneTimePassword",
		"emailCode",
		"recoveryCode",
		"credentialIdentifier",
		"clientDataJSON",
		"authenticatorData",
		"signature",
		"userHandle",
	})
	loader.SetDefault("logging.request.skipContentTypes", []string{
		"multipart/form-data",
		"application/octet-stream",
	})
	loader.SetDefault("logging.file.enabled", true)
	loader.SetDefault("logging.file.path", "logs/seven-agent-force-server.log")
	loader.SetDefault("logging.file.maxSizeMB", 100)
	loader.SetDefault("logging.file.maxBackups", 10)
	loader.SetDefault("logging.file.maxAgeDays", 30)
	loader.SetDefault("logging.file.compress", true)
	loader.SetDefault("logging.file.localTime", true)
	loader.SetDefault("storage.location", "uploads")
	loader.SetDefault("storage.forceCreated", true)
	loader.SetDefault("storage.staticPath", "/static")
	loader.SetDefault("datasource.driver", "mysql")
	loader.SetDefault("datasource.bootstrap.enabled", false)
	loader.SetDefault("datasource.bootstrap.mode", string(BootstrapModeManual))
	loader.SetDefault("datasource.bootstrap.migrationsDir", "")
	loader.SetDefault("datasource.bootstrap.versionTable", "goose_db_version")
	loader.SetDefault("datasource.bootstrap.changeOwner", "goose")
	loader.SetDefault("datasource.bootstrap.baselineVersion", "")
	loader.SetDefault("datasource.bootstrap.allowLegacySync", false)
	loader.SetDefault("datasource.mysql.enabled", false)
	loader.SetDefault("datasource.mysql.dsn", "")
	loader.SetDefault("datasource.mysql.maxOpenConns", 20)
	loader.SetDefault("datasource.mysql.maxIdleConns", 5)
	loader.SetDefault("datasource.mysql.connMaxLifetime", "30m")
	loader.SetDefault("datasource.mysql.connMaxIdleTime", "10m")
	loader.SetDefault("datasource.postgres.enabled", false)
	loader.SetDefault("datasource.postgres.dsn", "")
	loader.SetDefault("datasource.postgres.maxOpenConns", 20)
	loader.SetDefault("datasource.postgres.maxIdleConns", 5)
	loader.SetDefault("datasource.postgres.connMaxLifetime", "30m")
	loader.SetDefault("datasource.postgres.connMaxIdleTime", "10m")
	loader.SetDefault("cache.enabled", false)
	loader.SetDefault("cache.prefix", "seven")
	loader.SetDefault("cache.codec", "sonic")
	loader.SetDefault("cache.l1.enabled", true)
	loader.SetDefault("cache.l1.maxCost", int64(10000000))
	loader.SetDefault("cache.l1.numCounters", int64(100000))
	loader.SetDefault("cache.l1.bufferItems", int64(64))
	loader.SetDefault("cache.l1.defaultTTL", "1m")
	loader.SetDefault("cache.redis.enabled", true)
	loader.SetDefault("cache.redis.mode", string(RedisCacheModeSingle))
	loader.SetDefault("cache.redis.keyPrefix", "seven")
	loader.SetDefault("cache.redis.database", 0)
	loader.SetDefault("cache.redis.username", "")
	loader.SetDefault("cache.redis.password", "")
	loader.SetDefault("cache.redis.clientName", "seven-agent-force-server")
	loader.SetDefault("cache.redis.dialTimeout", "3s")
	loader.SetDefault("cache.redis.readTimeout", "2s")
	loader.SetDefault("cache.redis.writeTimeout", "2s")
	loader.SetDefault("cache.redis.poolSize", 20)
	loader.SetDefault("cache.redis.minIdleConns", 2)
	loader.SetDefault("cache.redis.single.addr", "127.0.0.1:6379")
	loader.SetDefault("cache.redis.sentinel.masterName", "")
	loader.SetDefault("cache.redis.sentinel.addrs", []string{})
	loader.SetDefault("cache.redis.cluster.addrs", []string{})
	loader.SetDefault("rabbitmq.enabled", false)
	loader.SetDefault("rabbitmq.url", "")
	loader.SetDefault("rabbitmq.host", "127.0.0.1")
	loader.SetDefault("rabbitmq.port", 5672)
	loader.SetDefault("rabbitmq.username", "guest")
	loader.SetDefault("rabbitmq.password", "guest")
	loader.SetDefault("rabbitmq.vhost", "/")
	loader.SetDefault("rabbitmq.prefetch", 10)
	loader.SetDefault("rabbitmq.reconnectMin", "1s")
	loader.SetDefault("rabbitmq.reconnectMax", "30s")
	loader.SetDefault("rabbitmq.declare", true)
	loader.SetDefault("workforce.enabled", false)
	loader.SetDefault("workforce.artifact.endpoint", "127.0.0.1:9000")
	loader.SetDefault("workforce.artifact.bucket", "seven-workforce-artifacts")
	loader.SetDefault("workforce.artifact.useTLS", false)
	loader.SetDefault("workforce.worker.id", "local-worker-1")
	loader.SetDefault("workforce.worker.maxActiveContainers", 1)
	loader.SetDefault("workforce.worker.workspaceRoot", "/tmp/seven-workforce/workspaces")
	loader.SetDefault("workforce.worker.pollInterval", "1s")
	loader.SetDefault("workforce.harness.alias", "fake-local-v1")
	loader.SetDefault("workforce.harness.contract", "seven.worker-image/v1")
	loader.SetDefault("workforce.providerSSRF.enabled", true)
	loader.SetDefault("workforce.providerSSRF.allowedList", []WorkforceProviderSSRFAllowEntry{})
	loader.SetDefault("workforce.providerSSRF.maxRequestBytes", int64(128<<20))
	loader.SetDefault("workforce.providerSSRF.maxResponseHeaderBytes", int64(256<<10))
	loader.SetDefault("workforce.providerSSRF.requestTimeout", "10m")
	loader.SetDefault("workforce.providerSSRF.probeTimeout", "2m")
	loader.SetDefault("workforce.providerSSRF.responseHeaderTimeout", "2m")
	loader.SetDefault("workforce.providerSSRF.tlsHandshakeTimeout", "30s")
	loader.SetDefault("workforce.managedHooks.enabled", false)
	loader.SetDefault("workforce.managedHooks.cacheRoot", "/tmp/seven-workforce/hook-cache")
	loader.SetDefault("workforce.reviewer.enabled", false)
	loader.SetDefault("workforce.openIntegration.enabled", false)
	loader.SetDefault("workforce.openIntegration.allowLocalWebhookEndpoint", false)
	loader.SetDefault("workforce.openIntegration.projectorInterval", "1s")
	loader.SetDefault("workforce.openIntegration.deliveryBatchSize", 32)
	loader.SetDefault("workforce.openIntegration.deliveryLeaseSeconds", 60)
	loader.SetDefault("workforce.openIntegration.httpTimeoutSeconds", 10)
	loader.SetDefault("microservice.enabled", false)
	loader.SetDefault("microservice.internalServer.enabled", false)
	loader.SetDefault("microservice.internalServer.listen", "127.0.0.1:9377")
	loader.SetDefault("microservice.service.name", "seven-hub")
	loader.SetDefault("microservice.service.instanceId", "")
	loader.SetDefault("microservice.service.advertisedHost", "")
	loader.SetDefault("microservice.service.advertisedPort", 0)
	loader.SetDefault("microservice.service.scheme", "http")
	loader.SetDefault("microservice.service.tags", []string{"seven", "platform", "v1"})
	loader.SetDefault("microservice.service.metadata", map[string]string{"version": "1.0.0"})
	loader.SetDefault("microservice.registry.enabled", true)
	loader.SetDefault("microservice.registry.type", "consul")
	loader.SetDefault("microservice.registry.address", "http://127.0.0.1:8500")
	loader.SetDefault("microservice.registry.datacenter", "dc1")
	loader.SetDefault("microservice.registry.token", "")
	loader.SetDefault("microservice.registry.registrationRequired", false)
	loader.SetDefault("microservice.registry.registerTimeout", "3s")
	loader.SetDefault("microservice.registry.deregisterTimeout", "3s")
	loader.SetDefault("microservice.health.interval", "10s")
	loader.SetDefault("microservice.health.timeout", "2s")
	loader.SetDefault("microservice.discovery.type", "consul")
	loader.SetDefault("microservice.discovery.cacheTtl", "10s")
	loader.SetDefault("microservice.discovery.emptyResultTtl", "1s")
	loader.SetDefault("microservice.discovery.resolveTimeout", "2s")
	loader.SetDefault("microservice.discovery.datacenter", "dc1")
	loader.SetDefault("microservice.discovery.tags", []string{})
	loader.SetDefault("microservice.discovery.staticFallbackEnabled", true)
	loader.SetDefault("microservice.static.services", map[string][]MicroserviceStaticInstanceConfig{})
	loader.SetDefault("microservice.client.connectTimeout", "1s")
	loader.SetDefault("microservice.client.requestTimeout", "3s")
	loader.SetDefault("microservice.client.maxIdleConns", 100)
	loader.SetDefault("microservice.client.maxIdleConnsPerHost", 20)
	loader.SetDefault("microservice.client.idleConnTimeout", "90s")
	loader.SetDefault("microservice.client.maxRequestBytes", int64(1<<20))
	loader.SetDefault("microservice.client.maxResponseBytes", int64(4<<20))
	loader.SetDefault("microservice.outbound.trustedHosts", []string{})
	loader.SetDefault("microservice.outbound.trustedCidrs", []string{})
	loader.SetDefault("microservice.outbound.registryTrustedHosts", []string{})
	loader.SetDefault("microservice.outbound.registryTrustedCidrs", []string{})
	loader.SetDefault("platform.mode", string(PlatformModeLocal))
	loader.SetDefault("platform.node.managementBearer", "")
	loader.SetDefault("platform.node.code", "")
	loader.SetDefault("platform.node.internalListener.enabled", false)
	loader.SetDefault("platform.node.internalListener.listen", "127.0.0.1:9777")
	loader.SetDefault("docker.enabled", false)
	loader.SetDefault("docker.failFast", false)
	loader.SetDefault("docker.provider", "docker")
	loader.SetDefault("docker.engine.host", "")
	loader.SetDefault("docker.engine.apiVersion", "")
	loader.SetDefault("docker.engine.timeout", "30s")
	loader.SetDefault("docker.engine.apiVersionNegotiation", true)
	loader.SetDefault("docker.compose.binary", "docker")
	loader.SetDefault("docker.compose.tempDir", "")
	loader.SetDefault("docker.compose.timeout", "60s")
	loader.SetDefault("docker.compose.outputMax", 1048576)
	loader.SetDefault("docker.compose.workspaceRoots", []string{"data/docker-compose"})
	loader.SetDefault("docker.compose.defaultFileName", "docker-compose.yaml")
	loader.SetDefault("docker.compose.dirMode", uint32(0750))
	loader.SetDefault("docker.compose.fileMode", uint32(0640))
	loader.SetDefault("docker.compose.maxComposeBytes", 1048576)
	loader.SetDefault("docker.compose.maxDockerfileBytes", 262144)
	loader.SetDefault("docker.compose.maxExtraFilesBytes", 2097152)
	loader.SetDefault("docker.compose.allowedProjectFileSuffixes", []string{".env", ".conf", ".json", ".yaml", ".yml", ".sh", "Dockerfile"})
	loader.SetDefault("docker.registry.timeout", "15s")
	loader.SetDefault("docker.registry.defaultPageSize", int64(20))
	loader.SetDefault("docker.registry.maxPageSize", int64(200))
	loader.SetDefault("docker.registry.maxPages", int64(64))
	loader.SetDefault("docker.operation.maxConcurrent", 4)
	loader.SetDefault("docker.operation.maxQueued", 64)
	loader.SetDefault("docker.operation.defaultTimeout", "10m")
	loader.SetDefault("docker.operation.eventRetentionLimit", 1000)
	loader.SetDefault("docker.operation.sseHeartbeat", "15s")
	loader.SetDefault("docker.operation.pollInterval", "1s")
	loader.SetDefault("docker.registry.insecureFallback", true)
	loader.SetDefault("docker.security.strictContainerCreate", false)
	loader.SetDefault("limiter.enabled", true)
	loader.SetDefault("limiter.keyPrefix", "seven:limit")
	loader.SetDefault("limiter.defaultLimit", int64(60))
	loader.SetDefault("limiter.defaultWindow", "1m")
	loader.SetDefault("limiter.failOpen", true)
	loader.SetDefault("email.enabled", true)
	loader.SetDefault("email.provider", "mock")
	loader.SetDefault("email.defaultFrom", "SevenFramework <noreply@localhost>")
	loader.SetDefault("email.appName", "SevenFramework")
	loader.SetDefault("email.smtp.host", "127.0.0.1")
	loader.SetDefault("email.smtp.port", 1025)
	loader.SetDefault("email.smtp.username", "")
	loader.SetDefault("email.smtp.password", "")
	loader.SetDefault("email.smtp.from", "")
	loader.SetDefault("email.smtp.useTls", false)
	loader.SetDefault("email.smtp.startTls", false)
	loader.SetDefault("email.smtp.timeout", "10s")
	loader.SetDefault("email.smtp.skipVerify", false)
	loader.SetDefault("email.mock.capturePrefix", "email:mock:capture")
	loader.SetDefault("email.mock.ttl", "10m")
	loader.SetDefault("email.templates.challengeOtp.subject", "【{{.AppName}}】-{{.SceneName}}")
	loader.SetDefault("email.templates.challengeOtp.text", "您的验证码是 {{.Code}}，{{.TTLMinutes}} 分钟内有效。")
	loader.SetDefault("email.templates.challengeOtp.html", "<p>您的验证码是 <strong>{{.Code}}</strong>，{{.TTLMinutes}} 分钟内有效。</p>")
	loader.SetDefault("notification.outbound.policies", []map[string]any{})
	loader.SetDefault("security.originPatterns", []string{})
	loader.SetDefault("security.password.algorithm", "bcrypt")
	loader.SetDefault("security.password.bcrypt.cost", 10)
	loader.SetDefault("security.random.tokenLength", 32)
	loader.SetDefault("security.random.nonceLength", 24)
	loader.SetDefault("security.random.codeLength", 6)
	loader.SetDefault("security.keys.provider", "local")
	loader.SetDefault("security.keys.master.active.kid", "master-local")
	loader.SetDefault("security.keys.master.active.source", "")
	loader.SetDefault("security.keys.master.retired", []map[string]any{})
	loader.SetDefault("security.keys.jwt.algorithm", "RS256")
	loader.SetDefault("security.keys.jwt.active.kid", "jwt-active")
	loader.SetDefault("security.keys.jwt.active.privateKeySource", "")
	loader.SetDefault("security.keys.jwt.active.publicKeySource", "")
	loader.SetDefault("security.keys.jwt.next.kid", "")
	loader.SetDefault("security.keys.jwt.next.privateKeySource", "")
	loader.SetDefault("security.keys.jwt.next.publicKeySource", "")
	loader.SetDefault("security.keys.jwt.next.verifyUntil", "0s")
	loader.SetDefault("security.keys.jwt.retired", []map[string]any{})
	loader.SetDefault("challenge.otpIssuerName", "")
	loader.SetDefault("challenge.otpAllowedDriftWindows", 1)
	loader.SetDefault("challenge.recoveryBatchSize", 10)
	loader.SetDefault("challenge.sessionTtlSeconds", 300)
	loader.SetDefault("challenge.proofTokenTtlMinSeconds", 60)
	loader.SetDefault("challenge.proofTokenTtlMaxSeconds", 300)
	loader.SetDefault("challenge.passwordMaxAttempts", 5)
	loader.SetDefault("challenge.passwordCooldownSeconds", 10)
	loader.SetDefault("challenge.imageMaxAttempts", 5)
	loader.SetDefault("challenge.imageCooldownSeconds", 10)
	loader.SetDefault("challenge.otpMaxAttempts", 5)
	loader.SetDefault("challenge.otpCooldownSeconds", 30)
	loader.SetDefault("challenge.recoveryMaxAttempts", 5)
	loader.SetDefault("challenge.recoveryCooldownSeconds", 30)
	loader.SetDefault("challenge.emailMaxAttempts", 5)
	loader.SetDefault("challenge.emailCooldownSeconds", 60)
	loader.SetDefault("challenge.triggerMaxAttempts", 10)
	loader.SetDefault("challenge.throttleMaxFailures", 0)
	loader.SetDefault("challenge.throttleWindowSeconds", 900)
	loader.SetDefault("challenge.throttleLockSeconds", 900)
	loader.SetDefault("challenge.webauthnRpId", "localhost")
	loader.SetDefault("challenge.webauthnRpName", "SevenFramework")
	loader.SetDefault("challenge.webauthnAllowedOrigins", []string{"http://localhost:3000", "http://127.0.0.1:3000", "http://localhost:5177", "http://127.0.0.1:5177"})
	loader.SetDefault("challenge.webauthnChallengeTimeoutSeconds", 300)
	loader.SetDefault("login.enabled", true)
	loader.SetDefault("login.interactionTtlSeconds", 300)
	loader.SetDefault("login.captchaThreshold", 3)
	loader.SetDefault("login.totpThreshold", 5)
	loader.SetDefault("login.lockThreshold", 10)
	loader.SetDefault("login.contextLockThreshold", 10)
	loader.SetDefault("login.lockDurationHours", 24)
	loader.SetDefault("externalLogin.enabled", true)
	loader.SetDefault("externalLogin.enterpriseFakeBaseUrl", "")
	loader.SetDefault("externalLogin.stateTtlSeconds", 300)
	loader.SetDefault("externalLogin.failClosed", true)
	loader.SetDefault("sso.enabled", true)
	loader.SetDefault("sso.issuer", "")
	loader.SetDefault("sso.baseUrl", "")
	loader.SetDefault("sso.defaultFirstPartyClientId", "authorization-console")
	loader.SetDefault("sso.frontendPrimaryEnabled", true)
	loader.SetDefault("sso.frontendLoginUrl", "http://127.0.0.1:5177/login")
	loader.SetDefault("sso.resourceServerEnabled", true)
	loader.SetDefault("sso.loginTransactionTtlSeconds", 300)
	loader.SetDefault("sso.sessionIdleTimeoutSeconds", 1800)
	loader.SetDefault("sso.sessionTouchThrottleSeconds", 60)
	loader.SetDefault("sso.userinfoTouchThrottleSeconds", 300)
	loader.SetDefault("sso.refreshReplayClockSkewSeconds", 30)
	loader.SetDefault("sso.rateLimit.tokenLimit", 60)
	loader.SetDefault("sso.rateLimit.tokenWindow", "1m")
	loader.SetDefault("sso.rateLimit.userinfoLimit", 120)
	loader.SetDefault("sso.rateLimit.userinfoWindow", "1m")
	loader.SetDefault("sso.rateLimit.failClosedOnError", false)
	loader.SetDefault("sso.sessionCookie.name", "SEVEN_SSO_SESSION")
	loader.SetDefault("sso.sessionCookie.path", "/")
	loader.SetDefault("sso.sessionCookie.sameSite", "Lax")
	loader.SetDefault("sso.sessionCookie.secure", false)
	loader.SetDefault("sso.refreshCookie.name", "__Host-seven_sso_rt")
	loader.SetDefault("sso.refreshCookie.path", "/")
	loader.SetDefault("sso.refreshCookie.sameSite", "Lax")
	loader.SetDefault("sso.refreshCookie.secure", false)
	loader.SetDefault("sso.refreshCookie.httpOnly", true)
	loader.SetDefault("sso.jwt.currentKid", "sso-kid-v1")
	loader.SetDefault("sso.jwt.nextKid", "sso-kid-v2")
	loader.SetDefault("sso.jwt.privateKeysByKid", map[string]string{})
	loader.SetDefault("sso.jwt.publicKeysByKid", map[string]string{})
	loader.SetDefault("sso.jwt.keyStatusByKid", map[string]string{
		"sso-kid-v1": "ACTIVE",
		"sso-kid-v2": "NEXT",
	})
	loader.SetDefault("authorization.enabled", true)
	loader.SetDefault("authorization.mode", string(AuthorizationModeLocal))
	loader.SetDefault("authorization.anonymousUrls", []string{
		"/ping",
		"/healthz",
		"/ops/modules",
		"/sso/.well-known/**",
		"/sso/runtime/config",
		"/sso/oauth2/authorize",
		"/sso/oauth2/authorize/login",
		"/sso/oauth2/token",
		"/sso/oauth2/userinfo",
		"/sso/oauth2/revoke",
		"/sso/oauth2/introspect",
		"/login/**",
		"GET /external-identities/oauth/callback/*",
		"/system/features/runtime",
		"/platform/public/login-options",
		"/platform/login-options",
		"/platform/admin/**",
		"/external-login/admin/**",
		"/v1/challenges/**",
		"/dict-client/**",
		"/config-client/**",
		"GET /config-assets/:id",
		"/uploads/callback",
		"/setup/status",
		"/setup/owner",
		"GET /open/workforce/v1/capabilities",
		"POST /open/workforce/v1/projects/:projectId/tasks",
		"GET /open/workforce/v1/projects/:projectId/model-options",
		"GET /open/workforce/v1/projects/:projectId/runtime-options",
		"GET /open/workforce/v1/tasks/**",
	})
	loader.SetDefault("authorization.gateway.signatureEnabled", false)
	loader.SetDefault("authorization.gateway.signatureVersion", "v1")
	loader.SetDefault("authorization.gateway.acceptedVersions", []string{"v1"})
	loader.SetDefault("authorization.gateway.secretsByVersion", map[string]string{})
	loader.SetDefault("authorization.gateway.secret", "")
	loader.SetDefault("authorization.gateway.timestampToleranceSeconds", 300)
	loader.SetDefault("authorization.cache.enabled", true)
	loader.SetDefault("authorization.cache.ttlSeconds", 300)
	loader.SetDefault("authorization.cache.keyPrefix", "seven:auth:userctx")
	loader.SetDefault("authorization.remote.serviceName", "authorization-center")
	loader.SetDefault("authorization.remote.serviceUrl", "")
	loader.SetDefault("authorization.remote.internalBasePath", "/internal/auth")
	loader.SetDefault("authorization.remote.ssoIssuer", "")
	loader.SetDefault("authorization.remote.ssoJwksUri", "")
	loader.SetDefault("authorization.remote.ssoAudience", "authorization-console")
	loader.SetDefault("authorization.remote.timeoutMilliseconds", 3000)
	loader.SetDefault("authorization.remote.failClosed", true)
	loader.SetDefault("authorization.remote.acceptGatewayHeaders", false)
	loader.SetDefault("authorization.internal.enabled", true)
	loader.SetDefault("authorization.internal.headerName", "X-Internal-Token")
	loader.SetDefault("authorization.internal.token", "")
	loader.SetDefault("authorization.internal.signatureEnabled", false)
	loader.SetDefault("authorization.internal.signatureSecret", "")
	loader.SetDefault("authorization.internal.timestampToleranceMs", 300000)
	loader.SetDefault("authorization.internal.nonceTtlSeconds", 300)
	loader.SetDefault("authorization.internal.nonceMinLength", 8)
	loader.SetDefault("authorization.internal.nonceMaxLength", 64)
	loader.SetDefault("authorization.network.trustForwardHeaders", false)
	loader.SetDefault("authorization.network.trustedProxies", []string{})
	loader.SetDefault("authorization.network.trustedCidrs", []string{})
	loader.SetDefault("setup.enabled", true)
	loader.SetDefault("setup.tokenSecret", "")
	loader.SetDefault("setup.tokenTtlSeconds", int64(300))
	loader.SetDefault("setup.ownerBootstrapLockSeconds", int64(30))
	loader.SetDefault("setup.bootstrapClientId", "authorization-console")
	loader.SetDefault("setup.allowedOriginPatterns", []string{"http://127.0.0.1:*", "http://localhost:*"})
	loader.SetDefault("setup.requireOriginHeader", true)
	loader.SetDefault("setup.bootstrap.superAdminRoleCode", "SUPER_ADMIN")
	loader.SetDefault("setup.bootstrap.superAdminRoleName", "超级管理员")
	loader.SetDefault("file.enabled", true)
	loader.SetDefault("file.binding.maxRetries", 3)
	loader.SetDefault("file.binding.retryDelaySeconds", 60)
	loader.SetDefault("file.binding.failedRetentionDays", 7)
	loader.SetDefault("file.binding.retryBatchSize", 50)
	loader.SetDefault("file.binding.routes.DEFAULT_FILE.mode", "local")
	loader.SetDefault("file.binding.routes.USER_AVATAR.mode", "local")
	loader.SetDefault("file.chunkUpload.expireHours", 24)
	loader.SetDefault("file.chunkUpload.maxChunkSize", int64(104857600))
	loader.SetDefault("file.chunkUpload.maxRequestRate", 100)
	loader.SetDefault("file.chunkUpload.bufferSize", 8192)
	loader.SetDefault("file.chunkUpload.tempDirectory", "temp/chunks")
	loader.SetDefault("file.directUpload.presignTtlSeconds", 3600)
	loader.SetDefault("file.directUpload.downloadTtlSeconds", 300)
	loader.SetDefault("file.directUpload.taskExpireHours", 24)
	loader.SetDefault("file.directUpload.multipartThresholdBytes", int64(10485760))
	loader.SetDefault("file.directUpload.partSizeBytes", int64(5242880))
	loader.SetDefault("file.directUpload.stagingPrefix", "staging")
	loader.SetDefault("file.directUpload.cleanPrefix", "clean")
	loader.SetDefault("file.directUpload.taskMaxRetries", 3)
	loader.SetDefault("file.directUpload.taskRetryDelaysMs", []int64{10000, 60000, 300000})
	loader.SetDefault("file.directUpload.taskDlqMaxRetries", 2)
	loader.SetDefault("file.directUpload.taskDlqRetryDelayMs", int64(600000))
	loader.SetDefault("file.distribution.gatewayPath", "/file/download")
	loader.SetDefault("file.distribution.signedUrlSecret", "")
	loader.SetDefault("file.distribution.signedUrlTtlSeconds", 300)
	loader.SetDefault("file.distribution.allowIpBind", true)
	loader.SetDefault("file.distribution.oneTimeToken", false)
	loader.SetDefault("file.distribution.rangeEnabled", true)
	loader.SetDefault("file.distribution.cacheControlPublic", "public,max-age=604800,immutable")
	loader.SetDefault("file.distribution.cacheControlPrivate", "private,no-store,max-age=0")
	loader.SetDefault("file.distribution.hotlinkProtectionEnabled", false)
	loader.SetDefault("file.distribution.hotlinkAllowedDomains", "")
	loader.SetDefault("file.distribution.allowEmptyReferer", true)
	loader.SetDefault("file.outbox.enabled", true)
	loader.SetDefault("file.outbox.legacyDirectMq", false)
	loader.SetDefault("file.outbox.relayIntervalMs", 3000)
	loader.SetDefault("file.outbox.batchSize", 100)
	loader.SetDefault("file.processTask.timeoutSeconds", 300)
	loader.SetDefault("file.processTask.outputDirectory", "temp/processed")
	loader.SetDefault("file.cleanup.deletedFileRetentionDays", 30)
	loader.SetDefault("file.cleanup.unreferencedFileCleanupDays", 7)
	loader.SetDefault("file.healthCheck.minRequestsForFailover", 10)
	loader.SetDefault("file.defaultPost.enableThumbnail", true)
	loader.SetDefault("file.defaultPost.enableCompress", true)
	loader.SetDefault("file.defaultPost.compressThresholdBytes", int64(1048576))
	loader.SetDefault("file.defaultPost.thumbnailSpecs", []string{"thumbnail_small", "thumbnail_medium"})
	loader.SetDefault("file.defaultPost.compressType", "ZIP")
	loader.SetDefault("file.rabbit.enabled", true)
	loader.SetDefault("admin.runtimeLog.enabled", true)
	loader.SetDefault("admin.runtimeLog.baseDir", "")
	loader.SetDefault("admin.runtimeLog.activeFile", "")
	loader.SetDefault("admin.runtimeLog.maxSearchWindowDays", 30)
	loader.SetDefault("admin.runtimeLog.maxPageSize", 500)
	loader.SetDefault("admin.runtimeLog.maxScanLines", 500000)
	loader.SetDefault("admin.runtimeLog.tailPollIntervalMs", 5000)
	loader.SetDefault("admin.runtimeLog.heartbeatIntervalMs", 5000)
	loader.SetDefault("admin.runtimeLog.maxGlobalConnections", 20)
	loader.SetDefault("admin.runtimeLog.maxConnectionsPerUser", 3)
	loader.SetDefault("admin.runtimeLog.defaultLastN", 100)
	loader.SetDefault("admin.runtimeLog.maxLastN", 1000)
	loader.SetDefault("admin.runtimeLog.emitterTimeoutMs", int64(0))
	loader.SetDefault("admin.runtimeLog.defaultHistoryWindow", "24h")
	loader.SetDefault("scheduler.enabled", true)
	loader.SetDefault("scheduler.timezone", "Asia/Shanghai")
	loader.SetDefault("scheduler.lock.enabled", true)
	loader.SetDefault("scheduler.lock.ttl", "1m")
	loader.SetDefault("observability.enabled", true)
	loader.SetDefault("observability.snapshotIntervalMs", int64(300000))
	loader.SetDefault("observability.prometheus.enabled", true)
	loader.SetDefault("observability.prometheus.path", "/ops/prometheus")
	loader.SetDefault("observability.prometheus.accessToken", "")
	loader.SetDefault("observability.tracing.enabled", true)
	loader.SetDefault("observability.tracing.serviceName", "seven-agent-force-server")
	loader.SetDefault("observability.tracing.otlpEndpoint", "")
	loader.SetDefault("observability.tracing.insecure", false)
	loader.SetDefault("observability.tracing.exportTimeout", "3s")
	loader.SetDefault("observability.pprof.enabled", false)
	loader.SetDefault("observability.pprof.prefix", "/ops/debug/pprof")
	loader.SetDefault("observability.logs.enabled", true)
	loader.SetDefault("observability.logs.recentLimit", 20)
	loader.SetDefault("observability.logs.errorLimit", 10)
	loader.SetDefault("observability.logs.hotLoggerLimit", 8)
	loader.SetDefault("observability.logs.trendBucketSeconds", int64(300))
	loader.SetDefault("id.node", int64(1))
}

func mergeFile(loader *viper.Viper, path string) error {
	fileLoader := viper.New()
	fileLoader.SetConfigFile(path)
	if err := fileLoader.ReadInConfig(); err != nil {
		return fmt.Errorf("read config %s: %w", path, err)
	}
	if err := loader.MergeConfigMap(fileLoader.AllSettings()); err != nil {
		return fmt.Errorf("merge config %s: %w", path, err)
	}
	return nil
}

func resolveExistingFile(configDir, baseName string) (string, error) {
	path, err := resolveOptionalFile(configDir, baseName)
	if err != nil {
		return "", err
	}
	if path == "" {
		return "", fmt.Errorf("config file %s(.yaml|.yml) not found in %s", baseName, configDir)
	}
	return path, nil
}

func resolveOptionalFile(configDir, baseName string) (string, error) {
	for _, ext := range []string{".yaml", ".yml"} {
		path := filepath.Join(configDir, baseName+ext)
		_, err := os.Stat(path)
		if err == nil {
			return path, nil
		}
		if !errors.Is(err, os.ErrNotExist) {
			return "", fmt.Errorf("stat config file %s: %w", path, err)
		}
	}
	return "", nil
}

func normalize(cfg *Config) {
	if strings.TrimSpace(cfg.Seven.Env) == "" {
		cfg.Seven.Env = cfg.Profile
	}
	cfg.Logging.Level = strings.ToLower(strings.TrimSpace(cfg.Logging.Level))
	cfg.Logging.Format = strings.ToLower(strings.TrimSpace(cfg.Logging.Format))
	if cfg.Logging.Level == "" {
		cfg.Logging.Level = "info"
	}
	if cfg.Logging.Format == "" {
		cfg.Logging.Format = "json"
	}
	if cfg.Logging.Request.MaxBodyBytes <= 0 {
		cfg.Logging.Request.MaxBodyBytes = 4096
	}
	if cfg.Logging.Request.MaxFieldLength <= 0 {
		cfg.Logging.Request.MaxFieldLength = 512
	}
	if len(cfg.Logging.Request.MaskedFields) == 0 {
		cfg.Logging.Request.MaskedFields = []string{
			"password",
			"pwd",
			"token",
			"secret",
			"authorization",
			"cookie",
			"set-cookie",
			"accessToken",
			"refreshToken",
			"idToken",
			"clientSecret",
			"secretHash",
			"codeVerifier",
			"secretCiphertext",
			"passwordHash",
			"apiKey",
			"accessKey",
			"privateKey",
			"configValue",
			"isSensitive",
			"captchaCode",
			"otpCode",
			"totpCode",
			"oneTimePassword",
			"emailCode",
			"recoveryCode",
			"credentialIdentifier",
			"clientDataJSON",
			"authenticatorData",
			"signature",
			"userHandle",
		}
	}
	if len(cfg.Logging.Request.SkipContentTypes) == 0 {
		cfg.Logging.Request.SkipContentTypes = []string{
			"multipart/form-data",
			"application/octet-stream",
		}
	}
	cfg.Logging.File.Path = strings.TrimSpace(cfg.Logging.File.Path)
	if cfg.Logging.File.Path == "" {
		cfg.Logging.File.Path = "logs/seven-agent-force-server.log"
	}
	if cfg.Logging.File.MaxSizeMB <= 0 {
		cfg.Logging.File.MaxSizeMB = 100
	}
	if cfg.Logging.File.MaxBackups < 0 {
		cfg.Logging.File.MaxBackups = 0
	}
	if cfg.Logging.File.MaxAgeDays < 0 {
		cfg.Logging.File.MaxAgeDays = 0
	}
	cfg.Datasource.Driver = strings.ToLower(strings.TrimSpace(cfg.Datasource.Driver))
	if cfg.Datasource.Driver == "" {
		cfg.Datasource.Driver = "mysql"
	}
	cfg.Datasource.Bootstrap.Mode = DatasourceBootstrapMode(strings.ToLower(strings.TrimSpace(string(cfg.Datasource.Bootstrap.Mode))))
	switch cfg.Datasource.Bootstrap.Mode {
	case BootstrapModeManual, BootstrapModeStartup, BootstrapModeBoth:
	default:
		cfg.Datasource.Bootstrap.Mode = BootstrapModeManual
	}
	cfg.Datasource.Bootstrap.MigrationsDir = strings.TrimSpace(cfg.Datasource.Bootstrap.MigrationsDir)
	cfg.Datasource.Bootstrap.VersionTable = strings.TrimSpace(cfg.Datasource.Bootstrap.VersionTable)
	if cfg.Datasource.Bootstrap.VersionTable == "" {
		cfg.Datasource.Bootstrap.VersionTable = "goose_db_version"
	}
	cfg.Datasource.Bootstrap.ChangeOwner = strings.ToLower(strings.TrimSpace(cfg.Datasource.Bootstrap.ChangeOwner))
	if cfg.Datasource.Bootstrap.ChangeOwner == "" {
		cfg.Datasource.Bootstrap.ChangeOwner = "goose"
	}
	cfg.Datasource.Bootstrap.BaselineVersion = strings.TrimSpace(cfg.Datasource.Bootstrap.BaselineVersion)
	cfg.Datasource.MySQL.DSN = strings.TrimSpace(cfg.Datasource.MySQL.DSN)
	if cfg.Datasource.MySQL.MaxOpenConns <= 0 {
		cfg.Datasource.MySQL.MaxOpenConns = 20
	}
	if cfg.Datasource.MySQL.MaxIdleConns < 0 {
		cfg.Datasource.MySQL.MaxIdleConns = 0
	}
	if cfg.Datasource.MySQL.ConnMaxLifetime <= 0 {
		cfg.Datasource.MySQL.ConnMaxLifetime = 30 * time.Minute
	}
	if cfg.Datasource.MySQL.ConnMaxIdleTime < 0 {
		cfg.Datasource.MySQL.ConnMaxIdleTime = 0
	}
	cfg.Datasource.Postgres.DSN = strings.TrimSpace(cfg.Datasource.Postgres.DSN)
	if cfg.Datasource.Postgres.MaxOpenConns <= 0 {
		cfg.Datasource.Postgres.MaxOpenConns = 20
	}
	if cfg.Datasource.Postgres.MaxIdleConns < 0 {
		cfg.Datasource.Postgres.MaxIdleConns = 0
	}
	if cfg.Datasource.Postgres.ConnMaxLifetime <= 0 {
		cfg.Datasource.Postgres.ConnMaxLifetime = 30 * time.Minute
	}
	if cfg.Datasource.Postgres.ConnMaxIdleTime < 0 {
		cfg.Datasource.Postgres.ConnMaxIdleTime = 0
	}
	cfg.Cache.Prefix = strings.TrimSpace(cfg.Cache.Prefix)
	if cfg.Cache.Prefix == "" {
		cfg.Cache.Prefix = "seven"
	}
	cfg.Cache.Codec = strings.ToLower(strings.TrimSpace(cfg.Cache.Codec))
	if cfg.Cache.Codec == "" {
		cfg.Cache.Codec = "sonic"
	}
	if cfg.Cache.L1.MaxCost <= 0 {
		cfg.Cache.L1.MaxCost = 10000000
	}
	if cfg.Cache.L1.NumCounters <= 0 {
		cfg.Cache.L1.NumCounters = 100000
	}
	if cfg.Cache.L1.BufferItems <= 0 {
		cfg.Cache.L1.BufferItems = 64
	}
	if cfg.Cache.L1.DefaultTTL < 0 {
		cfg.Cache.L1.DefaultTTL = 0
	}
	cfg.Cache.Governance.InstanceID = strings.TrimSpace(cfg.Cache.Governance.InstanceID)
	if cfg.Cache.Governance.RelayInterval <= 0 {
		cfg.Cache.Governance.RelayInterval = time.Second
	}
	if cfg.Cache.Governance.RelayBatch <= 0 {
		cfg.Cache.Governance.RelayBatch = 50
	}
	cfg.Cache.Redis.Mode = RedisCacheMode(strings.ToLower(strings.TrimSpace(string(cfg.Cache.Redis.Mode))))
	switch cfg.Cache.Redis.Mode {
	case RedisCacheModeSingle, RedisCacheModeSentinel, RedisCacheModeCluster:
	default:
		cfg.Cache.Redis.Mode = RedisCacheModeSingle
	}
	cfg.Cache.Redis.KeyPrefix = strings.TrimSpace(cfg.Cache.Redis.KeyPrefix)
	if cfg.Cache.Redis.KeyPrefix == "" {
		cfg.Cache.Redis.KeyPrefix = cfg.Cache.Prefix
	}
	cfg.Cache.Redis.Username = strings.TrimSpace(cfg.Cache.Redis.Username)
	cfg.Cache.Redis.Password = strings.TrimSpace(cfg.Cache.Redis.Password)
	cfg.Cache.Redis.ClientName = strings.TrimSpace(cfg.Cache.Redis.ClientName)
	if cfg.Cache.Redis.ClientName == "" {
		cfg.Cache.Redis.ClientName = "seven-agent-force-server"
	}
	if cfg.Cache.Redis.DialTimeout <= 0 {
		cfg.Cache.Redis.DialTimeout = 3 * time.Second
	}
	if cfg.Cache.Redis.ReadTimeout <= 0 {
		cfg.Cache.Redis.ReadTimeout = 2 * time.Second
	}
	if cfg.Cache.Redis.WriteTimeout <= 0 {
		cfg.Cache.Redis.WriteTimeout = 2 * time.Second
	}
	if cfg.Cache.Redis.PoolSize <= 0 {
		cfg.Cache.Redis.PoolSize = 20
	}
	if cfg.Cache.Redis.MinIdleConns < 0 {
		cfg.Cache.Redis.MinIdleConns = 0
	}
	cfg.Cache.Redis.Single.Addr = strings.TrimSpace(cfg.Cache.Redis.Single.Addr)
	if cfg.Cache.Redis.Single.Addr == "" {
		cfg.Cache.Redis.Single.Addr = "127.0.0.1:6379"
	}
	cfg.Cache.Redis.Sentinel.MasterName = strings.TrimSpace(cfg.Cache.Redis.Sentinel.MasterName)
	cfg.Cache.Redis.Sentinel.Addrs = trimValues(cfg.Cache.Redis.Sentinel.Addrs)
	cfg.Cache.Redis.Cluster.Addrs = trimValues(cfg.Cache.Redis.Cluster.Addrs)
	cfg.Security.OriginPatterns = trimValues(cfg.Security.OriginPatterns)
	cfg.Security.Password.Algorithm = strings.ToLower(strings.TrimSpace(cfg.Security.Password.Algorithm))
	if cfg.Security.Password.Algorithm == "" {
		cfg.Security.Password.Algorithm = "bcrypt"
	}
	if cfg.Security.Password.Bcrypt.Cost <= 0 {
		cfg.Security.Password.Bcrypt.Cost = 10
	}
	if cfg.Security.Random.TokenLength <= 0 {
		cfg.Security.Random.TokenLength = 32
	}
	if cfg.Security.Random.NonceLength <= 0 {
		cfg.Security.Random.NonceLength = 24
	}
	if cfg.Security.Random.CodeLength <= 0 {
		cfg.Security.Random.CodeLength = 6
	}
	cfg.Security.Keys.Provider = strings.ToLower(strings.TrimSpace(cfg.Security.Keys.Provider))
	if cfg.Security.Keys.Provider == "" {
		cfg.Security.Keys.Provider = "local"
	}
	cfg.Security.Keys.Master.Active.KID = strings.TrimSpace(cfg.Security.Keys.Master.Active.KID)
	cfg.Security.Keys.Master.Active.Source = strings.TrimSpace(cfg.Security.Keys.Master.Active.Source)
	for index := range cfg.Security.Keys.Master.Retired {
		cfg.Security.Keys.Master.Retired[index].KID = strings.TrimSpace(cfg.Security.Keys.Master.Retired[index].KID)
		cfg.Security.Keys.Master.Retired[index].Source = strings.TrimSpace(cfg.Security.Keys.Master.Retired[index].Source)
	}
	cfg.Security.Keys.JWT.Algorithm = strings.ToUpper(strings.TrimSpace(cfg.Security.Keys.JWT.Algorithm))
	if cfg.Security.Keys.JWT.Algorithm == "" {
		cfg.Security.Keys.JWT.Algorithm = "RS256"
	}
	cfg.Security.Keys.JWT.Active.KID = strings.TrimSpace(cfg.Security.Keys.JWT.Active.KID)
	cfg.Security.Keys.JWT.Active.PrivateKeySource = strings.TrimSpace(cfg.Security.Keys.JWT.Active.PrivateKeySource)
	cfg.Security.Keys.JWT.Active.PublicKeySource = strings.TrimSpace(cfg.Security.Keys.JWT.Active.PublicKeySource)
	cfg.Security.Keys.JWT.Next.KID = strings.TrimSpace(cfg.Security.Keys.JWT.Next.KID)
	cfg.Security.Keys.JWT.Next.PrivateKeySource = strings.TrimSpace(cfg.Security.Keys.JWT.Next.PrivateKeySource)
	cfg.Security.Keys.JWT.Next.PublicKeySource = strings.TrimSpace(cfg.Security.Keys.JWT.Next.PublicKeySource)
	for index := range cfg.Security.Keys.JWT.Retired {
		cfg.Security.Keys.JWT.Retired[index].KID = strings.TrimSpace(cfg.Security.Keys.JWT.Retired[index].KID)
		cfg.Security.Keys.JWT.Retired[index].PrivateKeySource = strings.TrimSpace(cfg.Security.Keys.JWT.Retired[index].PrivateKeySource)
		cfg.Security.Keys.JWT.Retired[index].PublicKeySource = strings.TrimSpace(cfg.Security.Keys.JWT.Retired[index].PublicKeySource)
	}
	if cfg.Login.InteractionTTLSeconds <= 0 {
		cfg.Login.InteractionTTLSeconds = 300
	}
	if cfg.Login.CaptchaThreshold <= 0 {
		cfg.Login.CaptchaThreshold = 3
	}
	if cfg.Login.TOTPThreshold <= 0 {
		cfg.Login.TOTPThreshold = 5
	}
	if cfg.Login.LockThreshold <= 0 {
		cfg.Login.LockThreshold = 10
	}
	if cfg.Login.ContextLockThreshold <= 0 {
		cfg.Login.ContextLockThreshold = cfg.Login.LockThreshold
	}
	if cfg.Login.LockDurationHours <= 0 {
		cfg.Login.LockDurationHours = 24
	}
	if strings.TrimSpace(cfg.Server.ContextPath) == "" {
		cfg.Server.ContextPath = cfg.Server.ContextPathKebab
	}
	cfg.Server.ContextPath = normalizeContextPath(cfg.Server.ContextPath)
	cfg.Server.ContextPathKebab = ""
	cfg.SSO.Issuer = strings.TrimSpace(cfg.SSO.Issuer)
	cfg.SSO.BaseURL = strings.TrimRight(strings.TrimSpace(cfg.SSO.BaseURL), "/")
	if cfg.SSO.BaseURL == "" {
		host := strings.TrimSpace(cfg.Server.Host)
		if host == "" || host == "0.0.0.0" || host == "::" {
			host = "127.0.0.1"
		}
		port := cfg.Server.Port
		if port <= 0 {
			port = 8888
		}
		cfg.SSO.BaseURL = fmt.Sprintf("http://%s:%d%s/sso", host, port, cfg.Server.ContextPath)
	}
	if cfg.SSO.Issuer == "" {
		cfg.SSO.Issuer = cfg.SSO.BaseURL
	}
	cfg.SSO.DefaultFirstPartyClientID = strings.TrimSpace(cfg.SSO.DefaultFirstPartyClientID)
	if cfg.SSO.DefaultFirstPartyClientID == "" {
		cfg.SSO.DefaultFirstPartyClientID = "authorization-console"
	}
	cfg.Authorization.Mode = AuthorizationMode(strings.ToLower(strings.TrimSpace(string(cfg.Authorization.Mode))))
	switch cfg.Authorization.Mode {
	case AuthorizationModeLocal, AuthorizationModeRemote:
	default:
		cfg.Authorization.Mode = AuthorizationModeLocal
	}
	cfg.Authorization.Gateway.SignatureVersion = strings.TrimSpace(cfg.Authorization.Gateway.SignatureVersion)
	if cfg.Authorization.Gateway.SignatureVersion == "" {
		cfg.Authorization.Gateway.SignatureVersion = "v1"
	}
	if len(cfg.Authorization.Gateway.AcceptedSignatureVersions) == 0 {
		cfg.Authorization.Gateway.AcceptedSignatureVersions = []string{cfg.Authorization.Gateway.SignatureVersion}
	}
	if cfg.Authorization.Gateway.TimestampToleranceSeconds <= 0 {
		cfg.Authorization.Gateway.TimestampToleranceSeconds = 300
	}
	if cfg.Authorization.Cache.TTLSeconds <= 0 {
		cfg.Authorization.Cache.TTLSeconds = 300
	}
	cfg.Authorization.Cache.KeyPrefix = strings.TrimSpace(cfg.Authorization.Cache.KeyPrefix)
	if cfg.Authorization.Cache.KeyPrefix == "" {
		cfg.Authorization.Cache.KeyPrefix = "seven:auth:userctx"
	}
	cfg.Authorization.Remote.ServiceName = strings.TrimSpace(cfg.Authorization.Remote.ServiceName)
	if cfg.Authorization.Remote.ServiceName == "" {
		cfg.Authorization.Remote.ServiceName = "authorization-center"
	}
	cfg.Authorization.Remote.ServiceURL = strings.TrimSpace(cfg.Authorization.Remote.ServiceURL)
	cfg.Authorization.Remote.InternalBasePath = strings.TrimSpace(cfg.Authorization.Remote.InternalBasePath)
	if cfg.Authorization.Remote.InternalBasePath == "" {
		cfg.Authorization.Remote.InternalBasePath = "/internal/auth"
	}
	cfg.Authorization.Remote.SSOIssuer = strings.TrimSpace(cfg.Authorization.Remote.SSOIssuer)
	cfg.Authorization.Remote.SSOJWKSURI = strings.TrimSpace(cfg.Authorization.Remote.SSOJWKSURI)
	cfg.Authorization.Remote.SSOAudience = strings.TrimSpace(cfg.Authorization.Remote.SSOAudience)
	if cfg.Authorization.Remote.SSOAudience == "" {
		cfg.Authorization.Remote.SSOAudience = cfg.SSO.DefaultFirstPartyClientID
	}
	if cfg.Authorization.Remote.TimeoutMilliseconds <= 0 {
		cfg.Authorization.Remote.TimeoutMilliseconds = 3000
	}
	cfg.Authorization.Internal.HeaderName = strings.TrimSpace(cfg.Authorization.Internal.HeaderName)
	if cfg.Authorization.Internal.HeaderName == "" {
		cfg.Authorization.Internal.HeaderName = "X-Internal-Token"
	}
	cfg.Authorization.Internal.Token = strings.TrimSpace(cfg.Authorization.Internal.Token)
	cfg.Authorization.Internal.SignatureSecret = strings.TrimSpace(cfg.Authorization.Internal.SignatureSecret)
	if cfg.Authorization.Internal.TimestampToleranceMs <= 0 {
		cfg.Authorization.Internal.TimestampToleranceMs = 300000
	}
	if cfg.Authorization.Internal.NonceTTLSeconds <= 0 {
		cfg.Authorization.Internal.NonceTTLSeconds = 300
	}
	if cfg.Authorization.Internal.NonceMinLength <= 0 {
		cfg.Authorization.Internal.NonceMinLength = 8
	}
	if cfg.Authorization.Internal.NonceMaxLength < cfg.Authorization.Internal.NonceMinLength {
		cfg.Authorization.Internal.NonceMaxLength = cfg.Authorization.Internal.NonceMinLength
	}
	if len(cfg.Authorization.AnonymousURLs) == 0 {
		cfg.Authorization.AnonymousURLs = []string{
			"/ping",
			"/healthz",
			"/ops/modules",
			"/sso/.well-known/**",
			"/sso/runtime/config",
			"/sso/oauth2/authorize",
			"/sso/oauth2/authorize/login",
			"/sso/oauth2/token",
			"/sso/oauth2/userinfo",
			"/sso/oauth2/revoke",
			"/sso/oauth2/introspect",
			"/login/**",
			"GET /external-identities/oauth/callback/*",
			"/system/features/runtime",
			"/platform/public/login-options",
			"/platform/login-options",
			"/platform/admin/**",
			"/external-login/admin/**",
			"/v1/challenges/**",
			"/dict-client/**",
			"/config-client/**",
			"GET /config-assets/:id",
			"/uploads/callback",
			"/setup/status",
			"/setup/owner",
			"GET /open/workforce/v1/capabilities",
			"POST /open/workforce/v1/projects/:projectId/tasks",
			"GET /open/workforce/v1/projects/:projectId/model-options",
			"GET /open/workforce/v1/projects/:projectId/runtime-options",
			"GET /open/workforce/v1/tasks/**",
		}
	}
	cfg.Setup.TokenSecret = strings.TrimSpace(cfg.Setup.TokenSecret)
	if cfg.Setup.TokenSecret != "" && len(cfg.Setup.TokenSecret) < 32 {
		panic("setup.tokenSecret length must not be less than 32 characters")
	}
	if cfg.Setup.TokenTTLSeconds <= 0 {
		cfg.Setup.TokenTTLSeconds = 300
	}
	if cfg.Setup.OwnerBootstrapLockSeconds <= 0 {
		cfg.Setup.OwnerBootstrapLockSeconds = 30
	}
	cfg.Setup.BootstrapClientID = strings.TrimSpace(cfg.Setup.BootstrapClientID)
	if cfg.Setup.BootstrapClientID == "" {
		cfg.Setup.BootstrapClientID = "authorization-console"
	}
	cfg.Setup.AllowedOriginPatterns = trimValues(cfg.Setup.AllowedOriginPatterns)
	if len(cfg.Setup.AllowedOriginPatterns) == 0 {
		cfg.Setup.AllowedOriginPatterns = []string{"http://127.0.0.1:*", "http://localhost:*"}
	}
	cfg.Setup.Bootstrap.SuperAdminRoleCode = strings.TrimSpace(cfg.Setup.Bootstrap.SuperAdminRoleCode)
	if cfg.Setup.Bootstrap.SuperAdminRoleCode == "" {
		cfg.Setup.Bootstrap.SuperAdminRoleCode = "SUPER_ADMIN"
	}
	if !regexp.MustCompile(`^[A-Z][A-Z0-9_]{2,49}$`).MatchString(cfg.Setup.Bootstrap.SuperAdminRoleCode) {
		panic("setup.bootstrap.superAdminRoleCode must match ^[A-Z][A-Z0-9_]{2,49}$")
	}
	cfg.Setup.Bootstrap.SuperAdminRoleName = strings.TrimSpace(cfg.Setup.Bootstrap.SuperAdminRoleName)
	if cfg.Setup.Bootstrap.SuperAdminRoleName == "" {
		cfg.Setup.Bootstrap.SuperAdminRoleName = "超级管理员"
	}
	cfg.Admin.RuntimeLog.BaseDir = strings.TrimSpace(cfg.Admin.RuntimeLog.BaseDir)
	cfg.Admin.RuntimeLog.ActiveFile = strings.TrimSpace(cfg.Admin.RuntimeLog.ActiveFile)
	if cfg.Admin.RuntimeLog.MaxSearchWindowDays <= 0 {
		cfg.Admin.RuntimeLog.MaxSearchWindowDays = 30
	}
	if cfg.Admin.RuntimeLog.MaxPageSize <= 0 {
		cfg.Admin.RuntimeLog.MaxPageSize = 500
	}
	if cfg.Admin.RuntimeLog.MaxScanLines <= 0 {
		cfg.Admin.RuntimeLog.MaxScanLines = 500000
	}
	if cfg.Admin.RuntimeLog.TailPollIntervalMs <= 0 {
		cfg.Admin.RuntimeLog.TailPollIntervalMs = 5000
	}
	if cfg.Admin.RuntimeLog.HeartbeatIntervalMs <= 0 {
		cfg.Admin.RuntimeLog.HeartbeatIntervalMs = 5000
	}
	if cfg.Admin.RuntimeLog.MaxGlobalConnections <= 0 {
		cfg.Admin.RuntimeLog.MaxGlobalConnections = 20
	}
	if cfg.Admin.RuntimeLog.MaxConnectionsPerUser <= 0 {
		cfg.Admin.RuntimeLog.MaxConnectionsPerUser = 3
	}
	if cfg.Admin.RuntimeLog.DefaultLastN <= 0 {
		cfg.Admin.RuntimeLog.DefaultLastN = 100
	}
	if cfg.Admin.RuntimeLog.MaxLastN < cfg.Admin.RuntimeLog.DefaultLastN {
		cfg.Admin.RuntimeLog.MaxLastN = max(cfg.Admin.RuntimeLog.DefaultLastN, 1000)
	}
	if cfg.Admin.RuntimeLog.DefaultHistoryWindow <= 0 {
		cfg.Admin.RuntimeLog.DefaultHistoryWindow = 24 * time.Hour
	}
	if cfg.Admin.RuntimeLog.BaseDir == "" && cfg.Logging.File.Path != "" {
		cfg.Admin.RuntimeLog.BaseDir = filepath.Dir(cfg.Logging.File.Path)
	}
	if cfg.Admin.RuntimeLog.BaseDir == "" {
		cfg.Admin.RuntimeLog.BaseDir = "logs"
	}
	if cfg.Admin.RuntimeLog.ActiveFile == "" && cfg.Logging.File.Path != "" {
		cfg.Admin.RuntimeLog.ActiveFile = filepath.Base(cfg.Logging.File.Path)
	}
	if cfg.Admin.RuntimeLog.ActiveFile == "" {
		cfg.Admin.RuntimeLog.ActiveFile = "seven-agent-force-server.log"
	}
	cfg.Storage.Location = strings.TrimSpace(cfg.Storage.Location)
	if cfg.Storage.Location == "" {
		cfg.Storage.Location = "uploads"
	}
	cfg.Storage.StaticPath = strings.TrimSpace(cfg.Storage.StaticPath)
	if cfg.Storage.StaticPath == "" {
		cfg.Storage.StaticPath = "/static"
	}
	if !strings.HasPrefix(cfg.Storage.StaticPath, "/") {
		cfg.Storage.StaticPath = "/" + cfg.Storage.StaticPath
	}
	if cfg.File.Binding.MaxRetries <= 0 {
		cfg.File.Binding.MaxRetries = 3
	}
	if cfg.File.Binding.RetryDelaySeconds <= 0 {
		cfg.File.Binding.RetryDelaySeconds = 60
	}
	if cfg.File.Binding.FailedRetentionDays <= 0 {
		cfg.File.Binding.FailedRetentionDays = 7
	}
	if cfg.File.Binding.RetryBatchSize <= 0 {
		cfg.File.Binding.RetryBatchSize = 50
	}
	if cfg.File.Binding.Routes == nil {
		cfg.File.Binding.Routes = map[string]FileBindingRoute{}
	}
	for key, route := range cfg.File.Binding.Routes {
		route.Mode = strings.ToLower(strings.TrimSpace(route.Mode))
		if route.Mode == "" {
			route.Mode = "hybrid"
		}
		route.BaseURL = strings.TrimSpace(route.BaseURL)
		route.EndpointPrefix = strings.TrimSpace(route.EndpointPrefix)
		if route.EndpointPrefix == "" {
			route.EndpointPrefix = "/internal/file-bindings"
		}
		cfg.File.Binding.Routes[key] = route
	}
	if cfg.File.ChunkUpload.ExpireHours <= 0 {
		cfg.File.ChunkUpload.ExpireHours = 24
	}
	if cfg.File.ChunkUpload.MaxChunkSize <= 0 {
		cfg.File.ChunkUpload.MaxChunkSize = 100 * 1024 * 1024
	}
	if cfg.File.ChunkUpload.BufferSize <= 0 {
		cfg.File.ChunkUpload.BufferSize = 8192
	}
	cfg.File.ChunkUpload.TempDirectory = strings.TrimSpace(cfg.File.ChunkUpload.TempDirectory)
	if cfg.File.ChunkUpload.TempDirectory == "" {
		cfg.File.ChunkUpload.TempDirectory = "temp/chunks"
	}
	if cfg.File.DirectUpload.PresignTTLSeconds <= 0 {
		cfg.File.DirectUpload.PresignTTLSeconds = 3600
	}
	if cfg.File.DirectUpload.DownloadTTLSeconds <= 0 {
		cfg.File.DirectUpload.DownloadTTLSeconds = 300
	}
	if cfg.File.DirectUpload.TaskExpireHours <= 0 {
		cfg.File.DirectUpload.TaskExpireHours = 24
	}
	if cfg.File.DirectUpload.MultipartThreshold <= 0 {
		cfg.File.DirectUpload.MultipartThreshold = 10 * 1024 * 1024
	}
	if cfg.File.DirectUpload.PartSizeBytes <= 0 {
		cfg.File.DirectUpload.PartSizeBytes = 5 * 1024 * 1024
	}
	if len(cfg.File.DirectUpload.TaskRetryDelaysMS) == 0 {
		cfg.File.DirectUpload.TaskRetryDelaysMS = []int64{10000, 60000, 300000}
	}
	if cfg.File.DirectUpload.TaskMaxRetries <= 0 {
		cfg.File.DirectUpload.TaskMaxRetries = 3
	}
	if cfg.File.DirectUpload.TaskDLQMaxRetries <= 0 {
		cfg.File.DirectUpload.TaskDLQMaxRetries = 2
	}
	if cfg.File.DirectUpload.TaskDLQRetryDelayMS <= 0 {
		cfg.File.DirectUpload.TaskDLQRetryDelayMS = 600000
	}
	cfg.File.DirectUpload.StagingPrefix = strings.Trim(strings.TrimSpace(cfg.File.DirectUpload.StagingPrefix), "/")
	if cfg.File.DirectUpload.StagingPrefix == "" {
		cfg.File.DirectUpload.StagingPrefix = "staging"
	}
	cfg.File.DirectUpload.CleanPrefix = strings.Trim(strings.TrimSpace(cfg.File.DirectUpload.CleanPrefix), "/")
	if cfg.File.DirectUpload.CleanPrefix == "" {
		cfg.File.DirectUpload.CleanPrefix = "clean"
	}
	if cfg.File.Distribution.SignedURLTTLSeconds <= 0 {
		cfg.File.Distribution.SignedURLTTLSeconds = 300
	}
	cfg.File.Distribution.GatewayPath = strings.TrimSpace(cfg.File.Distribution.GatewayPath)
	if cfg.File.Distribution.GatewayPath == "" {
		cfg.File.Distribution.GatewayPath = "/file/download"
	}
	if !strings.HasPrefix(cfg.File.Distribution.GatewayPath, "/") {
		cfg.File.Distribution.GatewayPath = "/" + cfg.File.Distribution.GatewayPath
	}
	if cfg.File.Distribution.CacheControlPublic == "" {
		cfg.File.Distribution.CacheControlPublic = "public,max-age=604800,immutable"
	}
	if cfg.File.Distribution.CacheControlPrivate == "" {
		cfg.File.Distribution.CacheControlPrivate = "private,no-store,max-age=0"
	}
	if cfg.File.Outbox.RelayIntervalMS <= 0 {
		cfg.File.Outbox.RelayIntervalMS = 3000
	}
	if cfg.File.Outbox.BatchSize <= 0 {
		cfg.File.Outbox.BatchSize = 100
	}
	if cfg.File.ProcessTask.TimeoutSeconds <= 0 {
		cfg.File.ProcessTask.TimeoutSeconds = 300
	}
	if cfg.File.ProcessTask.OutputDirectory == "" {
		cfg.File.ProcessTask.OutputDirectory = "temp/processed"
	}
	if cfg.File.ProcessTask.RetryIntervalMS <= 0 {
		cfg.File.ProcessTask.RetryIntervalMS = 300000
	}
	if cfg.File.ProcessTask.RetryBatchSize <= 0 {
		cfg.File.ProcessTask.RetryBatchSize = 100
	}
	if cfg.File.Cleanup.ChunkCleanupIntervalMS <= 0 {
		cfg.File.Cleanup.ChunkCleanupIntervalMS = 600000
	}
	if cfg.File.Cleanup.BatchSize <= 0 {
		cfg.File.Cleanup.BatchSize = 100
	}
	if cfg.File.HealthCheck.IntervalMS <= 0 {
		cfg.File.HealthCheck.IntervalMS = 300000
	}
	if cfg.RabbitMQ.Port <= 0 {
		cfg.RabbitMQ.Port = 5672
	}
	if cfg.RabbitMQ.Username == "" {
		cfg.RabbitMQ.Username = "guest"
	}
	if cfg.RabbitMQ.Password == "" {
		cfg.RabbitMQ.Password = "guest"
	}
	if cfg.RabbitMQ.VHost == "" {
		cfg.RabbitMQ.VHost = "/"
	}
	if cfg.RabbitMQ.Prefetch <= 0 {
		cfg.RabbitMQ.Prefetch = 10
	}
	if cfg.RabbitMQ.ReconnectMin <= 0 {
		cfg.RabbitMQ.ReconnectMin = time.Second
	}
	if cfg.RabbitMQ.ReconnectMax <= 0 {
		cfg.RabbitMQ.ReconnectMax = 30 * time.Second
	}
	cfg.Microservice.InternalServer.Listen = strings.TrimSpace(cfg.Microservice.InternalServer.Listen)
	cfg.Microservice.Service.Name = strings.TrimSpace(cfg.Microservice.Service.Name)
	cfg.Microservice.Service.InstanceID = strings.TrimSpace(cfg.Microservice.Service.InstanceID)
	cfg.Microservice.Service.AdvertisedHost = strings.TrimSpace(cfg.Microservice.Service.AdvertisedHost)
	cfg.Microservice.Service.Scheme = strings.ToLower(strings.TrimSpace(cfg.Microservice.Service.Scheme))
	cfg.Microservice.Service.Tags = trimValues(cfg.Microservice.Service.Tags)
	if cfg.Microservice.Service.Metadata == nil {
		cfg.Microservice.Service.Metadata = map[string]string{}
	}
	cfg.Microservice.Registry.Type = strings.ToLower(strings.TrimSpace(cfg.Microservice.Registry.Type))
	cfg.Microservice.Registry.Address = strings.TrimRight(strings.TrimSpace(cfg.Microservice.Registry.Address), "/")
	cfg.Microservice.Registry.Datacenter = strings.TrimSpace(cfg.Microservice.Registry.Datacenter)
	cfg.Microservice.Registry.Token = strings.TrimSpace(cfg.Microservice.Registry.Token)
	cfg.Microservice.Discovery.Type = strings.ToLower(strings.TrimSpace(cfg.Microservice.Discovery.Type))
	cfg.Microservice.Discovery.Datacenter = strings.TrimSpace(cfg.Microservice.Discovery.Datacenter)
	cfg.Microservice.Discovery.Tags = trimValues(cfg.Microservice.Discovery.Tags)
	if cfg.Microservice.Static.Services == nil {
		cfg.Microservice.Static.Services = map[string][]MicroserviceStaticInstanceConfig{}
	}
	for serviceName, instances := range cfg.Microservice.Static.Services {
		trimmedName := strings.TrimSpace(serviceName)
		for index := range instances {
			instances[index].URL = strings.TrimRight(strings.TrimSpace(instances[index].URL), "/")
		}
		if trimmedName != serviceName {
			delete(cfg.Microservice.Static.Services, serviceName)
		}
		cfg.Microservice.Static.Services[trimmedName] = instances
	}
	if cfg.Microservice.Client.MaxRequestBytes == 0 {
		cfg.Microservice.Client.MaxRequestBytes = 1 << 20
	}
	if cfg.Microservice.Client.MaxResponseBytes == 0 {
		cfg.Microservice.Client.MaxResponseBytes = 4 << 20
	}
	cfg.Microservice.Outbound.TrustedHosts = lowerTrimValues(cfg.Microservice.Outbound.TrustedHosts)
	cfg.Microservice.Outbound.TrustedCIDRs = trimValues(cfg.Microservice.Outbound.TrustedCIDRs)
	cfg.Microservice.Outbound.RegistryTrustedHosts = lowerTrimValues(cfg.Microservice.Outbound.RegistryTrustedHosts)
	cfg.Microservice.Outbound.RegistryTrustedCIDRs = trimValues(cfg.Microservice.Outbound.RegistryTrustedCIDRs)
	cfg.Platform.Node.Code = strings.TrimSpace(cfg.Platform.Node.Code)
	cfg.Platform.Node.ManagementBearer = strings.TrimSpace(cfg.Platform.Node.ManagementBearer)
	cfg.Platform.Node.InternalListener.Listen = strings.TrimSpace(cfg.Platform.Node.InternalListener.Listen)
	cfg.Docker.Provider = strings.ToLower(strings.TrimSpace(cfg.Docker.Provider))
	if cfg.Docker.Provider == "" {
		cfg.Docker.Provider = "docker"
	}
	cfg.Docker.Engine.Host = strings.TrimSpace(cfg.Docker.Engine.Host)
	cfg.Docker.Engine.APIVersion = strings.TrimSpace(cfg.Docker.Engine.APIVersion)
	if cfg.Docker.Engine.Timeout <= 0 {
		cfg.Docker.Engine.Timeout = 30 * time.Second
	}
	cfg.Docker.Compose.Binary = strings.TrimSpace(cfg.Docker.Compose.Binary)
	if cfg.Docker.Compose.Binary == "" {
		cfg.Docker.Compose.Binary = "docker"
	}
	cfg.Docker.Compose.TempDir = strings.TrimSpace(cfg.Docker.Compose.TempDir)
	if cfg.Docker.Compose.Timeout <= 0 {
		cfg.Docker.Compose.Timeout = time.Minute
	}
	if cfg.Docker.Compose.OutputMax <= 0 {
		cfg.Docker.Compose.OutputMax = 1024 * 1024
	}
	if cfg.Docker.Registry.Timeout <= 0 {
		cfg.Docker.Registry.Timeout = 15 * time.Second
	}
	if cfg.Docker.Registry.DefaultPageSize <= 0 {
		cfg.Docker.Registry.DefaultPageSize = 20
	}
	if cfg.Docker.Registry.MaxPageSize <= 0 {
		cfg.Docker.Registry.MaxPageSize = 200
	}
	if cfg.Docker.Registry.MaxPages <= 0 {
		cfg.Docker.Registry.MaxPages = 64
	}
	if cfg.Docker.Operation.MaxConcurrent <= 0 {
		cfg.Docker.Operation.MaxConcurrent = 4
	}
	if cfg.Docker.Operation.MaxQueued <= 0 {
		cfg.Docker.Operation.MaxQueued = cfg.Docker.Operation.MaxConcurrent * 16
	}
	if cfg.Docker.Operation.MaxQueued < cfg.Docker.Operation.MaxConcurrent {
		cfg.Docker.Operation.MaxQueued = cfg.Docker.Operation.MaxConcurrent
	}
	if cfg.Docker.Operation.DefaultTimeout <= 0 {
		cfg.Docker.Operation.DefaultTimeout = 10 * time.Minute
	}
	if cfg.Docker.Operation.EventRetentionLimit <= 0 {
		cfg.Docker.Operation.EventRetentionLimit = 1000
	}
	if cfg.Docker.Operation.SSEHeartbeat <= 0 {
		cfg.Docker.Operation.SSEHeartbeat = 15 * time.Second
	}
	if cfg.Docker.Operation.PollInterval <= 0 {
		cfg.Docker.Operation.PollInterval = time.Second
	}
	cfg.Docker.Security.PolicyProfile = strings.ToLower(strings.TrimSpace(cfg.Docker.Security.PolicyProfile))
	if cfg.Docker.Security.PolicyProfile == "" {
		cfg.Docker.Security.PolicyProfile = "compatible"
	}
	cfg.Docker.Security.TrustedRegistries = normalizeStringList(cfg.Docker.Security.TrustedRegistries)
	cfg.Docker.Security.TrustedImages = normalizeStringList(cfg.Docker.Security.TrustedImages)
	cfg.Docker.Security.AllowedNetworks = normalizeStringList(cfg.Docker.Security.AllowedNetworks)
	cfg.Docker.Security.AllowedVolumes = normalizeStringList(cfg.Docker.Security.AllowedVolumes)
	cfg.Docker.Security.SensitiveKeys = normalizeStringList(cfg.Docker.Security.SensitiveKeys)
	cfg.Limiter.KeyPrefix = strings.TrimSpace(cfg.Limiter.KeyPrefix)
	if cfg.Limiter.KeyPrefix == "" {
		cfg.Limiter.KeyPrefix = "seven:limit"
	}
	if cfg.Limiter.DefaultLimit <= 0 {
		cfg.Limiter.DefaultLimit = 60
	}
	if cfg.Limiter.DefaultWindow <= 0 {
		cfg.Limiter.DefaultWindow = time.Minute
	}
	cfg.Email.Provider = strings.ToLower(strings.TrimSpace(cfg.Email.Provider))
	if cfg.Email.Provider == "" {
		cfg.Email.Provider = "mock"
	}
	if cfg.Email.DefaultFrom == "" {
		cfg.Email.DefaultFrom = "SevenFramework <noreply@localhost>"
	}
	if cfg.Email.AppName == "" {
		cfg.Email.AppName = "SevenFramework"
	}
	if cfg.Email.SMTP.Port <= 0 {
		cfg.Email.SMTP.Port = 1025
	}
	if cfg.Email.SMTP.Timeout <= 0 {
		cfg.Email.SMTP.Timeout = 10 * time.Second
	}
	if cfg.Email.Mock.CapturePrefix == "" {
		cfg.Email.Mock.CapturePrefix = "email:mock:capture"
	}
	if cfg.Email.Mock.TTL <= 0 {
		cfg.Email.Mock.TTL = 10 * time.Minute
	}
	cfg.SSO.FrontendLoginURL = strings.TrimSpace(cfg.SSO.FrontendLoginURL)
	if cfg.SSO.FrontendLoginURL == "" {
		cfg.SSO.FrontendLoginURL = "http://127.0.0.1:5177/login"
	}
	if cfg.SSO.LoginTransactionTTLSeconds <= 0 {
		cfg.SSO.LoginTransactionTTLSeconds = 300
	}
	if cfg.SSO.SessionIdleTimeoutSeconds <= 0 {
		cfg.SSO.SessionIdleTimeoutSeconds = 1800
	}
	if cfg.SSO.SessionTouchThrottleSecond <= 0 {
		cfg.SSO.SessionTouchThrottleSecond = 60
	}
	if cfg.SSO.UserinfoTouchThrottleSec <= 0 {
		cfg.SSO.UserinfoTouchThrottleSec = 300
	}
	if cfg.SSO.RefreshReplayClockSkewSec <= 0 {
		cfg.SSO.RefreshReplayClockSkewSec = 30
	}
	if cfg.SSO.RateLimit.TokenLimit <= 0 {
		cfg.SSO.RateLimit.TokenLimit = 60
	}
	if cfg.SSO.RateLimit.TokenWindow <= 0 {
		cfg.SSO.RateLimit.TokenWindow = time.Minute
	}
	if cfg.SSO.RateLimit.UserInfoLimit <= 0 {
		cfg.SSO.RateLimit.UserInfoLimit = 120
	}
	if cfg.SSO.RateLimit.UserInfoWindow <= 0 {
		cfg.SSO.RateLimit.UserInfoWindow = time.Minute
	}
	cfg.SSO.SessionCookie.Name = strings.TrimSpace(cfg.SSO.SessionCookie.Name)
	if cfg.SSO.SessionCookie.Name == "" {
		cfg.SSO.SessionCookie.Name = "SEVEN_SSO_SESSION"
	}
	cfg.SSO.SessionCookie.Path = strings.TrimSpace(cfg.SSO.SessionCookie.Path)
	if cfg.SSO.SessionCookie.Path == "" {
		cfg.SSO.SessionCookie.Path = "/"
	}
	cfg.SSO.SessionCookie.SameSite = strings.TrimSpace(cfg.SSO.SessionCookie.SameSite)
	if cfg.SSO.SessionCookie.SameSite == "" {
		cfg.SSO.SessionCookie.SameSite = "Lax"
	}
	cfg.SSO.RefreshCookie.Name = strings.TrimSpace(cfg.SSO.RefreshCookie.Name)
	if cfg.SSO.RefreshCookie.Name == "" {
		cfg.SSO.RefreshCookie.Name = "__Host-seven_sso_rt"
	}
	cfg.SSO.RefreshCookie.Path = strings.TrimSpace(cfg.SSO.RefreshCookie.Path)
	if cfg.SSO.RefreshCookie.Path == "" {
		cfg.SSO.RefreshCookie.Path = "/"
	}
	cfg.SSO.RefreshCookie.SameSite = strings.TrimSpace(cfg.SSO.RefreshCookie.SameSite)
	if cfg.SSO.RefreshCookie.SameSite == "" {
		cfg.SSO.RefreshCookie.SameSite = "Lax"
	}
	if cfg.SSO.JWT.PrivateKeysByKID == nil {
		cfg.SSO.JWT.PrivateKeysByKID = map[string]string{}
	}
	if cfg.SSO.JWT.PublicKeysByKID == nil {
		cfg.SSO.JWT.PublicKeysByKID = map[string]string{}
	}
	if cfg.SSO.JWT.KeyStatusByKID == nil {
		cfg.SSO.JWT.KeyStatusByKID = map[string]string{}
	}
	cfg.SSO.JWT.CurrentKID = strings.TrimSpace(cfg.SSO.JWT.CurrentKID)
	if cfg.SSO.JWT.CurrentKID == "" {
		cfg.SSO.JWT.CurrentKID = "sso-kid-v1"
	}
	cfg.SSO.JWT.NextKID = strings.TrimSpace(cfg.SSO.JWT.NextKID)
	if cfg.SSO.JWT.NextKID == "" {
		cfg.SSO.JWT.NextKID = "sso-kid-v2"
	}
	for kid, source := range cfg.SSO.JWT.PrivateKeysByKID {
		cfg.SSO.JWT.PrivateKeysByKID[kid] = strings.TrimSpace(source)
	}
	for kid, source := range cfg.SSO.JWT.PublicKeysByKID {
		cfg.SSO.JWT.PublicKeysByKID[kid] = strings.TrimSpace(source)
	}
	for kid, status := range cfg.SSO.JWT.KeyStatusByKID {
		cfg.SSO.JWT.KeyStatusByKID[kid] = strings.ToUpper(strings.TrimSpace(status))
	}
	applyChallengeJWTFallback(cfg)
	cfg.Scheduler.Timezone = strings.TrimSpace(cfg.Scheduler.Timezone)
	if cfg.Scheduler.Timezone == "" {
		cfg.Scheduler.Timezone = "Asia/Shanghai"
	}
	if cfg.Scheduler.Lock.TTL <= 0 {
		cfg.Scheduler.Lock.TTL = time.Minute
	}
	if !cfg.Observability.Enabled {
		cfg.Observability.Prometheus.Enabled = false
		cfg.Observability.Tracing.Enabled = false
		cfg.Observability.Pprof.Enabled = false
		cfg.Observability.Logs.Enabled = false
	}
	if cfg.Observability.SnapshotIntervalMs <= 0 {
		cfg.Observability.SnapshotIntervalMs = 300000
	}
	cfg.Observability.Prometheus.Path = strings.TrimSpace(cfg.Observability.Prometheus.Path)
	if cfg.Observability.Prometheus.Path == "" {
		cfg.Observability.Prometheus.Path = "/ops/prometheus"
	}
	cfg.Observability.Prometheus.AccessToken = strings.TrimSpace(cfg.Observability.Prometheus.AccessToken)
	cfg.Observability.Tracing.ServiceName = strings.TrimSpace(cfg.Observability.Tracing.ServiceName)
	if cfg.Observability.Tracing.ServiceName == "" {
		cfg.Observability.Tracing.ServiceName = cfg.Seven.Name
		if cfg.Observability.Tracing.ServiceName == "" {
			cfg.Observability.Tracing.ServiceName = "seven-agent-force-server"
		}
	}
	cfg.Observability.Tracing.OTLPEndpoint = strings.TrimSpace(cfg.Observability.Tracing.OTLPEndpoint)
	if cfg.Observability.Tracing.ExportTimeout <= 0 {
		cfg.Observability.Tracing.ExportTimeout = 3 * time.Second
	}
	cfg.Observability.Pprof.Prefix = strings.TrimSpace(cfg.Observability.Pprof.Prefix)
	if cfg.Observability.Pprof.Prefix == "" {
		cfg.Observability.Pprof.Prefix = "/ops/debug/pprof"
	}
	if cfg.Observability.Logs.RecentLimit <= 0 {
		cfg.Observability.Logs.RecentLimit = 20
	}
	if cfg.Observability.Logs.ErrorLimit <= 0 {
		cfg.Observability.Logs.ErrorLimit = 10
	}
	if cfg.Observability.Logs.HotLoggerLimit <= 0 {
		cfg.Observability.Logs.HotLoggerLimit = 8
	}
	if cfg.Observability.Logs.TrendBucketSeconds <= 0 {
		cfg.Observability.Logs.TrendBucketSeconds = 300
	}
	if cfg.Server.Host == "" {
		cfg.Server.Host = "0.0.0.0"
	}
	if cfg.Server.Port == 0 {
		cfg.Server.Port = 8888
	}
	if cfg.Server.MaxRequestBodyBytes <= 0 {
		cfg.Server.MaxRequestBodyBytes = 8 * 1024 * 1024
	}
	if cfg.Server.ReadTimeout == 0 {
		cfg.Server.ReadTimeout = 5 * time.Second
	}
	if cfg.Server.WriteTimeout == 0 {
		cfg.Server.WriteTimeout = 10 * time.Second
	}
	if cfg.Server.IdleTimeout == 0 {
		cfg.Server.IdleTimeout = 60 * time.Second
	}
	if cfg.ID.Node == 0 {
		cfg.ID.Node = 1
	}
}

func validateProductionSecurity(cfg *Config) error {
	if cfg == nil || !isProductionEnvironment(cfg) {
		return nil
	}
	violations := make([]string, 0)
	if cfg.SSO.Enabled {
		if !isHTTPSURL(cfg.SSO.Issuer) {
			violations = append(violations, "sso.issuer must use https in production")
		}
		if !isHTTPSURL(cfg.SSO.BaseURL) {
			violations = append(violations, "sso.baseUrl must use https in production")
		}
		if !isHTTPSURL(cfg.SSO.FrontendLoginURL) {
			violations = append(violations, "sso.frontendLoginUrl must use https in production")
		}
		if !cfg.SSO.SessionCookie.Secure {
			violations = append(violations, "sso.sessionCookie.secure must be true in production")
		}
		if !isSameSiteLaxOrStrict(cfg.SSO.SessionCookie.SameSite) {
			violations = append(violations, "sso.sessionCookie.sameSite must be Lax or Strict in production")
		}
		if !cfg.SSO.RefreshCookie.Secure {
			violations = append(violations, "sso.refreshCookie.secure must be true in production")
		}
		if !cfg.SSO.RefreshCookie.HTTPOnly {
			violations = append(violations, "sso.refreshCookie.httpOnly must be true in production")
		}
		if !isSameSiteLaxOrStrict(cfg.SSO.RefreshCookie.SameSite) {
			violations = append(violations, "sso.refreshCookie.sameSite must be Lax or Strict in production")
		}
		if strings.HasPrefix(strings.TrimSpace(cfg.SSO.RefreshCookie.Name), "__Host-") && strings.TrimSpace(cfg.SSO.RefreshCookie.Path) != "/" {
			violations = append(violations, "__Host- sso.refreshCookie.path must be / in production")
		}
		if !cfg.SSO.RateLimit.FailClosedOnError {
			violations = append(violations, "sso.rateLimit.failClosedOnError must be true in production")
		}
	}
	violations = append(violations, productionSetupOriginViolations(cfg.Setup)...)
	violations = append(violations, productionOriginPatternViolations(cfg.Security)...)
	if cfg.Authorization.Internal.Enabled {
		if isWeakProductionSecret(cfg.Authorization.Internal.Token) {
			violations = append(violations, "authorization.internal.token must be production-strength")
		}
		if !cfg.Authorization.Internal.SignatureEnabled {
			violations = append(violations, "authorization.internal.signatureEnabled must be true in production")
		}
		if cfg.Authorization.Internal.SignatureEnabled && isWeakProductionSecret(cfg.Authorization.Internal.SignatureSecret) {
			violations = append(violations, "authorization.internal.signatureSecret must be production-strength")
		}
	}
	if cfg.Authorization.Remote.AcceptGatewayHeaders {
		if !cfg.Authorization.Gateway.SignatureEnabled {
			violations = append(violations, "authorization.gateway.signatureEnabled must be true when accepting gateway headers in production")
		}
		if len(cfg.Authorization.Network.TrustedProxies) == 0 && len(cfg.Authorization.Network.TrustedCIDRs) == 0 {
			violations = append(violations, "authorization.network trusted proxies or CIDRs must be configured when accepting gateway headers in production")
		}
	}
	if cfg.Authorization.Gateway.SignatureEnabled {
		if isWeakProductionSecret(cfg.Authorization.Gateway.Secret) && !hasProductionGatewayVersionSecret(cfg.Authorization.Gateway.SecretsByVersion) {
			violations = append(violations, "authorization.gateway secret must be production-strength")
		}
	}
	violations = append(violations, productionWebAuthnViolations(cfg.Challenge)...)
	if len(violations) > 0 {
		return fmt.Errorf("production config security gate failed: %s", strings.Join(violations, "; "))
	}
	return nil
}

func isProductionEnvironment(cfg *Config) bool {
	env := strings.ToLower(strings.TrimSpace(cfg.Seven.Env))
	profile := strings.ToLower(strings.TrimSpace(cfg.Profile))
	return env == "prod" || env == "production" || profile == "prod" || profile == "production"
}

func isHTTPSURL(value string) bool {
	parsed, err := url.Parse(strings.TrimSpace(value))
	return err == nil && strings.EqualFold(parsed.Scheme, "https") && strings.TrimSpace(parsed.Host) != ""
}

func productionSetupOriginViolations(cfg SetupConfig) []string {
	if !cfg.Enabled {
		return nil
	}
	violations := make([]string, 0, 4)
	if !cfg.RequireOriginHeader {
		violations = append(violations, "setup.requireOriginHeader must be true in production")
	}
	if len(cfg.AllowedOriginPatterns) == 0 {
		violations = append(violations, "setup.allowedOriginPatterns must not be empty in production")
		return violations
	}
	for _, pattern := range cfg.AllowedOriginPatterns {
		value := strings.TrimSpace(pattern)
		if strings.Contains(value, "*") {
			violations = append(violations, "setup.allowedOriginPatterns must not contain wildcards in production")
			continue
		}
		parsed, err := url.Parse(value)
		if err != nil || !strings.EqualFold(parsed.Scheme, "https") || strings.TrimSpace(parsed.Host) == "" {
			violations = append(violations, "setup.allowedOriginPatterns must use https production origins")
			continue
		}
		if parsed.Path != "" || parsed.RawQuery != "" || parsed.Fragment != "" {
			violations = append(violations, "setup.allowedOriginPatterns must be origin URLs in production")
		}
		host := normalizeWebAuthnDomain(parsed.Hostname())
		if !isProductionWebAuthnRPID(host) {
			violations = append(violations, "setup.allowedOriginPatterns must be production domains")
		}
	}
	return uniqueStrings(violations)
}

func productionOriginPatternViolations(cfg SecurityConfig) []string {
	violations := make([]string, 0, len(cfg.OriginPatterns))
	for _, pattern := range cfg.OriginPatterns {
		value := strings.TrimSpace(pattern)
		if strings.Contains(value, "*") {
			violations = append(violations, "security.originPatterns must not contain wildcards in production")
			continue
		}
		parsed, err := url.Parse(value)
		if err != nil || !strings.EqualFold(parsed.Scheme, "https") || strings.TrimSpace(parsed.Host) == "" {
			violations = append(violations, "security.originPatterns must use https production origins")
			continue
		}
		if parsed.Path != "" || parsed.RawQuery != "" || parsed.Fragment != "" {
			violations = append(violations, "security.originPatterns must be origin URLs in production")
		}
	}
	return uniqueStrings(violations)
}

func productionWebAuthnViolations(cfg ChallengeConfig) []string {
	violations := make([]string, 0, 3)
	rpID := normalizeWebAuthnDomain(cfg.WebAuthnRPID)
	if !isProductionWebAuthnRPID(rpID) {
		violations = append(violations, "challenge.webauthnRpId must be production domain")
	}
	if len(cfg.WebAuthnAllowedOrigins) == 0 {
		violations = append(violations, "challenge.webauthnAllowedOrigins must not be empty in production")
		return violations
	}
	for _, origin := range cfg.WebAuthnAllowedOrigins {
		parsed, err := url.Parse(strings.TrimSpace(origin))
		if err != nil || !strings.EqualFold(parsed.Scheme, "https") || strings.TrimSpace(parsed.Host) == "" {
			violations = append(violations, "challenge.webauthnAllowedOrigins must use https")
			continue
		}
		host := normalizeWebAuthnDomain(parsed.Hostname())
		if rpID != "" && !webauthnOriginMatchesRPID(host, rpID) {
			violations = append(violations, "challenge.webauthnAllowedOrigins must match challenge.webauthnRpId")
		}
	}
	return uniqueStrings(violations)
}

func isProductionWebAuthnRPID(value string) bool {
	if value == "" || strings.Contains(value, "://") || strings.Contains(value, ":") {
		return false
	}
	if strings.HasPrefix(value, ".") || strings.Contains(value, "..") {
		return false
	}
	if value == "localhost" || strings.HasSuffix(value, ".localhost") {
		return false
	}
	if net.ParseIP(value) != nil {
		return false
	}
	if isReservedWebAuthnTLD(value) {
		return false
	}
	return strings.Contains(value, ".")
}

func normalizeWebAuthnDomain(value string) string {
	return strings.TrimSuffix(strings.ToLower(strings.TrimSpace(value)), ".")
}

func isReservedWebAuthnTLD(value string) bool {
	parts := strings.Split(value, ".")
	tld := parts[len(parts)-1]
	switch tld {
	case "localhost", "local", "test", "example", "invalid":
		return true
	default:
		return false
	}
}

func webauthnOriginMatchesRPID(originHost, rpID string) bool {
	return originHost == rpID || strings.HasSuffix(originHost, "."+rpID)
}

func uniqueStrings(values []string) []string {
	if len(values) < 2 {
		return values
	}
	result := make([]string, 0, len(values))
	seen := map[string]struct{}{}
	for _, value := range values {
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		result = append(result, value)
	}
	return result
}

func isWeakProductionSecret(value string) bool {
	secret := strings.TrimSpace(value)
	if len(secret) < 24 {
		return true
	}
	lowered := strings.ToLower(secret)
	weakMarkers := []string{"dev", "test", "local", "example", "change-me", "changeme", "default", "secret", "token"}
	for _, marker := range weakMarkers {
		if strings.Contains(lowered, marker) {
			return true
		}
	}
	return false
}

func isSameSiteLaxOrStrict(value string) bool {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "lax", "strict":
		return true
	default:
		return false
	}
}

func hasProductionGatewayVersionSecret(items map[string]string) bool {
	for _, value := range items {
		if !isWeakProductionSecret(value) {
			return true
		}
	}
	return false
}

func (c Config) Address() string {
	return fmt.Sprintf("%s:%d", c.Server.Host, c.Server.Port)
}

func (c Config) ContextPath() string {
	return c.Server.ContextPath
}

func (c Config) ExternalBaseURL() string {
	host := strings.TrimSpace(c.Server.Host)
	if host == "" || host == "0.0.0.0" || host == "::" {
		host = "127.0.0.1"
	}
	port := c.Server.Port
	if port <= 0 {
		port = 8888
	}
	return fmt.Sprintf("http://%s:%d%s", host, port, c.Server.ContextPath)
}

func normalizeContextPath(value string) string {
	value = strings.TrimSpace(value)
	if value == "" || value == "/" {
		return ""
	}
	value = "/" + strings.Trim(value, "/")
	return value
}

func (c MySQLConfig) Configured() bool {
	return c.Enabled && c.DSN != ""
}

func (c PostgresConfig) Configured() bool {
	return c.Enabled && c.DSN != ""
}

func (c DatasourceConfig) Enabled() bool {
	switch c.Driver {
	case "postgres":
		return c.Postgres.Enabled
	case "mysql":
		return c.MySQL.Enabled
	default:
		return false
	}
}

func (c DatasourceConfig) Configured() bool {
	switch c.Driver {
	case "postgres":
		return c.Postgres.Configured()
	case "mysql":
		return c.MySQL.Configured()
	default:
		return false
	}
}

func (c DatasourceBootstrapConfig) StartupEnabled() bool {
	return c.Enabled && (c.Mode == BootstrapModeStartup || c.Mode == BootstrapModeBoth)
}

func (c DatasourceBootstrapConfig) ManualEnabled() bool {
	return c.Enabled && (c.Mode == BootstrapModeManual || c.Mode == BootstrapModeBoth)
}

func (c PlatformConfig) Capabilities() PlatformCapabilities {
	return PlatformCapabilities{
		ControlPlane:      c.Mode == PlatformModeHub,
		FederatedHubLogin: c.Mode == PlatformModeNode,
		NodeAPI:           c.Mode == PlatformModeNode,
	}
}

func validateMicroserviceConfig(cfg MicroserviceConfig) error {
	if _, _, err := net.SplitHostPort(cfg.InternalServer.Listen); err != nil {
		return fmt.Errorf("microservice.internalServer.listen must be a valid host:port: %w", err)
	}
	if cfg.Service.Name == "" {
		return fmt.Errorf("microservice.service.name must not be empty")
	}
	if cfg.Service.Scheme != "http" && cfg.Service.Scheme != "https" {
		return fmt.Errorf("microservice.service.scheme must be one of: http, https")
	}
	if cfg.Service.AdvertisedPort < 0 || cfg.Service.AdvertisedPort > 65535 {
		return fmt.Errorf("microservice.service.advertisedPort must be between 0 and 65535")
	}
	if cfg.Registry.Type != "consul" {
		return fmt.Errorf("microservice.registry.type must be consul")
	}
	if err := validateMicroserviceURL(cfg.Registry.Address, false); err != nil {
		return fmt.Errorf("microservice.registry.address: %w", err)
	}
	if cfg.Registry.RegisterTimeout <= 0 || cfg.Registry.DeregisterTimeout <= 0 {
		return fmt.Errorf("microservice registry timeouts must be greater than zero")
	}
	if cfg.Health.Interval <= 0 || cfg.Health.Timeout <= 0 {
		return fmt.Errorf("microservice health interval and timeout must be greater than zero")
	}
	if cfg.Discovery.Type != "static" && cfg.Discovery.Type != "consul" {
		return fmt.Errorf("microservice.discovery.type must be one of: static, consul")
	}
	if cfg.Discovery.CacheTTL <= 0 || cfg.Discovery.EmptyResultTTL <= 0 || cfg.Discovery.ResolveTimeout <= 0 {
		return fmt.Errorf("microservice discovery TTLs and timeout must be greater than zero")
	}
	if cfg.Client.ConnectTimeout <= 0 || cfg.Client.RequestTimeout <= 0 || cfg.Client.IdleConnTimeout <= 0 {
		return fmt.Errorf("microservice client timeouts must be greater than zero")
	}
	if cfg.Client.MaxIdleConns <= 0 || cfg.Client.MaxIdleConnsPerHost <= 0 {
		return fmt.Errorf("microservice client idle connection limits must be greater than zero")
	}
	if cfg.Client.MaxRequestBytes < 0 || cfg.Client.MaxResponseBytes < 0 {
		return fmt.Errorf("microservice client payload limits must be greater than zero")
	}
	for serviceName, instances := range cfg.Static.Services {
		if serviceName == "" {
			return fmt.Errorf("microservice.static.services contains an empty service name")
		}
		for index, instance := range instances {
			if err := validateMicroserviceURL(instance.URL, true); err != nil {
				return fmt.Errorf("microservice.static.services.%s[%d].url: %w", serviceName, index, err)
			}
		}
	}
	for field, values := range map[string][]string{
		"trustedCidrs": cfg.Outbound.TrustedCIDRs, "registryTrustedCidrs": cfg.Outbound.RegistryTrustedCIDRs,
	} {
		for _, value := range values {
			if _, _, err := net.ParseCIDR(value); err != nil {
				return fmt.Errorf("microservice.outbound.%s contains invalid CIDR %q", field, value)
			}
		}
	}
	for field, values := range map[string][]string{
		"trustedHosts": cfg.Outbound.TrustedHosts, "registryTrustedHosts": cfg.Outbound.RegistryTrustedHosts,
	} {
		for _, value := range values {
			if value == "" || strings.ContainsAny(value, "/@?#") || strings.Contains(value, ":") {
				return fmt.Errorf("microservice.outbound.%s contains invalid hostname %q", field, value)
			}
		}
	}
	return nil
}

func validateMicroserviceURL(value string, requirePort bool) error {
	parsed, err := url.Parse(value)
	if err != nil || parsed.Host == "" || (parsed.Scheme != "http" && parsed.Scheme != "https") {
		return fmt.Errorf("must be an http or https URL")
	}
	if parsed.User != nil || parsed.RawQuery != "" || parsed.Fragment != "" || (parsed.Path != "" && parsed.Path != "/") {
		return fmt.Errorf("must contain only scheme, host and optional port")
	}
	if requirePort && parsed.Port() == "" {
		return fmt.Errorf("must include an explicit port")
	}
	if portText := parsed.Port(); portText != "" {
		port, err := strconv.Atoi(portText)
		if err != nil || port < 1 || port > 65535 {
			return fmt.Errorf("port must be between 1 and 65535")
		}
	}
	return nil
}

func validatePlatformConfig(cfg PlatformConfig) error {
	switch cfg.Mode {
	case PlatformModeLocal, PlatformModeHub, PlatformModeNode:
	default:
		return fmt.Errorf("platform.mode must be one of: local, hub, node")
	}
	if cfg.Mode == PlatformModeNode && cfg.Node.ManagementBearer == "" {
		return fmt.Errorf("platform.node.managementBearer must be configured when platform.mode is node")
	}
	if cfg.Mode == PlatformModeNode {
		if cfg.Node.Code == "" {
			return fmt.Errorf("platform.node.code must be configured when platform.mode is node")
		}
		if _, err := federation.CanonicalManagedOwner(cfg.Node.Code); err != nil || !regexp.MustCompile(`^[a-z0-9][a-z0-9-]*$`).MatchString(cfg.Node.Code) {
			return fmt.Errorf("platform.node.code must use lowercase letters, numbers, and hyphens")
		}
	}
	if cfg.Mode == PlatformModeNode && cfg.Node.InternalListener.Enabled {
		if _, _, err := net.SplitHostPort(cfg.Node.InternalListener.Listen); err != nil {
			return fmt.Errorf("platform.node.internalListener.listen must be a host:port: %w", err)
		}
	}
	return nil
}

func (c CacheConfig) L1Enabled() bool {
	return c.Enabled && c.L1.Enabled
}

func (c SecurityConfig) HasMasterKey() bool {
	return c.Keys.Provider == "local" && strings.TrimSpace(c.Keys.Master.Active.Source) != ""
}

func (c JWTKeysConfig) Configured() bool {
	return strings.EqualFold(strings.TrimSpace(c.Algorithm), "RS256") &&
		strings.TrimSpace(c.Active.KID) != "" &&
		strings.TrimSpace(c.Active.PrivateKeySource) != "" &&
		strings.TrimSpace(c.Active.PublicKeySource) != ""
}

func applyChallengeJWTFallback(cfg *Config) {
	if cfg == nil || cfg.Security.Keys.JWT.Configured() {
		return
	}
	currentKID := strings.TrimSpace(cfg.SSO.JWT.CurrentKID)
	if currentKID == "" {
		return
	}
	privateKeySource := strings.TrimSpace(cfg.SSO.JWT.PrivateKeysByKID[currentKID])
	publicKeySource := strings.TrimSpace(cfg.SSO.JWT.PublicKeysByKID[currentKID])
	if privateKeySource == "" || publicKeySource == "" {
		return
	}
	cfg.Security.Keys.JWT.Algorithm = "RS256"
	cfg.Security.Keys.JWT.Active.KID = currentKID
	cfg.Security.Keys.JWT.Active.PrivateKeySource = privateKeySource
	cfg.Security.Keys.JWT.Active.PublicKeySource = publicKeySource
}

func (c RedisCacheConfig) Configured() bool {
	if !c.Enabled {
		return false
	}
	switch c.Mode {
	case RedisCacheModeSingle:
		return c.Single.Addr != ""
	case RedisCacheModeSentinel:
		return c.Sentinel.MasterName != "" && len(c.Sentinel.Addrs) > 0
	case RedisCacheModeCluster:
		return len(c.Cluster.Addrs) > 0
	default:
		return false
	}
}

func trimValues(values []string) []string {
	if len(values) == 0 {
		return nil
	}
	result := make([]string, 0, len(values))
	for _, value := range values {
		trimmed := strings.TrimSpace(value)
		if trimmed == "" {
			continue
		}
		result = append(result, trimmed)
	}
	return result
}

func lowerTrimValues(values []string) []string {
	trimmed := trimValues(values)
	for index := range trimmed {
		trimmed[index] = strings.ToLower(strings.TrimSuffix(trimmed[index], "."))
	}
	return trimmed
}

func normalizeStringList(values []string) []string {
	if len(values) == 0 {
		return nil
	}
	seen := make(map[string]struct{}, len(values))
	result := make([]string, 0, len(values))
	for _, value := range values {
		trimmed := strings.TrimSpace(value)
		if trimmed == "" {
			continue
		}
		key := strings.ToLower(trimmed)
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		result = append(result, trimmed)
	}
	return result
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}
