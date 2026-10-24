package build

import (
	"fmt"

	bootstrapruntime "github.com/CaixyPromise/seven-agent/force/internal/bootstrap/runtime"
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
)

func Security(cfg config.Config) (bootstrapruntime.SecurityDeps, error) {
	passwordService, err := passwordinfra.New(cfg.Security.Password)
	if err != nil {
		return bootstrapruntime.SecurityDeps{}, fmt.Errorf("build password service: %w", err)
	}

	randomService := randominfra.New(cfg.Security.Random)
	totpService := totpinfra.New()
	recoveryCodeService := recoverycodeinfra.New()

	localKeys, err := keyringinfra.NewLocalProvider(cfg.Security.Keys)
	if err != nil {
		return bootstrapruntime.SecurityDeps{}, fmt.Errorf("build local keyring: %w", err)
	}
	envelopeService := envelopeinfra.NewService(localKeys)
	credentialCodec := credentialcryptoinfra.NewCodec()
	secretValueService := secretvalueinfra.NewService(localKeys)
	jwtService, err := jwtinfra.New(localKeys, cfg.Security.Keys.JWT.Algorithm)
	if err != nil {
		return bootstrapruntime.SecurityDeps{}, fmt.Errorf("build jwt service: %w", err)
	}

	return bootstrapruntime.SecurityDeps{
		Password:        passwordService,
		Random:          randomService,
		Totp:            totpService,
		Recovery:        recoveryCodeService,
		MasterKeys:      localKeys,
		Envelope:        envelopeService,
		CredentialCodec: credentialCodec,
		SecretValue:     secretValueService,
		SigningKeys:     localKeys,
		JWT:             jwtService,
	}, nil
}
