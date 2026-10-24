package domain

import "time"

type InteractionSnapshot struct {
	LoginTransactionID         string
	LoginContextID             string
	PlatformCode               string
	UserAccount                string
	UserID                     int64
	PrimaryChallengeFlowNonce  string
	PrimaryChallengeIdentifier string
	PrimaryChallengeSatisfied  bool
	RegisterAccount            string
	RegisterEmail              string
	RegisterCaptchaFlowNonce   string
	RegisterCaptchaIdentifier  string
	RegisterCaptchaSatisfied   bool
	RegisterEmailFlowNonce     string
	RegisterEmailIdentifier    string
	RegisterEmailSatisfied     bool
	FlowNonce                  string
	ChallengeIdentifier        string
	CreatedAt                  *time.Time
	ExpiresAt                  *time.Time
}
