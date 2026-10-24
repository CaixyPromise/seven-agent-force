package keyring

import (
	"context"
	"crypto/rsa"
	"crypto/x509"
	"encoding/base64"
	"encoding/pem"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/CaixyPromise/seven-agent/force/internal/shared/config"
)

type LocalProvider struct {
	master       *MasterKey
	mastersByKID map[string]*MasterKey
	active       *SigningKey
	next         *SigningKey
	retired      []SigningKey
}

func NewLocalProvider(cfg config.KeysConfig) (*LocalProvider, error) {
	provider := &LocalProvider{
		mastersByKID: make(map[string]*MasterKey),
	}
	if cfg.Master.Active.Source != "" {
		master, err := buildMasterKey(cfg.Master.Active)
		if err != nil {
			return nil, fmt.Errorf("load master key: %w", err)
		}
		provider.master = master
		provider.mastersByKID[master.KID] = cloneMasterKey(master)
	}
	for _, item := range cfg.Master.Retired {
		master, err := buildMasterKey(item)
		if err != nil {
			return nil, fmt.Errorf("load retired master key %s: %w", item.KID, err)
		}
		if master != nil {
			provider.mastersByKID[master.KID] = cloneMasterKey(master)
		}
	}

	active, err := buildSigningKey(cfg.JWT.Algorithm, cfg.JWT.Active, KeyStatusActive)
	if err != nil {
		return nil, fmt.Errorf("load active signing key: %w", err)
	}
	provider.active = active

	next, err := buildSigningKey(cfg.JWT.Algorithm, cfg.JWT.Next, KeyStatusNext)
	if err != nil {
		return nil, fmt.Errorf("load next signing key: %w", err)
	}
	provider.next = next

	for _, item := range cfg.JWT.Retired {
		key, err := buildSigningKey(cfg.JWT.Algorithm, item, KeyStatusRetired)
		if err != nil {
			return nil, fmt.Errorf("load retired signing key %s: %w", item.KID, err)
		}
		if key != nil {
			provider.retired = append(provider.retired, *key)
		}
	}
	return provider, nil
}

func (p *LocalProvider) Current(ctx context.Context) (*MasterKey, error) {
	_ = ctx
	if p == nil || p.master == nil {
		return nil, nil
	}
	return cloneMasterKey(p.master), nil
}

func (p *LocalProvider) ByKID(ctx context.Context, kid string) (*MasterKey, error) {
	_ = ctx
	if p == nil || strings.TrimSpace(kid) == "" {
		return nil, nil
	}
	key := p.mastersByKID[strings.TrimSpace(kid)]
	if key == nil {
		return nil, nil
	}
	return cloneMasterKey(key), nil
}

func (p *LocalProvider) Active(ctx context.Context) (*SigningKey, error) {
	_ = ctx
	return cloneSigningKey(p.active), nil
}

func (p *LocalProvider) Next(ctx context.Context) (*SigningKey, error) {
	_ = ctx
	return cloneSigningKey(p.next), nil
}

func (p *LocalProvider) VerifyKeys(ctx context.Context, now time.Time) ([]SigningKey, error) {
	_ = ctx
	keys := make([]SigningKey, 0, 2+len(p.retired))
	if p.active != nil {
		keys = append(keys, *cloneSigningKey(p.active))
	}
	if p.next != nil {
		keys = append(keys, *cloneSigningKey(p.next))
	}
	for _, retired := range p.retired {
		if retired.VerifyUntil == nil || !retired.VerifyUntil.Before(now) {
			keys = append(keys, *cloneSigningKey(&retired))
		}
	}
	return keys, nil
}

func buildSigningKey(algorithm string, cfg config.JWTKeySourceConfig, status KeyStatus) (*SigningKey, error) {
	if strings.TrimSpace(cfg.PrivateKeySource) == "" && strings.TrimSpace(cfg.PublicKeySource) == "" {
		return nil, nil
	}
	if strings.TrimSpace(cfg.KID) == "" {
		return nil, fmt.Errorf("jwt key kid must not be empty")
	}
	if strings.TrimSpace(cfg.PublicKeySource) == "" {
		return nil, fmt.Errorf("jwt public key source must not be empty for %s", cfg.KID)
	}
	publicPEM, err := resolveSource(cfg.PublicKeySource)
	if err != nil {
		return nil, err
	}
	publicKey, err := parseRSAPublicKey(publicPEM)
	if err != nil {
		return nil, fmt.Errorf("parse public key %s: %w", cfg.KID, err)
	}

	var privateKey *rsa.PrivateKey
	if strings.TrimSpace(cfg.PrivateKeySource) != "" {
		privatePEM, err := resolveSource(cfg.PrivateKeySource)
		if err != nil {
			return nil, err
		}
		privateKey, err = parseRSAPrivateKey(privatePEM)
		if err != nil {
			return nil, fmt.Errorf("parse private key %s: %w", cfg.KID, err)
		}
	}
	var verifyUntil *time.Time
	if cfg.VerifyUntil > 0 {
		value := time.Now().Add(cfg.VerifyUntil)
		verifyUntil = &value
	}
	return &SigningKey{
		KID:         cfg.KID,
		Status:      status,
		Algorithm:   algorithm,
		PrivateKey:  privateKey,
		PublicKey:   publicKey,
		VerifyUntil: verifyUntil,
	}, nil
}

func buildMasterKey(cfg config.MasterKeySourceConfig) (*MasterKey, error) {
	if strings.TrimSpace(cfg.KID) == "" && strings.TrimSpace(cfg.Source) == "" {
		return nil, nil
	}
	if strings.TrimSpace(cfg.KID) == "" {
		return nil, fmt.Errorf("master key kid must not be empty")
	}
	if strings.TrimSpace(cfg.Source) == "" {
		return nil, fmt.Errorf("master key source must not be empty for %s", cfg.KID)
	}
	material, err := resolveSource(cfg.Source)
	if err != nil {
		return nil, err
	}
	trimmed := strings.TrimSpace(string(material))
	if decoded, err := base64.StdEncoding.DecodeString(trimmed); err == nil && len(decoded) > 0 {
		material = decoded
	}
	return &MasterKey{
		KID:      cfg.KID,
		Material: append([]byte(nil), material...),
	}, nil
}

func resolveSource(source string) ([]byte, error) {
	source = strings.TrimSpace(source)
	switch {
	case strings.HasPrefix(source, "env:"):
		value := os.Getenv(strings.TrimPrefix(source, "env:"))
		if value == "" {
			return nil, fmt.Errorf("environment source %s is empty", source)
		}
		return []byte(value), nil
	case strings.HasPrefix(source, "file:"):
		return os.ReadFile(strings.TrimPrefix(source, "file:"))
	case strings.HasPrefix(source, "classpath:"):
		return os.ReadFile(filepath.Clean(strings.TrimPrefix(source, "classpath:")))
	default:
		return []byte(source), nil
	}
}

func parseRSAPrivateKey(raw []byte) (*rsa.PrivateKey, error) {
	block, _ := pem.Decode(raw)
	if block == nil {
		return nil, fmt.Errorf("invalid private key pem")
	}
	if key, err := x509.ParsePKCS1PrivateKey(block.Bytes); err == nil {
		return key, nil
	}
	privateKey, err := x509.ParsePKCS8PrivateKey(block.Bytes)
	if err != nil {
		return nil, err
	}
	rsaKey, ok := privateKey.(*rsa.PrivateKey)
	if !ok {
		return nil, fmt.Errorf("private key is not rsa")
	}
	return rsaKey, nil
}

func parseRSAPublicKey(raw []byte) (*rsa.PublicKey, error) {
	block, _ := pem.Decode(raw)
	if block == nil {
		if decoded, err := base64.StdEncoding.DecodeString(strings.TrimSpace(string(raw))); err == nil {
			block, _ = pem.Decode(decoded)
		}
	}
	if block == nil {
		return nil, fmt.Errorf("invalid public key pem")
	}
	if publicKey, err := x509.ParsePKIXPublicKey(block.Bytes); err == nil {
		rsaKey, ok := publicKey.(*rsa.PublicKey)
		if !ok {
			return nil, fmt.Errorf("public key is not rsa")
		}
		return rsaKey, nil
	}
	cert, err := x509.ParseCertificate(block.Bytes)
	if err != nil {
		return nil, err
	}
	rsaKey, ok := cert.PublicKey.(*rsa.PublicKey)
	if !ok {
		return nil, fmt.Errorf("certificate public key is not rsa")
	}
	return rsaKey, nil
}

func cloneSigningKey(value *SigningKey) *SigningKey {
	if value == nil {
		return nil
	}
	clone := *value
	return &clone
}

func cloneMasterKey(value *MasterKey) *MasterKey {
	if value == nil {
		return nil
	}
	clone := *value
	clone.Material = append([]byte(nil), value.Material...)
	return &clone
}
