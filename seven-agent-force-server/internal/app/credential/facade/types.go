package facade

import "time"

type PasswordCredential struct {
	UserID             int64      `json:"userId"`
	CredentialKey      string     `json:"credentialKey"`
	PasswordHash       string     `json:"passwordHash"`
	MustChangePassword bool       `json:"mustChangePassword"`
	PasswordChangedAt  *time.Time `json:"passwordChangedAt,omitempty"`
	LastUsedAt         *time.Time `json:"lastUsedAt,omitempty"`
	VerifiedAt         *time.Time `json:"verifiedAt,omitempty"`
}

type TotpCredential struct {
	UserID           int64      `json:"userId"`
	CredentialKey    string     `json:"credentialKey"`
	SecretCiphertext string     `json:"secretCiphertext"`
	VerifiedAt       *time.Time `json:"verifiedAt,omitempty"`
	LastUsedAt       *time.Time `json:"lastUsedAt,omitempty"`
}

type TotpSecret struct {
	UserID        int64      `json:"userId"`
	CredentialKey string     `json:"credentialKey"`
	Secret        string     `json:"secret"`
	VerifiedAt    *time.Time `json:"verifiedAt,omitempty"`
	LastUsedAt    *time.Time `json:"lastUsedAt,omitempty"`
}

type PasskeyCredential struct {
	UserID            int64      `json:"userId"`
	CredentialKey     string     `json:"credentialKey"`
	PublicKeyCose     string     `json:"publicKeyCose"`
	SignCount         int64      `json:"signCount"`
	UserHandle        string     `json:"userHandle,omitempty"`
	AAGUID            string     `json:"aaguid,omitempty"`
	Transports        string     `json:"transports,omitempty"`
	AttestationFormat string     `json:"attestationFormat,omitempty"`
	DisplayName       string     `json:"displayName,omitempty"`
	VerifiedAt        *time.Time `json:"verifiedAt,omitempty"`
	CreateTime        *time.Time `json:"createTime,omitempty"`
	LastUsedAt        *time.Time `json:"lastUsedAt,omitempty"`
}

type UpsertPasswordCredentialCommand struct {
	UserID             int64      `json:"userId"`
	PasswordHash       string     `json:"passwordHash"`
	MustChangePassword bool       `json:"mustChangePassword"`
	PasswordChangedAt  *time.Time `json:"passwordChangedAt,omitempty"`
	CreatorID          *int64     `json:"creatorId,omitempty"`
	UpdaterID          *int64     `json:"updaterId,omitempty"`
}

type UpsertTotpCredentialCommand struct {
	UserID           int64      `json:"userId"`
	SecretCiphertext string     `json:"secretCiphertext"`
	VerifiedAt       *time.Time `json:"verifiedAt,omitempty"`
	CreatorID        *int64     `json:"creatorId,omitempty"`
	UpdaterID        *int64     `json:"updaterId,omitempty"`
}

type SavePasskeyCredentialCommand struct {
	UserID            int64      `json:"userId"`
	CredentialKey     string     `json:"credentialKey"`
	PublicKeyCose     string     `json:"publicKeyCose"`
	SignCount         int64      `json:"signCount"`
	UserHandle        string     `json:"userHandle,omitempty"`
	AAGUID            string     `json:"aaguid,omitempty"`
	Transports        string     `json:"transports,omitempty"`
	AttestationFormat string     `json:"attestationFormat,omitempty"`
	DisplayName       string     `json:"displayName,omitempty"`
	DisableExisting   bool       `json:"disableExisting"`
	VerifiedAt        *time.Time `json:"verifiedAt,omitempty"`
	CreatorID         *int64     `json:"creatorId,omitempty"`
	UpdaterID         *int64     `json:"updaterId,omitempty"`
}

type CompleteTotpBindingCommand struct {
	UserID            int64      `json:"userId"`
	PlainSecret       string     `json:"plainSecret"`
	VerifiedAt        *time.Time `json:"verifiedAt,omitempty"`
	RecoveryBatchSize int        `json:"recoveryBatchSize"`
}

type CompletePasskeyBindingCommand struct {
	UserID            int64      `json:"userId"`
	CredentialKey     string     `json:"credentialKey"`
	PublicKeyCose     string     `json:"publicKeyCose"`
	SignCount         int64      `json:"signCount"`
	UserHandle        string     `json:"userHandle,omitempty"`
	AAGUID            string     `json:"aaguid,omitempty"`
	Transports        string     `json:"transports,omitempty"`
	AttestationFormat string     `json:"attestationFormat,omitempty"`
	DisplayName       string     `json:"displayName,omitempty"`
	DisableExisting   bool       `json:"disableExisting"`
	VerifiedAt        *time.Time `json:"verifiedAt,omitempty"`
	RecoveryBatchSize int        `json:"recoveryBatchSize"`
}

type RegeneratedRecoveryCodes struct {
	BatchIdentifier string     `json:"batchIdentifier"`
	PlainCodes      []string   `json:"plainCodes"`
	GeneratedAt     *time.Time `json:"generatedAt,omitempty"`
}
