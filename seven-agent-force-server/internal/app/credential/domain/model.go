package domain

import "time"

type CredentialType string

const (
	CredentialTypePassword       CredentialType = "PASSWORD"
	CredentialTypeTotp           CredentialType = "TOTP"
	CredentialTypePasskey        CredentialType = "PASSKEY"
	CredentialTypeRecoveryCode   CredentialType = "RECOVERY_CODE"
	PrimaryCredentialKey                        = "PRIMARY"
	DefaultRecoveryHashIteration                = 210000
)

type CredentialStatus int

const (
	CredentialStatusActive CredentialStatus = iota
	CredentialStatusDisabled
	CredentialStatusConsumed
	CredentialStatusInvalidated
)

type CredentialRecord struct {
	ID                    int64
	UserID                int64
	CredentialType        CredentialType
	CredentialKey         string
	SecretHash            string
	SecretCiphertext      string
	CredentialPayloadJSON string
	Status                CredentialStatus
	VerifiedAt            *time.Time
	LastUsedAt            *time.Time
	InvalidatedAt         *time.Time
	MetadataJSON          string
	MustChangePassword    bool
	PasswordChangedAt     *time.Time
	CreatorID             *int64
	CreateTime            *time.Time
	UpdaterID             *int64
	UpdateTime            *time.Time
	IsDeleted             int
}

type PasswordCredential struct {
	UserID             int64
	CredentialKey      string
	PasswordHash       string
	MustChangePassword bool
	PasswordChangedAt  *time.Time
	LastUsedAt         *time.Time
	VerifiedAt         *time.Time
}

type TotpCredential struct {
	UserID           int64
	CredentialKey    string
	SecretCiphertext string
	VerifiedAt       *time.Time
	LastUsedAt       *time.Time
}

type TotpSecret struct {
	UserID        int64
	CredentialKey string
	Secret        string
	VerifiedAt    *time.Time
	LastUsedAt    *time.Time
}

type PasskeyPayload struct {
	PublicKeyCose     string `json:"publicKeyCose"`
	SignCount         int64  `json:"signCount"`
	UserHandle        string `json:"userHandle,omitempty"`
	AAGUID            string `json:"aaguid,omitempty"`
	Transports        string `json:"transports,omitempty"`
	AttestationFormat string `json:"attestationFormat,omitempty"`
	DisplayName       string `json:"displayName,omitempty"`
}

type PasskeyCredential struct {
	UserID            int64
	CredentialKey     string
	PublicKeyCose     string
	SignCount         int64
	UserHandle        string
	AAGUID            string
	Transports        string
	AttestationFormat string
	DisplayName       string
	VerifiedAt        *time.Time
	CreateTime        *time.Time
	LastUsedAt        *time.Time
}

type UpsertPasswordInput struct {
	UserID             int64
	PasswordHash       string
	MustChangePassword bool
	PasswordChangedAt  *time.Time
	CreatorID          *int64
	UpdaterID          *int64
}

type UpsertTotpInput struct {
	UserID           int64
	SecretCiphertext string
	VerifiedAt       *time.Time
	CreatorID        *int64
	UpdaterID        *int64
}

type SavePasskeyInput struct {
	UserID            int64
	CredentialKey     string
	PublicKeyCose     string
	SignCount         int64
	UserHandle        string
	AAGUID            string
	Transports        string
	AttestationFormat string
	DisplayName       string
	DisableExisting   bool
	VerifiedAt        *time.Time
	CreatorID         *int64
	UpdaterID         *int64
}

type RecoveryCodePayload struct {
	Salt            string `json:"salt"`
	HashAlgorithm   string `json:"hashAlgorithm"`
	IterationCount  int    `json:"iterationCount"`
	BatchIdentifier string `json:"batchIdentifier"`
}

type RecoveryCodeRecord struct {
	PlainCode string
	Hash      string
	Payload   RecoveryCodePayload
}

type RegeneratedRecoveryCodes struct {
	BatchIdentifier string
	PlainCodes      []string
	GeneratedAt     *time.Time
}
