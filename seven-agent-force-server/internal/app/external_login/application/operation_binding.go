package application

import "fmt"

const (
	StepUpActionExternalLoginProviderCreate       = "EXTERNAL_LOGIN_PROVIDER_CREATE"
	StepUpActionExternalLoginProviderUpdate       = "EXTERNAL_LOGIN_PROVIDER_UPDATE"
	StepUpActionExternalLoginProviderStatusChange = "EXTERNAL_LOGIN_PROVIDER_STATUS_CHANGE"
	StepUpActionExternalLoginProviderSecretRotate = "EXTERNAL_LOGIN_PROVIDER_SECRET_ROTATE"
	StepUpActionExternalLoginIdentityStatusChange = "EXTERNAL_LOGIN_IDENTITY_STATUS_CHANGE"
	StepUpActionExternalOAuthTokenRevoke          = "EXTERNAL_OAUTH_TOKEN_REVOKE"
)

func BuildProviderCreateOperationBinding(providerCode string) string {
	return fmt.Sprintf("external-login:provider:%s|create", providerCode)
}

func BuildProviderUpdateOperationBinding(providerCode string) string {
	return fmt.Sprintf("external-login:provider:%s|update", providerCode)
}

func BuildProviderStatusOperationBinding(providerCode string, status int) string {
	return fmt.Sprintf("external-login:provider:%s|status:%d", providerCode, status)
}

func BuildProviderSecretRotateOperationBinding(providerCode string) string {
	return fmt.Sprintf("external-login:provider:%s|secret-rotate", providerCode)
}

func BuildIdentityStatusOperationBinding(identityID int64, status int) string {
	return fmt.Sprintf("external-login:identity:%d|status:%d", identityID, status)
}

func BuildTokenRevokeOperationBinding(tokenID int64) string {
	return fmt.Sprintf("external-login:token:%d|revoke", tokenID)
}
