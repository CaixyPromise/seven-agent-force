package drivers

import (
	"net/url"

	"github.com/CaixyPromise/seven-agent/force/internal/app/external_login/domain"
)

const (
	googleIssuer                = "https://accounts.google.com"
	googleAuthorizationEndpoint = "https://accounts.google.com/o/oauth2/v2/auth"
	googleTokenEndpoint         = "https://oauth2.googleapis.com/token"
	googleJWKSURI               = "https://www.googleapis.com/oauth2/v3/certs"
)

type GoogleDriver struct {
	*oidcBase
}

func NewGoogleDriver(options ...OIDCOption) *GoogleDriver {
	return &GoogleDriver{oidcBase: newOIDCBase("google", oidcDefaults{
		issuer:                googleIssuer,
		authorizationEndpoint: googleAuthorizationEndpoint,
		tokenEndpoint:         googleTokenEndpoint,
		jwksURI:               googleJWKSURI,
	}, options...)}
}

func (d *GoogleDriver) Capabilities() domain.ProviderCapability {
	return domain.BuiltInProviderCapabilities()["google"]
}

func (d *GoogleDriver) enrichAuthorizationValues(provider domain.Provider, values url.Values) {
	if metadataBool(provider.MetadataJSON, "enableRefreshTokenStorage") {
		values.Set("access_type", "offline")
	}
}
