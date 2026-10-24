package keyring

import (
	"context"
	"crypto/rsa"
	"time"
)

type KeyStatus string

const (
	KeyStatusActive  KeyStatus = "ACTIVE"
	KeyStatusNext    KeyStatus = "NEXT"
	KeyStatusRetired KeyStatus = "RETIRED"
)

type MasterKey struct {
	KID      string
	Material []byte
}

type SigningKey struct {
	KID         string
	Status      KeyStatus
	Algorithm   string
	PrivateKey  *rsa.PrivateKey
	PublicKey   *rsa.PublicKey
	VerifyUntil *time.Time
}

type MasterKeyProvider interface {
	Current(ctx context.Context) (*MasterKey, error)
	ByKID(ctx context.Context, kid string) (*MasterKey, error)
}

type SigningKeyProvider interface {
	Active(ctx context.Context) (*SigningKey, error)
	Next(ctx context.Context) (*SigningKey, error)
	VerifyKeys(ctx context.Context, now time.Time) ([]SigningKey, error)
}
