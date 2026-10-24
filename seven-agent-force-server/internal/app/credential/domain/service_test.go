package domain

import (
	"context"
	"testing"
	"time"
)

type fakeRepo struct {
	singleAny           *CredentialRecord
	singleActive        *CredentialRecord
	activeByTypeAndKey  *CredentialRecord
	activeRecoveryCodes []CredentialRecord
	activeByUserAndType []CredentialRecord
	inserted            []*CredentialRecord
	updated             *CredentialRecord
	invalidatedCount    int64
	consumed            map[int64]bool
	count               int
}

func (f *fakeRepo) FindSingleActive(ctx context.Context, userID int64, credentialType CredentialType, credentialKey string) (*CredentialRecord, error) {
	return f.singleActive, nil
}
func (f *fakeRepo) FindSingleAny(ctx context.Context, userID int64, credentialType CredentialType, credentialKey string) (*CredentialRecord, error) {
	return f.singleAny, nil
}
func (f *fakeRepo) FindActiveByTypeAndKey(ctx context.Context, credentialType CredentialType, credentialKey string) (*CredentialRecord, error) {
	return f.activeByTypeAndKey, nil
}
func (f *fakeRepo) FindAnyByTypeAndKey(ctx context.Context, credentialType CredentialType, credentialKey string) (*CredentialRecord, error) {
	return nil, nil
}
func (f *fakeRepo) ListActiveByUserAndType(ctx context.Context, userID int64, credentialType CredentialType) ([]CredentialRecord, error) {
	return f.activeByUserAndType, nil
}
func (f *fakeRepo) CountActiveByUserAndType(ctx context.Context, userID int64, credentialType CredentialType) (int, error) {
	return f.count, nil
}
func (f *fakeRepo) Insert(ctx context.Context, record *CredentialRecord) error {
	copied := *record
	f.inserted = append(f.inserted, &copied)
	return nil
}
func (f *fakeRepo) Update(ctx context.Context, record *CredentialRecord) error {
	copied := *record
	f.updated = &copied
	return nil
}
func (f *fakeRepo) UpdateStatusByUserAndType(ctx context.Context, userID int64, credentialType CredentialType, fromStatus CredentialStatus, toStatus CredentialStatus, invalidatedAt time.Time) (int64, error) {
	return 1, nil
}
func (f *fakeRepo) UpdateStatusByScope(ctx context.Context, userID int64, credentialType CredentialType, credentialKey string, fromStatus CredentialStatus, toStatus CredentialStatus, invalidatedAt time.Time) (int64, error) {
	return 1, nil
}
func (f *fakeRepo) UpdateLastUsedByScope(ctx context.Context, userID int64, credentialType CredentialType, credentialKey string, status CredentialStatus, usedAt time.Time) error {
	return nil
}
func (f *fakeRepo) ListActiveRecoveryCodes(ctx context.Context, userID int64) ([]CredentialRecord, error) {
	return f.activeRecoveryCodes, nil
}
func (f *fakeRepo) InvalidateActiveRecoveryCodes(ctx context.Context, userID int64, invalidatedAt time.Time) (int64, error) {
	return f.invalidatedCount, nil
}
func (f *fakeRepo) ConsumeRecoveryCodeByID(ctx context.Context, id int64, usedAt time.Time) (bool, error) {
	return f.consumed[id], nil
}

type fakeIDGen struct{ next int64 }

func (f *fakeIDGen) NextID() int64 {
	f.next++
	return f.next
}

type fakeRecovery struct{}

func (fakeRecovery) GenerateCodes(batchSize int) ([]string, error) {
	return []string{"ABCD-EFGH", "IJKL-MNOP"}[:batchSize], nil
}
func (fakeRecovery) GenerateSalt() (string, error) { return "salt", nil }
func (fakeRecovery) HashCode(code, saltB64 string, iterationCount int) (string, error) {
	return code + ":" + saltB64, nil
}
func (fakeRecovery) VerifyCode(code, saltB64 string, iterationCount int, expectedHashB64 string) bool {
	return expectedHashB64 == code+":"+saltB64
}
func (fakeRecovery) HashAlgorithm() string { return "PBKDF2WithHmacSHA256" }

type fakePayloadCodec struct{}

func (fakePayloadCodec) EncodePasskey(payload PasskeyPayload) (string, error) {
	return payload.PublicKeyCose + "|" + payload.DisplayName, nil
}
func (fakePayloadCodec) DecodePasskey(payload string) (PasskeyPayload, error) {
	if payload == "" || payload == "passkey-bad" {
		return PasskeyPayload{}, context.DeadlineExceeded
	}
	return PasskeyPayload{PublicKeyCose: "public-key", SignCount: 1, DisplayName: "Test Passkey"}, nil
}
func (fakePayloadCodec) EncodeRecoveryCode(payload RecoveryCodePayload) (string, error) {
	return payload.Salt + "|" + payload.HashAlgorithm + "|" + payload.BatchIdentifier, nil
}
func (fakePayloadCodec) DecodeRecoveryCode(payload string) (RecoveryCodePayload, error) {
	if payload == "" {
		return RecoveryCodePayload{}, context.DeadlineExceeded
	}
	if payload == "semantic-bad" {
		return RecoveryCodePayload{}, nil
	}
	return RecoveryCodePayload{Salt: "salt", HashAlgorithm: "PBKDF2WithHmacSHA256", IterationCount: DefaultRecoveryHashIteration, BatchIdentifier: "batch"}, nil
}

func TestUpsertPasswordCredentialReuseExisting(t *testing.T) {
	repo := &fakeRepo{
		singleAny: &CredentialRecord{ID: 7, UserID: 1, CredentialType: CredentialTypePassword, CredentialKey: PrimaryCredentialKey},
	}
	service := NewService(repo, &fakeIDGen{}, fakeRecovery{}, fakePayloadCodec{})
	if err := service.UpsertPasswordCredential(context.Background(), UpsertPasswordInput{
		UserID:       1,
		PasswordHash: "hashed",
	}); err != nil {
		t.Fatalf("upsert password credential: %v", err)
	}
	if repo.updated == nil {
		t.Fatalf("expected updated record")
	}
	if repo.updated.SecretHash != "hashed" || repo.updated.Status != CredentialStatusActive {
		t.Fatalf("unexpected updated record: %#v", repo.updated)
	}
}

func TestRegenerateRecoveryCodesInvalidatesAndInserts(t *testing.T) {
	repo := &fakeRepo{invalidatedCount: 2}
	service := NewService(repo, &fakeIDGen{}, fakeRecovery{}, fakePayloadCodec{})
	result, err := service.RegenerateRecoveryCodes(context.Background(), 1, 2)
	if err != nil {
		t.Fatalf("regenerate recovery codes: %v", err)
	}
	if result == nil || len(result.PlainCodes) != 2 {
		t.Fatalf("unexpected regenerate result: %#v", result)
	}
	if len(repo.inserted) != 2 {
		t.Fatalf("expected 2 inserted recovery codes, got %d", len(repo.inserted))
	}
	if repo.inserted[0].CredentialType != CredentialTypeRecoveryCode {
		t.Fatalf("unexpected credential type: %s", repo.inserted[0].CredentialType)
	}
}

func TestConsumeRecoveryCode(t *testing.T) {
	repo := &fakeRepo{
		activeRecoveryCodes: []CredentialRecord{{ID: 11, SecretHash: "ABCD-EFGH:salt", CredentialPayloadJSON: "payload"}},
		consumed:            map[int64]bool{11: true},
	}
	service := NewService(repo, &fakeIDGen{}, fakeRecovery{}, fakePayloadCodec{})
	ok, err := service.ConsumeRecoveryCode(context.Background(), 1, "ABCD-EFGH", time.Now())
	if err != nil {
		t.Fatalf("consume recovery code: %v", err)
	}
	if !ok {
		t.Fatalf("expected recovery code consumed")
	}
}

func TestConsumeRecoveryCodeSkipsMalformedPayload(t *testing.T) {
	repo := &fakeRepo{
		activeRecoveryCodes: []CredentialRecord{
			{ID: 10, SecretHash: "bad:salt", CredentialPayloadJSON: ""},
			{ID: 11, SecretHash: "ABCD-EFGH:salt", CredentialPayloadJSON: "payload"},
		},
		consumed: map[int64]bool{11: true},
	}
	service := NewService(repo, &fakeIDGen{}, fakeRecovery{}, fakePayloadCodec{})
	ok, err := service.ConsumeRecoveryCode(context.Background(), 1, "ABCD-EFGH", time.Now())
	if err != nil {
		t.Fatalf("consume recovery code with malformed candidate: %v", err)
	}
	if !ok {
		t.Fatal("expected valid recovery code behind malformed payload to succeed")
	}
}

func TestConsumeRecoveryCodeSkipsSemanticallyMalformedPayload(t *testing.T) {
	repo := &fakeRepo{
		activeRecoveryCodes: []CredentialRecord{
			{ID: 10, SecretHash: "ABCD-EFGH:salt", CredentialPayloadJSON: "semantic-bad"},
			{ID: 11, SecretHash: "ABCD-EFGH:salt", CredentialPayloadJSON: "payload"},
		},
		consumed: map[int64]bool{11: true},
	}
	service := NewService(repo, &fakeIDGen{}, fakeRecovery{}, fakePayloadCodec{})
	ok, err := service.ConsumeRecoveryCode(context.Background(), 1, "ABCD-EFGH", time.Now())
	if err != nil {
		t.Fatalf("consume recovery code with semantically malformed candidate: %v", err)
	}
	if !ok {
		t.Fatal("expected valid recovery code behind semantically malformed payload to succeed")
	}
}

func TestListActivePasskeysSkipsMalformedPayload(t *testing.T) {
	repo := &fakeRepo{
		activeByUserAndType: []CredentialRecord{
			{ID: 10, CredentialKey: "bad-passkey", CredentialPayloadJSON: "passkey-bad"},
			{ID: 11, CredentialKey: "good-passkey", CredentialPayloadJSON: "payload"},
		},
	}
	service := NewService(repo, &fakeIDGen{}, fakeRecovery{}, fakePayloadCodec{})
	items, err := service.ListActivePasskeys(context.Background(), 1)
	if err != nil {
		t.Fatalf("list active passkeys with malformed payload: %v", err)
	}
	if len(items) != 1 {
		t.Fatalf("expected 1 valid passkey after skipping malformed payload, got %d", len(items))
	}
	if items[0].CredentialKey != "good-passkey" {
		t.Fatalf("unexpected remaining passkey: %#v", items[0])
	}
}
