package facade

import (
	"encoding/json"
	"github.com/CaixyPromise/seven-agent/force/internal/app/workforce/interaction/domain"
	"strings"
	"testing"
	"unicode/utf8"
)

func TestSDKDisplayProjection(t *testing.T) {
	for _, v := range []struct{ method, body, want string }{{"plan", `{"roundKey":"private","steps":["读取项目","验证结果"]}`, "读取项目\n验证结果"}, {"activity", `{"activityKey":"private","title":"检查测试","status":"STARTED"}`, "检查测试（进行中）"}, {"activity", `{"summary":"运行测试","status":"SUCCEEDED"}`, "运行测试（已完成）"}, {"execution.suspended", `{"sessionReference":"private"}`, "执行已暂停，正在保存进度"}} {
		_, text, e := ProjectAdapterEvent(v.method, json.RawMessage(v.body))
		if e != nil || !strings.Contains(text, v.want) || strings.Contains(text, "private") {
			t.Fatal(text, e)
		}
	}
	raw, _ := json.Marshal(map[string]any{"steps": []string{strings.Repeat("界", 10000)}})
	_, text, e := ProjectAdapterEvent("plan", raw)
	if e != nil || len(text) > domain.MaxAnswerBytes || !utf8.ValidString(text) {
		t.Fatal("unbounded projection", e)
	}
}
