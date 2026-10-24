package domain

import "github.com/CaixyPromise/seven-agent/force/internal/shared/config"

type RiskPolicy struct {
	captchaThreshold  int
	totpThreshold     int
	lockThreshold     int
	lockDurationHours int
}

func NewRiskPolicy(cfg config.LoginConfig) *RiskPolicy {
	return &RiskPolicy{
		captchaThreshold:  cfg.CaptchaThreshold,
		totpThreshold:     cfg.TOTPThreshold,
		lockThreshold:     cfg.LockThreshold,
		lockDurationHours: cfg.LockDurationHours,
	}
}

func (p *RiskPolicy) RequiresCaptcha(consecutiveFailureCount int) bool {
	return consecutiveFailureCount >= p.captchaThreshold
}

func (p *RiskPolicy) RequiresTotp(consecutiveFailureCount int) bool {
	return consecutiveFailureCount >= p.totpThreshold
}

func (p *RiskPolicy) ShouldLock(consecutiveFailureCount int) bool {
	return consecutiveFailureCount >= p.lockThreshold
}

func (p *RiskPolicy) LockDurationHours() int {
	return p.lockDurationHours
}

func (p *RiskPolicy) LockThreshold() int {
	return p.lockThreshold
}
