package cachepolicy

import "testing"

func TestCatalogAdmitsOnlyClassifiedConfigAndDictionaryReads(t *testing.T) {
	configRead, ok := ConfigReadRequest("SEVEN_FRONTEND_METADATA.title", "org:1", "anonymous")
	if !ok {
		t.Fatal("expected explicitly catalogued public config read to be eligible")
	}
	if !ValidateLoaded(configRead, "PUBLIC", "NORMAL", 1, true) {
		t.Fatal("expected matching public NORMAL config row to remain eligible")
	}
	if ValidateLoaded(configRead, "PUBLIC", "SECRET", 1, true) {
		t.Fatal("secret config must never be admitted to DG5")
	}
	if ValidateLoaded(configRead, "AUTHENTICATED", "NORMAL", 1, true) {
		t.Fatal("exposure mismatch must not reuse the public catalog entry")
	}
	for _, key := range []string{
		"SEVEN_FRONTEND_METADATA.loginLogo",
		"SEVEN_FRONTEND_METADATA.favicon",
	} {
		assetRead, ok := ConfigReadRequest(key, "org:1", "anonymous")
		if !ok || !ValidateLoaded(assetRead, "PUBLIC", "NORMAL", 1, true) {
			t.Fatalf("expected catalogued public config asset read for %s", key)
		}
	}
	if _, ok := ConfigReadRequest("secure.otp", "org:1", "anonymous"); ok {
		t.Fatal("uncatalogued config must remain database-authoritative")
	}

	dictRead, ok := DictReadRequest("gender", "org:1", "anonymous")
	if !ok {
		t.Fatal("expected explicitly catalogued dictionary read to be eligible")
	}
	if !ValidateLoaded(dictRead, "PUBLIC", "NORMAL", 1, true) {
		t.Fatal("expected matching public NORMAL dictionary row to remain eligible")
	}
	if _, ok := DictReadRequest("admin_roles", "org:1", "anonymous"); ok {
		t.Fatal("uncatalogued dictionary must remain database-authoritative")
	}
}

func TestReadKeyMaterialSeparatesScopeIdentityAndSchema(t *testing.T) {
	base, ok := ConfigReadRequest("SEVEN_FRONTEND_METADATA.themePrimaryColor", "org:1", "anonymous")
	if !ok {
		t.Fatal("expected catalogued config read")
	}
	otherScope, ok := ConfigReadRequest("SEVEN_FRONTEND_METADATA.themePrimaryColor", "org:2", "anonymous")
	if !ok {
		t.Fatal("expected catalogued config read in second scope")
	}
	otherIdentity, ok := ConfigReadRequest("SEVEN_FRONTEND_METADATA.themePrimaryColor", "org:1", "account:7:authz:4")
	if !ok {
		t.Fatal("expected catalogued config read for authenticated identity")
	}
	if base.KeyMaterial() == otherScope.KeyMaterial() {
		t.Fatal("scope must isolate cache key material")
	}
	if base.KeyMaterial() == otherIdentity.KeyMaterial() {
		t.Fatal("business identity must isolate cache key material")
	}
	if base.SchemaVersion != 1 || base.MaxStale != 0 {
		t.Fatalf("unexpected frozen catalog invariants: schema=%d maxStale=%s", base.SchemaVersion, base.MaxStale)
	}
}

func TestReadRequestRejectsForgedUncataloguedTarget(t *testing.T) {
	request, ok := ConfigReadRequest("SEVEN_FRONTEND_METADATA.title", "org:1", "anonymous")
	if !ok {
		t.Fatal("expected base request")
	}
	request.Target = "SEVEN_FRONTEND_METADATA.unreviewedSecret"
	if request.Valid() {
		t.Fatal("forged target must not inherit a catalogued config class")
	}
	if ValidateLoaded(request, "PUBLIC", "NORMAL", 1, true) {
		t.Fatal("forged target must never be admitted to L1/L2")
	}
}

func TestAuthorizationReadRequestsRequireOpaqueFeatureFingerprintAndSeparateUsers(t *testing.T) {
	fingerprint := "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"
	first, ok := AuthorizationContextReadRequest(7, fingerprint)
	if !ok || !first.Valid() {
		t.Fatal("expected catalogued authorization context request")
	}
	second, ok := AuthorizationContextReadRequest(8, fingerprint)
	if !ok || first.KeyMaterial() == second.KeyMaterial() {
		t.Fatal("authorization snapshot must isolate user identity")
	}
	if _, ok := AuthorizationMenuReadRequest(7, "not-a-feature-fingerprint"); ok {
		t.Fatal("authorization snapshot must reject an unversioned feature set")
	}
	first.Target = "user:7:features:invalid"
	if first.Valid() {
		t.Fatal("forged authorization target must not inherit catalog admission")
	}
}
