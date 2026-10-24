package facade

import (
	"reflect"
	"testing"
)

func TestAssetBindingFacadeExposesOnlyServerOwnedAuthorityFields(t *testing.T) {
	commandType := reflect.TypeOf(BindUploadedFileCommand{})
	for _, forbidden := range []string{
		"UserID", "TenantID", "ScopeID", "BizID", "BizType",
		"AccessScope", "AccessLevel", "VisitStrategy", "VisitURL",
	} {
		if _, ok := commandType.FieldByName(forbidden); ok {
			t.Fatalf("asset binding command exposes caller-owned authority field %q", forbidden)
		}
	}
	configCommandType := reflect.TypeOf(BindConfigAssetCommand{})
	for _, forbidden := range []string{
		"UserID", "TenantID", "ScopeID", "ReferenceID", "BizType",
		"AccessScope", "AccessLevel", "VisitStrategy", "VisitURL", "DisplayName",
	} {
		if _, ok := configCommandType.FieldByName(forbidden); ok {
			t.Fatalf("config asset command exposes caller-owned authority field %q", forbidden)
		}
	}
	facadesType := reflect.TypeOf(Facades{})
	if facadesType.NumField() != 3 || facadesType.Field(0).Name != "Assets" || facadesType.Field(1).Name != "ConfigAssets" || facadesType.Field(2).Name != "WorkforceInputs" {
		t.Fatalf("file facade registry re-exposed a legacy authority surface: %v", facadesType)
	}
}
