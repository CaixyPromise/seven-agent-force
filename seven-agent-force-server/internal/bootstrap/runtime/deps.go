package runtime

import (
	cacheinfra "github.com/CaixyPromise/seven-agent/force/internal/infrastructure/cache"
	"github.com/CaixyPromise/seven-agent/force/internal/infrastructure/datasource/store"
	dockerinfra "github.com/CaixyPromise/seven-agent/force/internal/infrastructure/docker"
	emailinfra "github.com/CaixyPromise/seven-agent/force/internal/infrastructure/email"
	jobscheduler "github.com/CaixyPromise/seven-agent/force/internal/infrastructure/job/scheduler"
	limiterinfra "github.com/CaixyPromise/seven-agent/force/internal/infrastructure/limiter"
	lockinfra "github.com/CaixyPromise/seven-agent/force/internal/infrastructure/lock"
	rabbitmqinfra "github.com/CaixyPromise/seven-agent/force/internal/infrastructure/messaging/rabbitmq"
	obsinfra "github.com/CaixyPromise/seven-agent/force/internal/infrastructure/observability"
	credentialcryptoinfra "github.com/CaixyPromise/seven-agent/force/internal/infrastructure/security/crypto/credentialcrypto"
	envelopeinfra "github.com/CaixyPromise/seven-agent/force/internal/infrastructure/security/crypto/envelope"
	secretvalueinfra "github.com/CaixyPromise/seven-agent/force/internal/infrastructure/security/crypto/secretvalue"
	jwtinfra "github.com/CaixyPromise/seven-agent/force/internal/infrastructure/security/jwt"
	keyringinfra "github.com/CaixyPromise/seven-agent/force/internal/infrastructure/security/keyring"
	passwordinfra "github.com/CaixyPromise/seven-agent/force/internal/infrastructure/security/password"
	randominfra "github.com/CaixyPromise/seven-agent/force/internal/infrastructure/security/random"
	recoverycodeinfra "github.com/CaixyPromise/seven-agent/force/internal/infrastructure/security/recoverycode"
	totpinfra "github.com/CaixyPromise/seven-agent/force/internal/infrastructure/security/totp"
	"github.com/CaixyPromise/seven-agent/force/internal/shared/config"
	"github.com/CaixyPromise/seven-agent/force/internal/shared/core"
	"github.com/CaixyPromise/seven-agent/force/internal/shared/features"
	"github.com/CaixyPromise/seven-agent/force/internal/shared/xid"
	"github.com/cloudwego/hertz/pkg/app/server"
	"github.com/cloudwego/hertz/pkg/route"
	"go.uber.org/zap"
)

// PublicRouteMounter installs routes under the configured public API context path.
type PublicRouteMounter interface {
	MountPublic(route.IRouter)
}

// HubRouteMounter installs Hub-only control-plane routes on the public API router.
type HubRouteMounter interface {
	MountHub(route.IRouter)
}

// InternalRouteMounter installs Node-only management routes outside the public context path.
type InternalRouteMounter interface {
	MountInternal(route.IRouter)
}

type InfraDeps struct {
	Datasource store.Provider
	Transactor store.Transactor
	Cache      cacheinfra.Provider
	CacheMgr   cacheinfra.Manager
	Locker     lockinfra.Service
	Limiter    limiterinfra.Limiter
	Jobs       jobscheduler.Scheduler
	RabbitMQ   *rabbitmqinfra.Client
	Email      emailinfra.Sender
	Docker     dockerinfra.Service
	Obs        obsinfra.Manager
	HTTPServer *server.Hertz
}

type SecurityDeps struct {
	Password        *passwordinfra.Service
	Random          *randominfra.Service
	Totp            *totpinfra.Service
	Recovery        *recoverycodeinfra.Service
	MasterKeys      keyringinfra.MasterKeyProvider
	Envelope        envelopeinfra.Service
	CredentialCodec credentialcryptoinfra.Codec
	SecretValue     secretvalueinfra.Service
	SigningKeys     keyringinfra.SigningKeyProvider
	JWT             *jwtinfra.Service
}

type ModuleDeps struct {
	Config   config.Config
	Features features.Set
	Logger   *zap.Logger
	IDGen    *xid.Generator
	Registry core.ModuleCatalog
	Infra    InfraDeps
	Security SecurityDeps
}
