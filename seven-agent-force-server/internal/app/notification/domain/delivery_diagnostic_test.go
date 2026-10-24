package domain

import "testing"

func TestDeliveryDiagnosticSecretPermissionMatchesRegisteredCapability(t *testing.T) {
	if got := DeliveryDiagnosticPermission(DeliveryContentTierSecretEphemeral); got != "system:notification:delivery:content:secret-ephemeral" {
		t.Fatalf("secret diagnostic permission=%q", got)
	}
	if got := DeliveryDiagnosticPermission("unknown"); got != "system:notification:delivery:content:sensitive" {
		t.Fatalf("unknown tier permission=%q, want fail-closed sensitive", got)
	}
}

func TestContentTierForTemplateVariablesFailsClosed(t *testing.T) {
	if got := ContentTierForTemplateVariables([]TemplateVariable{{Name: "reference", Classification: TemplateVariableClassificationPublic}}); got != DeliveryContentTierPublic {
		t.Fatalf("all-public template variables tier=%q, want PUBLIC", got)
	}
	for _, variables := range [][]TemplateVariable{
		nil,
		{{Name: "reference", Classification: TemplateVariableClassificationSensitive}},
		{{Name: "reference", Classification: "unexpected"}},
	} {
		if got := ContentTierForTemplateVariables(variables); got != DeliveryContentTierSensitive {
			t.Fatalf("template variables %#v tier=%q, want fail-closed SENSITIVE", variables, got)
		}
	}
}
