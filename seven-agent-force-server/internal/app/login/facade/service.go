package facade

import "context"

type PasswordFlowFacade interface {
	GetPasswordState(ctx context.Context, request PasswordStateRequest) (*PasswordState, error)
	SubmitPassword(ctx context.Context, request PasswordSubmitRequest) (*PasswordSubmitResult, error)
	GetRegisterState(ctx context.Context, request RegisterStateRequest) (*RegisterState, error)
	SendRegisterEmailCode(ctx context.Context, request RegisterEmailCodeRequest) (*RegisterEmailCodeResult, error)
	SubmitRegister(ctx context.Context, request RegisterSubmitRequest) (*RegisterSubmitResult, error)
	StartPasskey(ctx context.Context, request PasskeyStartRequest) (*PasskeyStartResult, error)
	VerifyPasskey(ctx context.Context, request PasskeyVerifyRequest) (*PasskeyVerifyResult, error)
	VerifyTotp(ctx context.Context, request TotpVerifyRequest) (*TotpVerifyResult, error)
}
