package infrastructure

import (
	"context"
	"encoding/json"

	projectapp "github.com/CaixyPromise/seven-agent/force/internal/app/workforce/project/application"
	"github.com/CaixyPromise/seven-agent/force/internal/app/workforce/project/domain"
	secretvalue "github.com/CaixyPromise/seven-agent/force/internal/infrastructure/security/crypto/secretvalue"
)

type RepositoryCredentialCipher struct{ Secrets secretvalue.Service }

func (c RepositoryCredentialCipher) EncryptRepositoryCredential(ctx context.Context, credential domain.RepositoryCredential) (projectapp.CredentialEnvelope, error) {
	body, err := json.Marshal(credential)
	if err != nil {
		return projectapp.CredentialEnvelope{}, err
	}
	value, err := c.Secrets.EncryptBytes(ctx, body)
	for i := range body {
		body[i] = 0
	}
	if err != nil {
		return projectapp.CredentialEnvelope{}, err
	}
	return projectapp.CredentialEnvelope{Ciphertext: value.CiphertextB64, EDEK: value.EDEKB64, WrapKeyRef: value.WrapKeyRef}, nil
}

func (c RepositoryCredentialCipher) DecryptRepositoryCredential(ctx context.Context, ciphertext, edek, wrapKeyRef string) (domain.RepositoryCredential, error) {
	body, err := c.Secrets.DecryptBytes(ctx, secretvalue.SecretValue{CiphertextB64: ciphertext, EDEKB64: edek, WrapKeyRef: wrapKeyRef})
	if err != nil {
		return domain.RepositoryCredential{}, err
	}
	defer func() {
		for i := range body {
			body[i] = 0
		}
	}()
	var credential domain.RepositoryCredential
	if err = json.Unmarshal(body, &credential); err != nil {
		return domain.RepositoryCredential{}, err
	}
	return credential, nil
}

var _ projectapp.CredentialCipher = RepositoryCredentialCipher{}
