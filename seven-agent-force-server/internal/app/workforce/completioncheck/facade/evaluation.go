package facade

import (
	"github.com/CaixyPromise/seven-agent/force/internal/app/workforce/completioncheck/domain"
	processor "github.com/CaixyPromise/seven-agent/force/internal/app/workforce/resultprocessor/facade"
)

type ComparisonContext = domain.ComparisonContext

// GateEvaluation consumes Server-loaded evidence, not Processor/Agent claims.
// Apply before the existing no-progress/cycle-limit and continuation policies.
// It never overrides a mandatory processor rejection or asks an Agent to repair
// historical failures whose attribution has not been independently established.
func GateEvaluation(contract processor.ResultContract, baseline, current domain.Observation, c domain.ComparisonContext) processor.ResultContract {
	if contract.Decision != "PASS" && contract.Decision != "REVISE" {
		return contract
	}
	comparison := domain.Compare(baseline, current, c)
	switch comparison.Disposition {
	case domain.Proceed, domain.NoCheck, domain.ProceedWithLimitations:
		return contract
	case domain.Repair:
		contract.Decision = "REVISE"
		contract.Message = "完成检查出现新增失败，请修正本轮引入的问题。"
	case domain.Blocked:
		contract.Decision = "REJECT"
		contract.Message = "必要检查尚未通过。"
	default:
		contract.Decision = "WAIT_USER"
		contract.Message = "完成检查证据不足或无法确认失败归因，请确认后续处理方式。"
	}
	contract.ReasonCode = comparison.ReasonCode
	contract.Actions = nil
	contract.Outputs = nil
	return contract
}
