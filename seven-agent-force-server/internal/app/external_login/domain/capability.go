package domain

const (
	CapabilityOAuthLogin   = "OAUTH_LOGIN"
	CapabilityOIDCLogin    = "OIDC_LOGIN"
	CapabilityProfile      = "PROFILE"
	CapabilityEmail        = "EMAIL"
	CapabilityTokenRefresh = "TOKEN_REFRESH"
	CapabilityTokenRevoke  = "TOKEN_REVOKE"
	CapabilityAPIToken     = "API_TOKEN"
)

func BuiltInProviderCapabilities() map[string]ProviderCapability {
	return map[string]ProviderCapability{
		"github": {
			ProviderCode: "github",
			DisplayName:  "GitHub",
			ProtocolType: ProtocolTypeOAuth2,
			Capabilities: []string{
				CapabilityOAuthLogin,
				CapabilityProfile,
				CapabilityEmail,
				CapabilityAPIToken,
			},
			DefaultScopes: []string{"read:user", "user:email"},
		},
		"google": {
			ProviderCode: "google",
			DisplayName:  "Google",
			ProtocolType: ProtocolTypeOIDC,
			Capabilities: []string{
				CapabilityOIDCLogin,
				CapabilityProfile,
				CapabilityEmail,
				CapabilityTokenRefresh,
				CapabilityAPIToken,
			},
			DefaultScopes: []string{"openid", "email", "profile"},
		},
		"oidc": {
			ProviderCode: "oidc",
			DisplayName:  "Generic OIDC",
			ProtocolType: ProtocolTypeOIDC,
			Capabilities: []string{
				CapabilityOIDCLogin,
				CapabilityProfile,
				CapabilityEmail,
				CapabilityTokenRefresh,
			},
			DefaultScopes: []string{"openid", "email", "profile"},
		},
	}
}
