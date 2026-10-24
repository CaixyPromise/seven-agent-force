package application

import (
	"context"
	"errors"
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/CaixyPromise/seven-agent/force/internal/app/workforce/provider/domain"
	secretvalue "github.com/CaixyPromise/seven-agent/force/internal/infrastructure/security/crypto/secretvalue"
)

type repositoryFake struct {
	items           map[int64]domain.Credential
	uses            int
	retainedHistory bool
}

func (r *repositoryFake) Create(_ context.Context, value domain.Credential) (domain.Credential, error) {
	r.items[value.ID] = value
	return value, nil
}
func (r *repositoryFake) ListTenant(_ context.Context, tenant int64) ([]domain.Credential, error) {
	result := []domain.Credential{}
	for _, value := range r.items {
		if value.TenantID == tenant {
			result = append(result, value)
		}
	}
	return result, nil
}
func (r *repositoryFake) GetTenant(_ context.Context, tenant, id int64) (domain.Credential, error) {
	value, ok := r.items[id]
	if !ok || value.TenantID != tenant {
		return domain.Credential{}, ErrNotFound
	}
	return value, nil
}
func (r *repositoryFake) GetTenantForShare(ctx context.Context, _ pgx.Tx, tenant, id int64) (domain.Credential, error) {
	return r.GetTenant(ctx, tenant, id)
}
func (r *repositoryFake) WithLockedCredential(ctx context.Context, tenant, id int64, fn func(domain.Credential, CredentialUpdateRepository) error) error {
	item, err := r.GetTenant(ctx, tenant, id)
	if err != nil {
		return err
	}
	return fn(item, r)
}
func (r *repositoryFake) Update(_ context.Context, value domain.Credential, rotate bool) (domain.Credential, error) {
	value.Version++
	if rotate {
		value.SecretVersion++
	}
	r.items[value.ID] = value
	return value, nil
}
func (*repositoryFake) UpdateDiscovery(context.Context, int64, int64, string, string) error {
	return nil
}
func (r *repositoryFake) CountActiveProtocolUses(context.Context, int64, int64) (int, error) {
	return r.uses, nil
}
func (r *repositoryFake) Delete(_ context.Context, tenantID, id, expectedVersion int64) (domain.DeleteResult, error) {
	value, ok := r.items[id]
	if !ok || value.TenantID != tenantID {
		return domain.DeleteResult{}, ErrNotFound
	}
	if value.Version != expectedVersion {
		return domain.DeleteResult{}, ErrVersionConflict
	}
	if r.retainedHistory {
		value.Status = domain.StatusDeleted
		value.SecretCiphertext, value.SecretEDEK, value.SecretWrapKeyRef, value.SecretHint = "", "", "", ""
		value.Models = nil
		r.items[id] = value
	} else {
		delete(r.items, id)
	}
	return domain.DeleteResult{Deleted: true, RetainedHistory: r.retainedHistory}, nil
}

type secretFake struct{}

func (secretFake) EncryptString(_ context.Context, value string) (secretvalue.SecretValue, error) {
	return secretvalue.SecretValue{CiphertextB64: "cipher:" + value, EDEKB64: "edek", WrapKeyRef: "key"}, nil
}
func (secretFake) DecryptString(_ context.Context, value secretvalue.SecretValue) (string, error) {
	return value.CiphertextB64[len("cipher:"):], nil
}
func (secretFake) EncryptBytes(context.Context, []byte) (secretvalue.SecretValue, error) {
	return secretvalue.SecretValue{}, nil
}
func (secretFake) DecryptBytes(context.Context, secretvalue.SecretValue) ([]byte, error) {
	return nil, nil
}

type idFake struct{ id int64 }

func (i *idFake) NextID() int64 { i.id++; return i.id }

func TestCatalogIsOnlyTaskSelectionBoundary(t *testing.T) {
	repository := &repositoryFake{items: map[int64]domain.Credential{7: {
		ID: 7, TenantID: 1, Name: "key", Protocol: domain.ProtocolOpenAI,
		BaseURL: "https://models.example/v1", Models: []string{"primary"},
		SecretCiphertext: "cipher:secret", SecretEDEK: "edek", SecretVersion: 3,
		Status: domain.StatusActive, Version: 1,
	}}}
	service := NewService(repository, secretFake{}, nil, &idFake{id: 10})
	if _, err := service.ValidateTaskSelection(context.Background(), 1, 7, "helper"); !errors.Is(err, ErrModelNotInCatalog) {
		t.Fatalf("task selection err=%v", err)
	}
	credential, err := service.ResolveInvocationCredential(context.Background(), 1, 7, 1, "primary")
	if err != nil {
		t.Fatal(err)
	}
	if credential.CredentialVersion != 3 || credential.APIKey != "secret" || credential.PrimaryModel != "primary" {
		t.Fatalf("credential=%+v", credential)
	}
}

func TestCreateNormalizesCatalogOrderAndKeyIsNotReturned(t *testing.T) {
	repository := &repositoryFake{items: map[int64]domain.Credential{}}
	service := NewService(repository, secretFake{}, nil, &idFake{id: 100})
	view, err := service.Create(context.Background(), 1, CreateCommand{Name: " key ", Protocol: "anthropic",
		BaseURL: "http://models.internal/v1/", APIKey: "top-secret", Models: []string{" b ", "a", "b"}})
	if err != nil {
		t.Fatal(err)
	}
	if view.Protocol != domain.ProtocolAnthropic || view.BaseURL != "http://models.internal/v1" || len(view.Models) != 2 || view.Models[0] != "b" || view.Models[1] != "a" {
		t.Fatalf("view=%+v", view)
	}
	if view.SecretHint == "top-secret" || !view.CredentialConfigured {
		t.Fatalf("unsafe view=%+v", view)
	}
}

func TestProtocolChangeIsFencedWhileInUse(t *testing.T) {
	repository := &repositoryFake{uses: 1, items: map[int64]domain.Credential{7: {
		ID: 7, TenantID: 1, Name: "key", Protocol: domain.ProtocolOpenAI, BaseURL: "https://models.example/v1",
		Models: []string{"m"}, SecretCiphertext: "cipher:key", SecretEDEK: "edek", SecretVersion: 1,
		Status: domain.StatusActive, Version: 2,
	}}}
	service := NewService(repository, secretFake{}, nil, &idFake{})
	_, err := service.Update(context.Background(), 1, 7, UpdateCommand{ExpectedVersion: 2, Name: "key", Protocol: "ANTHROPIC", BaseURL: "https://models.example/v1", Models: []string{"m"}})
	if !errors.Is(err, ErrProtocolInUse) {
		t.Fatalf("err=%v", err)
	}
}

type invocationCancellerFake struct {
	credentialID int64
	reason       string
}

func (c *invocationCancellerFake) CancelModelInvocations(_ context.Context, _, credentialID int64, reason string) error {
	c.credentialID, c.reason = credentialID, reason
	return nil
}

func TestDeletePhysicallyRemovesUnreferencedCredential(t *testing.T) {
	repository := &repositoryFake{items: map[int64]domain.Credential{7: {ID: 7, TenantID: 1, Version: 2}}}
	service := NewService(repository, secretFake{}, nil, &idFake{})
	result, err := service.Delete(context.Background(), 1, 7, 2)
	if err != nil {
		t.Fatal(err)
	}
	if !result.Deleted || result.RetainedHistory {
		t.Fatalf("result=%+v", result)
	}
	if _, exists := repository.items[7]; exists {
		t.Fatal("unreferenced credential was retained")
	}
}

func TestDeleteClearsSecretAndCancelsCallsWhenHistoryExists(t *testing.T) {
	repository := &repositoryFake{retainedHistory: true, items: map[int64]domain.Credential{7: {
		ID: 7, TenantID: 1, Version: 2, Status: domain.StatusActive,
		SecretCiphertext: "cipher:key", SecretEDEK: "edek", SecretWrapKeyRef: "wrap", Models: []string{"model"},
	}}}
	canceller := &invocationCancellerFake{}
	service := NewService(repository, secretFake{}, nil, &idFake{})
	service.SetInvocationCanceller(canceller)
	result, err := service.Delete(context.Background(), 1, 7, 2)
	if err != nil {
		t.Fatal(err)
	}
	stored := repository.items[7]
	if !result.RetainedHistory || stored.Status != domain.StatusDeleted || stored.SecretCiphertext != "" || len(stored.Models) != 0 {
		t.Fatalf("result=%+v stored=%+v", result, stored)
	}
	if canceller.credentialID != 7 || canceller.reason != "MODEL_CREDENTIAL_DELETED" {
		t.Fatalf("canceller=%+v", canceller)
	}
}
