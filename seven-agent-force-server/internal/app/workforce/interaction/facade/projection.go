package facade

import (
	"encoding/json"
	"github.com/CaixyPromise/seven-agent/force/internal/app/workforce/interaction/domain"
	"strings"
	"unicode/utf8"
)

// ProjectAdapterEvent reads only the public display fields of a validated
// protocol frame. Runtime keys and session material are deliberately omitted.
// Bounded truncation accommodates the SDK's larger activity envelope without
// converting a verbose display observation into a fatal Worker error.
func ProjectAdapterEvent(method string, body json.RawMessage) (kind, text string, err error) {
	if method == "execution.suspended" {
		return "ACTIVITY", "执行已暂停，正在保存进度", nil
	}
	var p struct {
		Text    string   `json:"text"`
		Steps   []string `json:"steps"`
		Title   string   `json:"title"`
		Summary string   `json:"summary"`
		Status  string   `json:"status"`
	}
	if json.Unmarshal(body, &p) != nil {
		return "", "", domain.ErrInvalid
	}
	kind = "ACTIVITY"
	switch method {
	case "message":
		kind = "ASSISTANT_MESSAGE"
		text = p.Text
	case "plan":
		steps := []string{}
		for _, step := range p.Steps {
			step = strings.TrimSpace(step)
			if step != "" {
				steps = append(steps, step)
			}
		}
		if len(steps) == 0 {
			text = "计划已更新"
		} else {
			text = "计划：\n" + strings.Join(steps, "\n")
		}
	case "activity":
		text = strings.TrimSpace(p.Summary)
		if text == "" {
			text = strings.TrimSpace(p.Title)
		}
		if text == "" {
			text = "执行活动"
		}
		status := map[string]string{"STARTED": "进行中", "COMPLETED": "已完成", "SUCCEEDED": "已完成", "FAILED": "失败", "CANCELLED": "已取消", "UNKNOWN": "状态未知"}[p.Status]
		if status != "" {
			text += "（" + status + "）"
		}
	default:
		return "", "", domain.ErrInvalid
	}
	if !utf8.ValidString(text) || strings.ContainsRune(text, 0) {
		return "", "", domain.ErrInvalid
	}
	text = strings.TrimSpace(text)
	if text == "" {
		text = "消息已更新"
	}
	if len(text) > domain.MaxAnswerBytes {
		end := domain.MaxAnswerBytes - len("…")
		for end > 0 && !utf8.RuneStart(text[end]) {
			end--
		}
		text = text[:end] + "…"
	}
	return kind, text, nil
}
