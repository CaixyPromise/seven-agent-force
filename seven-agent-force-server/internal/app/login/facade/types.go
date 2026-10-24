package facade

import challengefacade "github.com/CaixyPromise/seven-agent/force/internal/app/challenge/facade"

type RequestContext struct {
	LoginIP   string `json:"loginIp,omitempty"`
	UserAgent string `json:"userAgent,omitempty"`
	DeviceID  string `json:"deviceId,omitempty"`
	TenantID  string `json:"tenantId,omitempty"`
	TraceID   string `json:"traceId,omitempty"`
	Host      string `json:"-"`
	Origin    string `json:"-"`
	Referer   string `json:"-"`
}

type Captcha struct {
	ChallengeIdentifier string `json:"challengeIdentifier"`
	StepIdentifier      string `json:"stepIdentifier"`
	ImageBase64         string `json:"imageBase64,omitempty"`
}

type PasswordStateRequest struct {
	LoginTransactionID string          `json:"loginTransactionId" validate:"required"`
	LoginContextID     string          `json:"loginContextId,omitempty"`
	UserAccount        string          `json:"userAccount" validate:"required"`
	RefreshCaptcha     bool            `json:"refreshCaptcha,omitempty"`
	RequestContext     *RequestContext `json:"requestContext,omitempty"`
}

type PasswordSubmitRequest struct {
	LoginTransactionID string          `json:"loginTransactionId" validate:"required"`
	LoginContextID     string          `json:"loginContextId,omitempty"`
	UserAccount        string          `json:"userAccount" validate:"required"`
	Password           string          `json:"password" validate:"required"`
	CaptchaCode        string          `json:"captchaCode,omitempty"`
	RequestContext     *RequestContext `json:"requestContext,omitempty"`
}

type RegisterStateRequest struct {
	LoginTransactionID string          `json:"loginTransactionId" validate:"required"`
	LoginContextID     string          `json:"loginContextId" validate:"required"`
	UserAccount        string          `json:"userAccount" validate:"required"`
	RequestContext     *RequestContext `json:"requestContext,omitempty"`
}

type RegisterSubmitRequest struct {
	LoginTransactionID string          `json:"loginTransactionId" validate:"required"`
	LoginContextID     string          `json:"loginContextId" validate:"required"`
	UserAccount        string          `json:"userAccount" validate:"required"`
	UserName           string          `json:"userName" validate:"required"`
	UserEmail          string          `json:"userEmail" validate:"required"`
	Password           string          `json:"password" validate:"required"`
	ConfirmPassword    string          `json:"confirmPassword" validate:"required"`
	EmailCode          string          `json:"emailCode" validate:"required"`
	RequestContext     *RequestContext `json:"requestContext,omitempty"`
}

type RegisterEmailCodeRequest struct {
	LoginTransactionID string          `json:"loginTransactionId" validate:"required"`
	LoginContextID     string          `json:"loginContextId" validate:"required"`
	UserAccount        string          `json:"userAccount" validate:"required"`
	UserEmail          string          `json:"userEmail" validate:"required"`
	CaptchaCode        string          `json:"captchaCode" validate:"required"`
	RequestContext     *RequestContext `json:"requestContext,omitempty"`
}

type TotpVerifyRequest struct {
	LoginTransactionID string          `json:"loginTransactionId" validate:"required"`
	LoginContextID     string          `json:"loginContextId,omitempty"`
	UserAccount        string          `json:"userAccount" validate:"required"`
	OTPCode            string          `json:"otpCode" validate:"required"`
	RequestContext     *RequestContext `json:"requestContext,omitempty"`
}

type PasskeyStartRequest struct {
	LoginTransactionID string          `json:"loginTransactionId" validate:"required"`
	LoginContextID     string          `json:"loginContextId,omitempty"`
	UserAccount        string          `json:"userAccount" validate:"required"`
	RequestContext     *RequestContext `json:"requestContext,omitempty"`
}

type PasskeyVerifyRequest struct {
	LoginTransactionID   string          `json:"loginTransactionId" validate:"required"`
	LoginContextID       string          `json:"loginContextId,omitempty"`
	UserAccount          string          `json:"userAccount" validate:"required"`
	CredentialIdentifier string          `json:"credentialIdentifier" validate:"required"`
	ClientDataJSON       string          `json:"clientDataJSON" validate:"required"`
	AuthenticatorData    string          `json:"authenticatorData" validate:"required"`
	Signature            string          `json:"signature" validate:"required"`
	RequestContext       *RequestContext `json:"requestContext,omitempty"`
}

type PasswordState struct {
	CanPasswordLogin bool     `json:"canPasswordLogin"`
	CaptchaRequired  bool     `json:"captchaRequired"`
	TotpRequired     bool     `json:"totpRequired"`
	Locked           bool     `json:"locked"`
	LockExpiresAt    *int64   `json:"lockExpiresAt,omitempty"`
	UnlockMethod     string   `json:"unlockMethod,omitempty"`
	Captcha          *Captcha `json:"captcha,omitempty"`
}

type RegisterState struct {
	CanRegister bool     `json:"canRegister"`
	Captcha     *Captcha `json:"captcha,omitempty"`
}

type RegisterSubmitResult struct {
	Registered  bool     `json:"registered"`
	UserID      int64    `json:"userId,omitempty"`
	UserAccount string   `json:"userAccount,omitempty"`
	Message     string   `json:"message"`
	Captcha     *Captcha `json:"captcha,omitempty"`
}

type RegisterEmailCodeResult struct {
	Sent             bool     `json:"sent"`
	EmailMasked      string   `json:"emailMasked,omitempty"`
	CooldownSeconds  int      `json:"cooldownSeconds"`
	ExpiresInSeconds int      `json:"expiresInSeconds"`
	Message          string   `json:"message"`
	Captcha          *Captcha `json:"captcha,omitempty"`
}

type PasswordSubmitResult struct {
	Authenticated            bool     `json:"authenticated"`
	RedirectURL              string   `json:"redirectUrl,omitempty"`
	AccessToken              string   `json:"accessToken,omitempty"`
	TokenType                string   `json:"tokenType,omitempty"`
	AccessTTLSeconds         int64    `json:"accessTtlSec,omitempty"`
	SessionCookieHeaderValue string   `json:"sessionCookieHeaderValue,omitempty"`
	RefreshCookieHeaderValue string   `json:"refreshCookieHeaderValue,omitempty"`
	CanPasswordLogin         bool     `json:"canPasswordLogin"`
	CaptchaRequired          bool     `json:"captchaRequired"`
	TotpRequired             bool     `json:"totpRequired"`
	Locked                   bool     `json:"locked"`
	LockExpiresAt            *int64   `json:"lockExpiresAt,omitempty"`
	UnlockMethod             string   `json:"unlockMethod,omitempty"`
	Captcha                  *Captcha `json:"captcha,omitempty"`
	CaptchaRejected          bool     `json:"-"`
}

type TotpVerifyResult struct {
	Authenticated            bool   `json:"authenticated"`
	RedirectURL              string `json:"redirectUrl,omitempty"`
	AccessToken              string `json:"accessToken,omitempty"`
	TokenType                string `json:"tokenType,omitempty"`
	AccessTTLSeconds         int64  `json:"accessTtlSec,omitempty"`
	SessionCookieHeaderValue string `json:"sessionCookieHeaderValue,omitempty"`
	RefreshCookieHeaderValue string `json:"refreshCookieHeaderValue,omitempty"`
	Locked                   bool   `json:"locked"`
	LockExpiresAt            *int64 `json:"lockExpiresAt,omitempty"`
}

type PasskeyStartResult struct {
	ChallengeIdentifier string                                  `json:"challengeIdentifier"`
	StepIdentifier      string                                  `json:"stepIdentifier"`
	UserInterfaceHints  map[string]any                          `json:"userInterfaceHints,omitempty"`
	Challenge           *challengefacade.StartChallengeResponse `json:"challenge,omitempty"`
}

type PasskeyVerifyResult struct {
	Authenticated            bool   `json:"authenticated"`
	RedirectURL              string `json:"redirectUrl,omitempty"`
	AccessToken              string `json:"accessToken,omitempty"`
	TokenType                string `json:"tokenType,omitempty"`
	AccessTTLSeconds         int64  `json:"accessTtlSec,omitempty"`
	SessionCookieHeaderValue string `json:"sessionCookieHeaderValue,omitempty"`
	RefreshCookieHeaderValue string `json:"refreshCookieHeaderValue,omitempty"`
	Locked                   bool   `json:"locked"`
	LockExpiresAt            *int64 `json:"lockExpiresAt,omitempty"`
}
