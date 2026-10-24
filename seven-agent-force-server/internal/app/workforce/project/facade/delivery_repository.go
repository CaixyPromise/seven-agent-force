package facade

import "context"

// DeliveryRepositoryBinding is server-only immutable ProjectVersion data.
// It is never a browser credential or an arbitrary repository selection.
type DeliveryRepositoryBinding struct {
	ProjectID, VersionID, CredentialVersion                          int64
	Current                                                          bool
	ProviderType, Scheme, Host, RepositoryPath, WebURL, ConfigDigest string
	Port                                                             int
	Ciphertext, EDEK, WrapKeyRef                                     string `json:"-"`
	Push, DraftReview                                                bool
}

type DeliveryRepositoryBindingReader interface {
	DeliveryRepositoryBinding(context.Context, int64, int64, int64) (DeliveryRepositoryBinding, error)
}
