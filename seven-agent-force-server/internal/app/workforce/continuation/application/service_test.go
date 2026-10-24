package application

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"testing"

	"github.com/CaixyPromise/seven-agent/force/internal/app/workforce/continuation/domain"
)

type repositoryStub struct {
	createMaterialization func(context.Context, CreateMaterializationRequest) (domain.MaterializationView, bool, error)
	claimMaterialization  func(context.Context, MachineAuthority, int64) (domain.MaterializationClaim, error)
}

func (r repositoryStub) CreateNextTurn(context.Context, CreateNextTurnRequest) (domain.Turn, bool, error) {
	return domain.Turn{}, false, errors.New("unused")
}
func (r repositoryStub) Approve(context.Context, ApproveRequest) (domain.Checkpoint, error) {
	return domain.Checkpoint{}, errors.New("unused")
}
func (r repositoryStub) ListTurns(context.Context, int64, int64) ([]domain.Turn, error) {
	return nil, errors.New("unused")
}
func (r repositoryStub) GetTurn(context.Context, int64, int64) (domain.Turn, error) {
	return domain.Turn{}, errors.New("unused")
}
func (r repositoryStub) GetCheckpoint(context.Context, int64, int64) (domain.Checkpoint, error) {
	return domain.Checkpoint{}, errors.New("unused")
}
func (r repositoryStub) CreateMaterialization(ctx context.Context, request CreateMaterializationRequest) (domain.MaterializationView, bool, error) {
	return r.createMaterialization(ctx, request)
}
func (r repositoryStub) GetMaterialization(context.Context, int64, int64) (domain.MaterializationView, error) {
	return domain.MaterializationView{}, errors.New("unused")
}
func (r repositoryStub) ListMaterializations(context.Context, int64, int64) ([]domain.MaterializationView, error) {
	return nil, errors.New("unused")
}
func (r repositoryStub) RetryMaterialization(context.Context, int64, int64, int64, string) error {
	return errors.New("unused")
}
func (r repositoryStub) ClaimMaterialization(ctx context.Context, authority MachineAuthority, runID int64) (domain.MaterializationClaim, error) {
	return r.claimMaterialization(ctx, authority, runID)
}
func (r repositoryStub) FreezeMaterialization(context.Context, MachineAuthority, int64, string, domain.MaterializationFreeze) error {
	return errors.New("unused")
}
func (r repositoryStub) FailMaterialization(context.Context, MachineAuthority, int64, string, string) error {
	return errors.New("unused")
}
func (r repositoryStub) AuthorizeCheckpointKey(context.Context, MachineAuthority, string) error {
	return errors.New("unused")
}
func (r repositoryStub) ResolveRestoreKeyEnvelope(context.Context, MachineAuthority, int64) (KeyEnvelope, string, error) {
	return KeyEnvelope{}, "", errors.New("unused")
}

type keyWrapperStub struct{ key []byte }

func (k keyWrapperStub) Wrap(context.Context, []byte) (KeyEnvelope, error) {
	return KeyEnvelope{}, errors.New("unused")
}
func (k keyWrapperStub) Unwrap(context.Context, KeyEnvelope) ([]byte, error) {
	return append([]byte(nil), k.key...), nil
}

func TestCreateMaterializationValidatesKindAndOutputIdentity(t *testing.T) {
	t.Parallel()
	called := 0
	service := New(repositoryStub{createMaterialization: func(_ context.Context, request CreateMaterializationRequest) (domain.MaterializationView, bool, error) {
		called++
		return domain.MaterializationView{ID: 456038378, Kind: request.Kind}, false, nil
	}})
	base := CreateMaterializationRequest{TaskID: 2610240832, TaskTurnID: 637649102, TenantID: 1001, Actor: 10001, IdempotencyKey: "materialize:1"}

	for name, mutate := range map[string]func(*CreateMaterializationRequest){
		"unknown kind":                    func(request *CreateMaterializationRequest) { request.Kind = "ZIP" },
		"declared output needs output id": func(request *CreateMaterializationRequest) { request.Kind = domain.MaterializationDeclaredOutput },
		"patch forbids output id": func(request *CreateMaterializationRequest) {
			request.Kind, request.OutputID = domain.MaterializationPatch, 1002
		},
	} {
		t.Run(name, func(t *testing.T) {
			request := base
			mutate(&request)
			if _, _, err := service.CreateMaterialization(context.Background(), request); !errors.Is(err, domain.ErrMaterializationInvalid) {
				t.Fatalf("error=%v", err)
			}
		})
	}

	request := base
	request.Kind, request.OutputID = domain.MaterializationDeclaredOutput, 1002
	if _, _, err := service.CreateMaterialization(context.Background(), request); err != nil {
		t.Fatal(err)
	}
	request = base
	request.Kind = domain.MaterializationBundle
	if _, _, err := service.CreateMaterialization(context.Background(), request); err != nil {
		t.Fatal(err)
	}
	if called != 2 {
		t.Fatalf("repository called %d times, want 2", called)
	}
}

func TestClaimMaterializationUnwrapsKeyAndRemovesEnvelope(t *testing.T) {
	t.Parallel()
	envelopeBody, err := json.Marshal(KeyEnvelope{Ciphertext: "ciphertext", EDEK: "edek", KeyID: "key_1"})
	if err != nil {
		t.Fatal(err)
	}
	service := New(repositoryStub{claimMaterialization: func(context.Context, MachineAuthority, int64) (domain.MaterializationClaim, error) {
		return domain.MaterializationClaim{RunID: 456038378, Restore: domain.RestoreContract{WrappedDataKey: string(envelopeBody)}}, nil
	}})
	key := make([]byte, 32)
	for i := range key {
		key[i] = byte(i + 1)
	}
	service.SetKeyWrapper(keyWrapperStub{key: key})

	claim, err := service.ClaimMaterialization(context.Background(), MachineAuthority{}, 1003)
	if err != nil {
		t.Fatal(err)
	}
	if claim.Key != base64.StdEncoding.EncodeToString(key) {
		t.Fatal("claim did not contain the expected cycle-scoped plaintext key")
	}
	if claim.Restore.WrappedDataKey != "" {
		t.Fatal("wrapped key escaped into Worker claim after unwrapping")
	}
}
