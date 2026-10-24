package facade

import "fmt"

// SafeSummary never includes command text, stdout, stderr, paths or filenames.
func SafeSummary(o Observation) string {
	switch o.Status {
	case "PASSED":
		if o.TestCountKnown {
			return fmt.Sprintf("命令检查通过；执行了 %d 项测试。", o.TestCount)
		}
		return "命令检查通过；测试数量未知。"
	case "NO_TESTS":
		return "未发现实际执行的测试。"
	case "TIMED_OUT":
		return "检查执行超时。"
	case "NOT_CONFIGURED":
		return "未配置可执行检查。"
	case "UNAVAILABLE":
		if o.ReasonCode == "CHECK_COMMAND_UNAVAILABLE" {
			return "检查命令不可用或无法启动。"
		}
		return "检查执行环境或验证证据不可用。"
	case "FAILED":
		if o.ExitCode != nil {
			return fmt.Sprintf("检查退出码 %d；失败归因尚未确认。", *o.ExitCode)
		}
		return "检查失败；失败归因尚未确认。"
	default:
		return "检查证据无效。"
	}
}
