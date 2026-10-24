package facade

import "testing"

func TestOperationTypeOptionsExposeStableCodesAndServerLabels(t *testing.T) {
	options := OperationTypeOptions()
	if len(options) != len(OperationTypes()) {
		t.Fatalf("unexpected option count: got=%d want=%d", len(options), len(OperationTypes()))
	}

	labels := make(map[string]string, len(options))
	for _, option := range options {
		if option.Value == "" {
			t.Fatal("operation type option must include a stable value")
		}
		if option.Label == "" {
			t.Fatalf("operation type %q must include a server label", option.Value)
		}
		if option.Label == option.Value {
			t.Fatalf("operation type %q must not fall back to its code as a catalog label", option.Value)
		}
		if _, exists := labels[option.Value]; exists {
			t.Fatalf("duplicate operation type option: %q", option.Value)
		}
		labels[option.Value] = option.Label
	}

	for code, wantLabel := range map[string]string{
		"USER_LOGIN":    "用户登录",
		"CONFIG_UPDATE": "更新配置",
	} {
		if got := labels[code]; got != wantLabel {
			t.Errorf("label for %q: got=%q want=%q", code, got, wantLabel)
		}
	}
}

func TestOperationTypeDescriptionFallsBackToHistoricalCode(t *testing.T) {
	if got := OperationTypeEnum("LEGACY_ACTION").Description(); got != "LEGACY_ACTION" {
		t.Fatalf("unexpected fallback description: got=%q", got)
	}
}

func TestOperationTypeDisplayLabelUsesSpecificDescriptionForOther(t *testing.T) {
	if got := OperationTypeOther.DisplayLabel("分页查询角色"); got != "分页查询角色" {
		t.Fatalf("unexpected display label for OTHER: got=%q want=%q", got, "分页查询角色")
	}
	if got := OperationTypeOther.DisplayLabel("  "); got != "其他" {
		t.Fatalf("empty OTHER description must keep the stable fallback: got=%q want=%q", got, "其他")
	}
	if got := OperationTypeUserLogin.DisplayLabel("密码登录"); got != "用户登录" {
		t.Fatalf("known operation type must keep its canonical label: got=%q want=%q", got, "用户登录")
	}
}
