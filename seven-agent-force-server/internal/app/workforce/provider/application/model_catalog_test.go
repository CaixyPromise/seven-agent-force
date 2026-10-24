package application

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"testing"

	"github.com/CaixyPromise/seven-agent/force/internal/app/workforce/provider/domain"
	providerfacade "github.com/CaixyPromise/seven-agent/force/internal/app/workforce/provider/facade"
)

type forbiddenCatalogDiscovery struct{}

func (forbiddenCatalogDiscovery) Discover(context.Context, domain.RuntimeCredential) ([]string, error) {
	panic("selection catalog must not invoke upstream discovery")
}

type unavailableSelectionRepository struct {
	*repositoryFake
	err error
}

func (r unavailableSelectionRepository) GetTenant(context.Context, int64, int64) (domain.Credential, error) {
	return domain.Credential{}, r.err
}

func TestTaskModelSelectionClassificationIsClosedAndPreservesCauses(t *testing.T) {
	repository := &repositoryFake{items: map[int64]domain.Credential{
		7: {ID: 7, TenantID: 10, Models: []string{"primary"}, Status: domain.StatusActive},
		8: {ID: 8, TenantID: 10, Models: []string{"primary"}, Status: domain.StatusDisabled},
	}}
	service := NewService(repository, nil, nil)
	for _, tc := range []struct {
		name  string
		id    int64
		model string
		cause error
	}{
		{"missing selection", 0, "primary", ErrModelSelection},
		{"empty model", 7, "", ErrModelSelection},
		{"unknown model", 7, "other", ErrModelNotInCatalog},
		{"disabled", 8, "primary", ErrDisabled},
		{"missing credential", 9, "primary", ErrNotFound},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, err := service.ValidateTaskSelection(context.Background(), 10, tc.id, tc.model)
			wrapped := fmt.Errorf("submission: %w", err)
			if !errors.Is(wrapped, providerfacade.ErrInvalidTaskModelSelection) || !errors.Is(wrapped, tc.cause) {
				t.Fatalf("selection classification or original cause lost: %v", err)
			}
		})
	}
	internal := errors.New("TASK_MODEL_NOT_IN_CATALOG: synthetic database outage")
	_, err := NewService(unavailableSelectionRepository{repository, internal}, nil, nil).ValidateTaskSelection(context.Background(), 10, 7, "primary")
	if !errors.Is(err, internal) || errors.Is(err, providerfacade.ErrInvalidTaskModelSelection) {
		t.Fatalf("internal failure downgraded: %v", err)
	}
	if _, err = service.ValidateTaskSelectionInTransaction(context.Background(), nil, 10, 7, "primary"); err == nil || errors.Is(err, providerfacade.ErrInvalidTaskModelSelection) {
		t.Fatalf("missing transaction is not caller input: %v", err)
	}
}

func TestTaskModelCatalogIsTenantScopedActiveAndSecretFree(t *testing.T) {
	repository := &repositoryFake{items: map[int64]domain.Credential{
		7: {ID: 7, TenantID: 10, Name: "configured", Protocol: domain.ProtocolOpenAI, Models: []string{"primary"}, Status: domain.StatusActive, BaseURL: "https://not-returned.invalid", SecretHint: "not-returned", SecretCiphertext: "synthetic-not-returned"},
		8: {ID: 8, TenantID: 10, Models: []string{"disabled"}, Status: domain.StatusDisabled},
		9: {ID: 9, TenantID: 11, Models: []string{"other-tenant"}, Status: domain.StatusActive},
	}}
	// No secret service is needed; discovery must never be invoked.
	var catalog providerfacade.TaskModelCatalog = NewService(repository, nil, forbiddenCatalogDiscovery{})
	options, err := catalog.ListTaskModelOptions(context.Background(), 10)
	if err != nil || len(options) != 1 || options[0].CredentialID != 7 || options[0].Model != "primary" {
		t.Fatalf("catalog = %#v, error = %v", options, err)
	}
	encoded, err := json.Marshal(options[0])
	if err != nil {
		t.Fatal(err)
	}
	var fields map[string]any
	if err = json.Unmarshal(encoded, &fields); err != nil {
		t.Fatal(err)
	}
	want := map[string]any{"credentialId": "7", "credentialName": "configured", "protocol": domain.ProtocolOpenAI, "model": "primary", "label": "primary · configured"}
	if !reflect.DeepEqual(fields, want) {
		t.Fatalf("selection projection fields = %#v", fields)
	}
	if _, err = catalog.ListTaskModelOptions(context.Background(), 0); !errors.Is(err, ErrNotFound) {
		t.Fatalf("invalid tenant error = %v", err)
	}
}
