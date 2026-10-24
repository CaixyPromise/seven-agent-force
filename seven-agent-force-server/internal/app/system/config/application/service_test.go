package application

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"strconv"
	"strings"
	"testing"
	"time"

	authorizationfacade "github.com/CaixyPromise/seven-agent/force/internal/app/authorization/facade"
	filefacade "github.com/CaixyPromise/seven-agent/force/internal/app/file/facade"
	"github.com/CaixyPromise/seven-agent/force/internal/app/system/config/domain"
	configfacade "github.com/CaixyPromise/seven-agent/force/internal/app/system/config/facade"
	apperrors "github.com/CaixyPromise/seven-agent/force/internal/common/errors"
	"github.com/CaixyPromise/seven-agent/force/internal/shared/cachepolicy"
	"github.com/CaixyPromise/seven-agent/force/internal/shared/stepup"
	"github.com/bytedance/sonic"
)

func TestAddConfigRejectsDuplicateKeyWithinGroup(t *testing.T) {
	repo := &fakeRepository{
		groupsByID: map[int64]*domain.ConfigGroup{
			10: {ID: 10, GroupCode: "basic", GroupName: "Basic", Status: 1},
		},
		configKeyCount: map[string]int64{"10|siteName": 1},
	}
	service := NewService(nil, repo, &fakeCacheStore{}, domain.NewService(), &fakeSecretCipher{}, &fakeRevealCipher{}, &fakeUserLookup{})

	_, err := service.AddConfig(context.Background(), Actor{UserID: 1001, IsAdmin: true}, configfacade.ConfigAddRequest{
		GroupID:     10,
		ConfigKey:   "siteName",
		ConfigValue: "Seven",
		ValueType:   "string",
	})
	appErr := apperrors.From(err)
	if appErr == nil || appErr.Code() != apperrors.CodeParamsError {
		t.Fatalf("expected duplicate params error, got %v", err)
	}
}

func TestOpenConfigAssetUsesTheSameExposurePolicyAsTypedConfigReads(t *testing.T) {
	group := &domain.ConfigGroup{ID: 70, GroupCode: "SEVEN_FRONTEND_METADATA", GroupName: "Frontend", Status: 1}
	item := &domain.Config{
		ID: 71, GroupID: 70, GroupCode: group.GroupCode, GroupName: group.GroupName, ConfigKey: "loginLogo",
		ConfigValue: filefacade.ConfigAssetStablePath(71), ValueType: string(domain.ConfigValueImage), UIWidget: string(domain.ConfigWidgetImageUpload),
		Exposure: string(domain.ConfigExposurePublic), Sensitivity: string(domain.ConfigSensitivityNormal), SchemaVersion: 1, Version: 1, IsEnabled: 1,
	}
	repo := &fakeRepository{
		groupsByID:  map[int64]*domain.ConfigGroup{70: group},
		configsByID: map[int64]*domain.Config{71: item},
	}
	assets := &fakeConfigAssetFacade{openResult: &filefacade.ConfigAssetOpenResult{
		Reader: io.NopCloser(strings.NewReader("png")), Size: 3, ContentType: "image/png", FileName: "brand.png",
		AssetType: filefacade.ConfigAssetImage, AccessScope: filefacade.ConfigAssetPublic,
	}}
	service := NewService(nil, repo, &fakeCacheStore{}, domain.NewService(), &fakeSecretCipher{}, &fakeRevealCipher{}, &fakeUserLookup{})
	service.BindConfigAssets(assets)
	result, err := service.OpenConfigAsset(context.Background(), Actor{}, 71)
	if err != nil || result == nil || result.AssetType != filefacade.ConfigAssetImage || assets.openCalls != 1 {
		t.Fatalf("anonymous public IMAGE read failed: result=%+v calls=%d err=%v", result, assets.openCalls, err)
	}
	_ = result.Reader.Close()

	item.Exposure = string(domain.ConfigExposureAuthenticated)
	assets.openCalls = 0
	if _, err := service.OpenConfigAsset(context.Background(), Actor{}, 71); err == nil || assets.openCalls != 0 {
		t.Fatalf("anonymous AUTHENTICATED asset bypassed typed policy: calls=%d err=%v", assets.openCalls, err)
	}
	assets.openResult = &filefacade.ConfigAssetOpenResult{
		Reader: io.NopCloser(strings.NewReader("png")), Size: 3, ContentType: "image/png", FileName: "brand.png",
		AssetType: filefacade.ConfigAssetImage, AccessScope: filefacade.ConfigAssetAuthenticated,
	}
	result, err = service.OpenConfigAsset(context.Background(), Actor{Authenticated: true, AccountID: 101, ScopeID: "org:7"}, 71)
	if err != nil || result == nil || assets.openCalls != 1 {
		t.Fatalf("authenticated asset read failed: result=%+v calls=%d err=%v", result, assets.openCalls, err)
	}
	_ = result.Reader.Close()

	item.Exposure = string(domain.ConfigExposureInternal)
	assets.openCalls = 0
	if _, err := service.OpenConfigAsset(context.Background(), Actor{Authenticated: true, AccountID: 101, ScopeID: "org:7"}, 71); err == nil || assets.openCalls != 0 {
		t.Fatalf("ordinary authenticated principal bypassed INTERNAL asset policy: calls=%d err=%v", assets.openCalls, err)
	}
}

func TestOpenConfigAssetRejectsNonCanonicalValueAndFacadePolicyMismatch(t *testing.T) {
	group := &domain.ConfigGroup{ID: 80, GroupCode: "SEVEN_FRONTEND_METADATA", GroupName: "Frontend", Status: 1}
	item := &domain.Config{
		ID: 81, GroupID: 80, GroupCode: group.GroupCode, GroupName: group.GroupName, ConfigKey: "favicon",
		ConfigValue: "https://untrusted.example/favicon.png", ValueType: string(domain.ConfigValueImage), UIWidget: string(domain.ConfigWidgetImageUpload),
		Exposure: string(domain.ConfigExposurePublic), Sensitivity: string(domain.ConfigSensitivityNormal), SchemaVersion: 1, Version: 1, IsEnabled: 1,
	}
	repo := &fakeRepository{groupsByID: map[int64]*domain.ConfigGroup{80: group}, configsByID: map[int64]*domain.Config{81: item}}
	assets := &fakeConfigAssetFacade{openResult: &filefacade.ConfigAssetOpenResult{
		Reader: io.NopCloser(strings.NewReader("png")), Size: 3, ContentType: "image/png", FileName: "favicon.png",
		AssetType: filefacade.ConfigAssetImage, AccessScope: filefacade.ConfigAssetPublic,
	}}
	service := NewService(nil, repo, &fakeCacheStore{}, domain.NewService(), &fakeSecretCipher{}, &fakeRevealCipher{}, &fakeUserLookup{})
	service.BindConfigAssets(assets)
	if _, err := service.OpenConfigAsset(context.Background(), Actor{}, 81); err == nil || assets.openCalls != 0 {
		t.Fatalf("raw URL config value reached asset facade: calls=%d err=%v", assets.openCalls, err)
	}

	item.ConfigValue = filefacade.ConfigAssetStablePath(81)
	assets.openResult = &filefacade.ConfigAssetOpenResult{
		Reader: io.NopCloser(strings.NewReader("png")), Size: 3, ContentType: "image/png", FileName: "favicon.png",
		AssetType: filefacade.ConfigAssetImage, AccessScope: filefacade.ConfigAssetAuthenticated,
	}
	if _, err := service.OpenConfigAsset(context.Background(), Actor{}, 81); err == nil {
		t.Fatal("mismatched CONFIG_ASSET reference policy unexpectedly streamed")
	}
}

func TestConfigAssetMutationRejectsAmbiguousClearAndRawValue(t *testing.T) {
	group := &domain.ConfigGroup{ID: 90, GroupCode: "assets", GroupName: "Assets", Status: 1}
	item := &domain.Config{
		ID: 91, GroupID: 90, GroupCode: group.GroupCode, GroupName: group.GroupName, ConfigKey: "logo",
		ConfigValue: filefacade.ConfigAssetStablePath(91), ValueType: string(domain.ConfigValueImage), UIWidget: string(domain.ConfigWidgetImageUpload),
		Exposure: string(domain.ConfigExposurePublic), Sensitivity: string(domain.ConfigSensitivityNormal), SchemaVersion: 1, Version: 1, IsEnabled: 1,
	}
	repo := &fakeRepository{groupsByID: map[int64]*domain.ConfigGroup{90: group}, configsByID: map[int64]*domain.Config{91: item}}
	service := NewService(nil, repo, &fakeCacheStore{}, domain.NewService(), &fakeSecretCipher{}, &fakeRevealCipher{}, &fakeUserLookup{})
	service.BindConfigAssets(&fakeConfigAssetFacade{})
	fileID := int64(99)
	clear := true
	version := int64(1)
	if err := service.UpdateConfig(context.Background(), Actor{UserID: 1, IsAdmin: true}, configfacade.ConfigUpdateRequest{
		ID: 91, Version: &version, AssetFileID: &fileID, ClearAsset: &clear,
	}); err == nil {
		t.Fatal("assetFileId + clearAsset unexpectedly succeeded")
	}
	rawURL := "data:image/png;base64,AAAA"
	if _, err := service.AddConfig(context.Background(), Actor{UserID: 1, IsAdmin: true}, configfacade.ConfigAddRequest{
		GroupID: 90, ConfigKey: "rawLogo", ConfigValue: rawURL, ValueType: string(domain.ConfigValueImage), AssetFileID: &fileID,
	}); err == nil {
		t.Fatal("raw IMAGE URL unexpectedly entered config add path")
	}
}

func TestGetConfigByKeyForClientChecksAccessRules(t *testing.T) {
	repo := &fakeRepository{
		groupsByID: map[int64]*domain.ConfigGroup{
			11: {ID: 11, GroupCode: "secure", GroupName: "Secure", PermissionCode: "perm:a;perm:b", Status: 1},
			12: {ID: 12, GroupCode: "public", GroupName: "Public", Status: 1},
		},
		groupsByCode: map[string]*domain.ConfigGroup{
			"secure": {ID: 11, GroupCode: "secure", GroupName: "Secure", PermissionCode: "perm:a;perm:b", Status: 1},
			"public": {ID: 12, GroupCode: "public", GroupName: "Public", Status: 1},
		},
		configsByID: map[int64]*domain.Config{
			101: {ID: 101, GroupID: 11, GroupCode: "secure", GroupName: "Secure", ConfigKey: "otp", ConfigValue: "123456", ValueType: "string", RequiredLogin: 1, IsEnabled: 1},
			102: {ID: 102, GroupID: 12, GroupCode: "public", GroupName: "Public", ConfigKey: "banner", ConfigValue: "hello", ValueType: "string", IsSystemConfig: 1, IsEnabled: 1},
			104: {ID: 104, GroupID: 12, GroupCode: "public", GroupName: "Public", ConfigKey: "secret", ConfigValue: "plain-secret", ValueType: "string", IsSensitive: 1, IsEnabled: 1},
		},
		configsByGroupKey: map[string]*domain.Config{
			"11|otp":    {ID: 101, GroupID: 11, GroupCode: "secure", GroupName: "Secure", ConfigKey: "otp", ConfigValue: "123456", ValueType: "string", RequiredLogin: 1, IsEnabled: 1},
			"12|banner": {ID: 102, GroupID: 12, GroupCode: "public", GroupName: "Public", ConfigKey: "banner", ConfigValue: "hello", ValueType: "string", IsSystemConfig: 1, IsEnabled: 1},
			"12|secret": {ID: 104, GroupID: 12, GroupCode: "public", GroupName: "Public", ConfigKey: "secret", ConfigValue: "plain-secret", ValueType: "string", IsSensitive: 1, IsEnabled: 1},
		},
	}
	service := NewService(nil, repo, &fakeCacheStore{}, domain.NewService(), &fakeSecretCipher{}, &fakeRevealCipher{}, &fakeUserLookup{})

	_, err := service.GetConfigByKeyForClient(context.Background(), Actor{}, "secure.otp")
	if appErr := apperrors.From(err); appErr == nil || appErr.Code() != apperrors.CodeNotLogin {
		t.Fatalf("expected not-login for requiredLogin config, got %v", err)
	}

	_, err = service.GetConfigByKeyForClient(context.Background(), Actor{Authenticated: true}, "public.banner")
	if appErr := apperrors.From(err); appErr == nil || appErr.Code() != apperrors.CodeForbidden {
		t.Fatalf("expected forbidden for system config, got %v", err)
	}

	_, err = service.GetConfigByKeyForClient(context.Background(), Actor{Authenticated: true}, "public.secret")
	if appErr := apperrors.From(err); appErr == nil || appErr.Code() != apperrors.CodeForbidden {
		t.Fatalf("expected forbidden for sensitive config, got %v", err)
	}

	repo.configsByID[103] = &domain.Config{ID: 103, GroupID: 11, GroupCode: "secure", GroupName: "Secure", ConfigKey: "permOnly", ConfigValue: "ok", ValueType: "STRING", Exposure: "AUTHENTICATED", Sensitivity: "NORMAL", SchemaVersion: 1, Version: 1, IsEnabled: 1}
	repo.configsByGroupKey["11|permOnly"] = &domain.Config{ID: 103, GroupID: 11, GroupCode: "secure", GroupName: "Secure", ConfigKey: "permOnly", ConfigValue: "ok", ValueType: "STRING", Exposure: "AUTHENTICATED", Sensitivity: "NORMAL", SchemaVersion: 1, Version: 1, IsEnabled: 1}
	item, err := service.GetConfigByKeyForClient(context.Background(), Actor{Authenticated: true, AccountID: 1001, ScopeID: "org:1", AuthzVersion: 2, Permissions: []string{"perm:b"}}, "secure.permOnly")
	if err != nil {
		t.Fatalf("expected permission-based access, got %v", err)
	}
	if item == nil || item.Value != "ok" {
		t.Fatalf("unexpected dto: %#v", item)
	}
}

// This is a regression guard for the actual DG5 Sonic cache boundary. A
// ConfigValueDTO has Value any, so caching the public DTO itself would turn an
// INTEGER into float64 on a cache hit. The cache record must preserve the raw
// scalar representation and rehydrate the declared type on every result.
func TestClassifiedConfigCachePreservesIntegerDTOTypeAcrossSonicHits(t *testing.T) {
	tests := []struct {
		name      string
		valueType string
		raw       string
		assert    func(*testing.T, any)
	}{
		{
			name:      "integer",
			valueType: "INTEGER",
			raw:       "9223372036854775807",
			assert: func(t *testing.T, value any) {
				t.Helper()
				if got, ok := value.(int64); !ok || got != int64(9223372036854775807) {
					t.Fatalf("INTEGER type/value changed: value=%#v (%T)", value, value)
				}
			},
		},
		{
			name:      "boolean",
			valueType: "BOOLEAN",
			raw:       "true",
			assert: func(t *testing.T, value any) {
				t.Helper()
				if got, ok := value.(bool); !ok || !got {
					t.Fatalf("BOOLEAN type/value changed: value=%#v (%T)", value, value)
				}
			},
		},
		{
			name:      "json-large-number",
			valueType: "JSON",
			raw:       `{"large":9223372036854775807}`,
			assert: func(t *testing.T, value any) {
				t.Helper()
				object, ok := value.(map[string]any)
				if !ok {
					t.Fatalf("JSON type changed: value=%#v (%T)", value, value)
				}
				number, ok := object["large"].(json.Number)
				if !ok || number.String() != "9223372036854775807" {
					t.Fatalf("JSON numeric contract changed: value=%#v (%T)", object["large"], object["large"])
				}
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cache := &sonicClassifiedCacheStore{}
			repo := &fakeRepository{
				groupsByCode: map[string]*domain.ConfigGroup{
					"SEVEN_FRONTEND_METADATA": {ID: 20, GroupCode: "SEVEN_FRONTEND_METADATA", GroupName: "Metadata", Status: 1},
				},
				configsByGroupKey: map[string]*domain.Config{
					"20|themePrimaryColor": {
						ID:            21,
						GroupID:       20,
						GroupCode:     "SEVEN_FRONTEND_METADATA",
						GroupName:     "Metadata",
						ConfigKey:     "themePrimaryColor",
						ConfigValue:   tt.raw,
						ValueType:     tt.valueType,
						Exposure:      "PUBLIC",
						Sensitivity:   "NORMAL",
						SchemaVersion: 1,
						Version:       1,
						IsEnabled:     1,
					},
				},
			}
			service := NewService(nil, repo, cache, domain.NewService(), &fakeSecretCipher{}, &fakeRevealCipher{}, &fakeUserLookup{})
			for attempt := 0; attempt < 2; attempt++ {
				item, err := service.GetConfigByKeyForClient(context.Background(), Actor{}, "SEVEN_FRONTEND_METADATA.themePrimaryColor")
				if err != nil || item == nil {
					t.Fatalf("classified read %d: item=%#v err=%v", attempt, item, err)
				}
				tt.assert(t, item.Value)
			}
			if cache.loads != 1 {
				t.Fatalf("expected one authoritative load then a Sonic cache hit, got %d", cache.loads)
			}
		})
	}
}

// A catalogued key is not cache-public merely because its row says PUBLIC.
// If the parent group becomes permission-gated, a prior anonymous L1/L2
// record must not bypass the existing group authorization rule.
func TestClassifiedConfigCacheDoesNotBypassParentGroupPermission(t *testing.T) {
	cache := &sonicClassifiedCacheStore{}
	group := &domain.ConfigGroup{ID: 20, GroupCode: "SEVEN_FRONTEND_METADATA", GroupName: "Metadata", Status: 1}
	repo := &fakeRepository{
		groupsByCode: map[string]*domain.ConfigGroup{
			"SEVEN_FRONTEND_METADATA": group,
		},
		configsByGroupKey: map[string]*domain.Config{
			"20|themePrimaryColor": {
				ID:            21,
				GroupID:       20,
				GroupCode:     "SEVEN_FRONTEND_METADATA",
				GroupName:     "Metadata",
				ConfigKey:     "themePrimaryColor",
				ConfigValue:   "#123456",
				ValueType:     "STRING",
				Exposure:      "PUBLIC",
				Sensitivity:   "NORMAL",
				SchemaVersion: 1,
				Version:       1,
				IsEnabled:     1,
			},
		},
	}
	service := NewService(nil, repo, cache, domain.NewService(), &fakeSecretCipher{}, &fakeRevealCipher{}, &fakeUserLookup{})
	if item, err := service.GetConfigByKeyForClient(context.Background(), Actor{}, "SEVEN_FRONTEND_METADATA.themePrimaryColor"); err != nil || item == nil || item.Value != "#123456" {
		t.Fatalf("prime classified public cache: item=%#v err=%v", item, err)
	}
	if cache.loads != 1 {
		t.Fatalf("classified public cache did not prime: loads=%d", cache.loads)
	}

	group.PermissionCode = "config:metadata:read"
	_, err := service.GetConfigByKeyForClient(context.Background(), Actor{}, "SEVEN_FRONTEND_METADATA.themePrimaryColor")
	if appErr := apperrors.From(err); appErr == nil || appErr.Code() != apperrors.CodeForbidden {
		t.Fatalf("permission-gated parent group returned cached public value: %v", err)
	}
}

func TestSecretManagementContractIsWriteOnlyAndAuditProtected(t *testing.T) {
	repo := &fakeRepository{
		groupsByID: map[int64]*domain.ConfigGroup{
			10: {ID: 10, GroupCode: "secure", GroupName: "Secure", Status: 1},
		},
	}
	service := NewService(nil, repo, &fakeCacheStore{}, domain.NewService(), &fakeSecretCipher{}, &fakeRevealCipher{}, &fakeUserLookup{})
	schemaVersion := 1
	id, err := service.AddConfig(context.Background(), Actor{UserID: 1001, IsAdmin: true}, configfacade.ConfigAddRequest{
		GroupID:       10,
		ConfigKey:     "apiToken",
		ConfigValue:   "top-secret",
		ValueType:     "STRING",
		Sensitivity:   "SECRET",
		Exposure:      "INTERNAL",
		SchemaVersion: &schemaVersion,
	})
	if err != nil {
		t.Fatalf("add secret: %v", err)
	}
	stored := repo.configsByID[id]
	if stored == nil || stored.ConfigValue != "" || stored.ExtJSON == nil || stored.ExtJSON.Secret == nil {
		t.Fatalf("secret was not encrypted at rest: %#v", stored)
	}
	if len(repo.insertedLogs) != 1 || repo.insertedLogs[0].NewValue != "[PROTECTED]" {
		t.Fatalf("secret leaked into change log: %#v", repo.insertedLogs)
	}
	vo, err := service.GetConfigByID(context.Background(), Actor{UserID: 1001, IsAdmin: true}, id)
	if err != nil {
		t.Fatalf("get secret metadata: %v", err)
	}
	if vo.ConfigValue != "" || !vo.ValuePresent || vo.Sensitivity != "SECRET" || vo.Version != 1 {
		t.Fatalf("secret read contract leaked or omitted metadata: %#v", vo)
	}
	if _, err := service.RevealSensitiveValue(context.Background(), Actor{
		UserID:      1001,
		IsAdmin:     true,
		StepUpProof: validConfigStepUpProof(stepUpActionConfigSensitiveReveal, sensitiveRevealBinding(id)),
	}, id, configfacade.ConfigSensitiveRevealRequest{ObfuscatedClientPublicKey: "client-key"}); err == nil {
		t.Fatal("SECRET reveal unexpectedly succeeded")
	}
}

func TestProtectedConfigRollbackDoesNotResolveOrLeakPlaintext(t *testing.T) {
	const canary = "CANARY-ROLLBACK-SENSITIVE-PLAINTEXT"
	repo := &fakeRepository{
		configsByID: map[int64]*domain.Config{
			41: {
				ID:            41,
				GroupID:       1,
				GroupCode:     "secure",
				ConfigKey:     "token",
				IsSensitive:   1,
				Sensitivity:   "SENSITIVE",
				IsEnabled:     1,
				ConfigValue:   canary,
				EffectType:    "realtime",
				SchemaVersion: 1,
				Version:       2,
			},
		},
		changeLogsByID: map[int64]*domain.ConfigChangeLog{
			51: {
				ID:                51,
				ConfigID:          41,
				ConfigKey:         "token",
				Status:            string(domain.ConfigStatusApplied),
				OldValue:          "[PROTECTED]",
				NewValue:          "[PROTECTED]",
				OldValueProtected: true,
				NewValueProtected: true,
			},
		},
	}
	service := NewService(nil, repo, &fakeCacheStore{}, domain.NewService(), &fakeSecretCipher{}, &fakeRevealCipher{}, &fakeUserLookup{})
	err := service.RollbackConfigChange(context.Background(), Actor{
		UserID:      7,
		IsAdmin:     true,
		StepUpProof: validConfigStepUpProof(stepUpActionConfigRollback, configRollbackBinding(51)),
	}, 51, "rollback")
	if err == nil {
		t.Fatal("protected rollback unexpectedly succeeded")
	}
	if strings.Contains(err.Error(), canary) || strings.Contains(err.Error(), "当前值：") {
		t.Fatalf("protected rollback leaked plaintext: %v", err)
	}
	if !strings.Contains(err.Error(), "受保护配置不支持") {
		t.Fatalf("unexpected protected rollback error: %v", err)
	}
}

func TestInternalConsumerReadRequiresExactRegistration(t *testing.T) {
	repo := &fakeRepository{
		groupsByCode: map[string]*domain.ConfigGroup{
			"runtime": {ID: 20, GroupCode: "runtime", GroupName: "Runtime", Status: 1},
		},
		configsByGroupKey: map[string]*domain.Config{
			"20|title": {ID: 21, GroupID: 20, GroupCode: "runtime", ConfigKey: "title", ConfigValue: "Seven", ValueType: "STRING", UIWidget: "INPUT", Exposure: "INTERNAL", Sensitivity: "SENSITIVE", SchemaVersion: 1, Version: 3, IsEnabled: 1},
		},
	}
	service := NewService(&fakeGovernedTransactor{}, repo, &fakeCacheStore{}, domain.NewService(), &fakeSecretCipher{}, &fakeRevealCipher{}, &fakeUserLookup{})
	service.BindConsumerRegistry([]domain.ConsumerRegistration{{
		ConsumerID:        "runtime.shell",
		FullyQualifiedKey: "runtime.title",
		ScopeID:           "server:local",
		Purpose:           "render title",
		AllowedSecret:     domain.ConfigSensitivitySensitive,
	}})
	request := configfacade.ConfigInternalReadRequest{
		ConsumerID:         "runtime.shell",
		FullyQualifiedKey:  "runtime.title",
		ServerScope:        "server:local",
		Purpose:            "render title",
		AllowedSensitivity: "SENSITIVE",
	}
	item, err := service.GetConfigForConsumer(context.Background(), request)
	if err != nil || item == nil || item.Value != "Seven" || item.Version != 3 {
		t.Fatalf("registered internal read failed: item=%#v err=%v", item, err)
	}
	request.Purpose = "export title"
	if _, err := service.GetConfigForConsumer(context.Background(), request); err == nil {
		t.Fatal("purpose mismatch unexpectedly succeeded")
	}
}

func TestInternalConsumerListBatchAndCacheUseExactRegisteredIdentity(t *testing.T) {
	repo := &fakeRepository{
		groupsByCode: map[string]*domain.ConfigGroup{
			"runtime": {ID: 20, GroupCode: "runtime", GroupName: "Runtime", Status: 1},
		},
		configsByGroupKey: map[string]*domain.Config{
			"20|title": {ID: 21, GroupID: 20, GroupCode: "runtime", ConfigKey: "title", ConfigValue: "Seven", ValueType: "STRING", Exposure: "INTERNAL", Sensitivity: "NORMAL", SchemaVersion: 1, Version: 3, IsEnabled: 1},
			"20|mode":  {ID: 22, GroupID: 20, GroupCode: "runtime", ConfigKey: "mode", ConfigValue: "safe", ValueType: "STRING", Exposure: "INTERNAL", Sensitivity: "NORMAL", SchemaVersion: 1, Version: 4, IsEnabled: 1},
		},
	}
	cache := &fakeCacheStore{}
	service := NewService(&fakeGovernedTransactor{}, repo, cache, domain.NewService(), &fakeSecretCipher{}, &fakeRevealCipher{}, &fakeUserLookup{})
	service.BindConfigConsumers([]configfacade.ConfigConsumerRegistration{
		{ConsumerID: "runtime.shell", FullyQualifiedKey: "runtime.title", ServerScope: "server:local", Purpose: "render runtime", AllowedSensitivity: "NORMAL"},
		{ConsumerID: "runtime.shell", FullyQualifiedKey: "runtime.mode", ServerScope: "server:local", Purpose: "render runtime", AllowedSensitivity: "NORMAL"},
	})

	batch, err := service.GetConfigBatchForConsumer(context.Background(), configfacade.ConfigInternalBatchReadRequest{
		ConsumerID:         "runtime.shell",
		FullyQualifiedKeys: []string{"runtime.mode", "runtime.title"},
		ServerScope:        "server:local",
		Purpose:            "render runtime",
		AllowedSensitivity: "NORMAL",
	})
	if err != nil || len(batch) != 2 {
		t.Fatalf("registered internal batch failed: result=%#v err=%v", batch, err)
	}
	if repo.listGroupsByCodesCalls != 1 || repo.listConfigsByRefsCalls != 1 ||
		repo.findGroupByCodeCalls != 0 || repo.findConfigByGroupKeyCalls != 0 {
		t.Fatalf("internal batch query shape groups=%d configs=%d findGroup=%d findConfig=%d",
			repo.listGroupsByCodesCalls, repo.listConfigsByRefsCalls,
			repo.findGroupByCodeCalls, repo.findConfigByGroupKeyCalls)
	}
	if !strings.Contains(cache.lastBatchKey, "consumer=runtime.shell") ||
		!strings.Contains(cache.lastBatchKey, "scope=server:local") ||
		!strings.Contains(cache.lastBatchKey, "purpose=render runtime") ||
		!strings.Contains(cache.lastBatchKey, "allowed=NORMAL") {
		t.Fatalf("internal cache key is not policy-bound: %q", cache.lastBatchKey)
	}

	listed, err := service.ListConfigsForConsumer(context.Background(), configfacade.ConfigInternalListRequest{
		ConsumerID:         "runtime.shell",
		ServerScope:        "server:local",
		Purpose:            "render runtime",
		AllowedSensitivity: "NORMAL",
	})
	if err != nil || len(listed) != 2 {
		t.Fatalf("registered internal list failed: result=%#v err=%v", listed, err)
	}
	if _, err := service.GetConfigBatchForConsumer(context.Background(), configfacade.ConfigInternalBatchReadRequest{
		ConsumerID:         "runtime.shell",
		FullyQualifiedKeys: []string{"runtime.title"},
		ServerScope:        "server:other",
		Purpose:            "render runtime",
		AllowedSensitivity: "NORMAL",
	}); err == nil {
		t.Fatal("scope mismatch unexpectedly succeeded")
	}
}

func TestBatchColdReadsUseOneSnapshotAndFailClosedWithoutSnapshotSupport(t *testing.T) {
	newRepository := func() *fakeRepository {
		return &fakeRepository{
			groupsByCode: map[string]*domain.ConfigGroup{
				"runtime": {ID: 20, GroupCode: "runtime", GroupName: "Runtime", Status: 1},
			},
			configsByGroupKey: map[string]*domain.Config{
				"20|title": {
					ID: 21, GroupID: 20, GroupCode: "runtime", ConfigKey: "title",
					ConfigValue: "Seven", ValueType: "STRING", Exposure: "AUTHENTICATED",
					Sensitivity: "NORMAL", SchemaVersion: 1, Version: 3, IsEnabled: 1,
				},
			},
		}
	}
	t.Run("registered consumer", func(t *testing.T) {
		repo := newRepository()
		tx := &fakeGovernedTransactor{}
		service := NewService(tx, repo, &fakeCacheStore{}, domain.NewService(), &fakeSecretCipher{}, &fakeRevealCipher{}, &fakeUserLookup{})
		service.BindConfigConsumers([]configfacade.ConfigConsumerRegistration{{
			ConsumerID: "runtime.shell", FullyQualifiedKey: "runtime.title",
			ServerScope: "server:local", Purpose: "render runtime", AllowedSensitivity: "NORMAL",
		}})
		_, err := service.GetConfigBatchForConsumer(context.Background(), configfacade.ConfigInternalBatchReadRequest{
			ConsumerID: "runtime.shell", FullyQualifiedKeys: []string{"runtime.title"},
			ServerScope: "server:local", Purpose: "render runtime", AllowedSensitivity: "NORMAL",
		})
		if err != nil {
			t.Fatalf("consumer batch cold read: %v", err)
		}
		if tx.snapshotCalls != 1 || repo.listGroupsByCodesCalls != 1 || repo.listConfigsByRefsCalls != 1 {
			t.Fatalf("consumer cold read snapshot=%d groups=%d configs=%d",
				tx.snapshotCalls, repo.listGroupsByCodesCalls, repo.listConfigsByRefsCalls)
		}
	})
	t.Run("authenticated client", func(t *testing.T) {
		repo := newRepository()
		tx := &fakeGovernedTransactor{}
		service := NewService(tx, repo, &fakeCacheStore{}, domain.NewService(), &fakeSecretCipher{}, &fakeRevealCipher{}, &fakeUserLookup{})
		_, err := service.GetConfigBatchForClient(context.Background(), Actor{
			Authenticated: true, AccountID: 42, ScopeID: "org:7", AuthzVersion: 9,
		}, configfacade.ConfigBatchRequest{ConfigKeys: []string{"runtime.title"}})
		if err != nil {
			t.Fatalf("client batch cold read: %v", err)
		}
		if tx.snapshotCalls != 1 || repo.listGroupsByCodesCalls != 1 || repo.listConfigsByRefsCalls != 1 {
			t.Fatalf("client cold read snapshot=%d groups=%d configs=%d",
				tx.snapshotCalls, repo.listGroupsByCodesCalls, repo.listConfigsByRefsCalls)
		}
	})
	t.Run("missing snapshot support", func(t *testing.T) {
		repo := newRepository()
		service := NewService(nil, repo, &fakeCacheStore{}, domain.NewService(), &fakeSecretCipher{}, &fakeRevealCipher{}, &fakeUserLookup{})
		_, err := service.GetConfigBatchForClient(context.Background(), Actor{
			Authenticated: true, AccountID: 42, ScopeID: "org:7", AuthzVersion: 9,
		}, configfacade.ConfigBatchRequest{ConfigKeys: []string{"runtime.title"}})
		if err == nil {
			t.Fatal("client batch cold read unexpectedly ran without snapshot support")
		}
		if repo.listGroupsByCodesCalls != 0 || repo.listConfigsByRefsCalls != 0 {
			t.Fatalf("cold read reached repositories before snapshot guard: groups=%d configs=%d",
				repo.listGroupsByCodesCalls, repo.listConfigsByRefsCalls)
		}
	})
}

func TestClientCacheKeyBindsAccountScopeAndAuthorizationGeneration(t *testing.T) {
	cache := &fakeCacheStore{}
	repo := &fakeRepository{
		groupsByCode: map[string]*domain.ConfigGroup{
			"runtime": {ID: 20, GroupCode: "runtime", GroupName: "Runtime", Status: 1},
		},
		configsByGroupKey: map[string]*domain.Config{
			"20|title": {ID: 21, GroupID: 20, GroupCode: "runtime", ConfigKey: "title", ConfigValue: "Seven", ValueType: "STRING", UIWidget: "INPUT", Exposure: "AUTHENTICATED", Sensitivity: "NORMAL", SchemaVersion: 1, Version: 3, IsEnabled: 1},
		},
	}
	service := NewService(nil, repo, cache, domain.NewService(), &fakeSecretCipher{}, &fakeRevealCipher{}, &fakeUserLookup{})
	_, err := service.GetConfigByKeyForClient(context.Background(), Actor{
		Authenticated: true,
		AccountID:     42,
		ScopeID:       "org:7",
		AuthzVersion:  9,
	}, "runtime.title")
	if err != nil {
		t.Fatalf("client read: %v", err)
	}
	if !strings.Contains(cache.lastSetConfigKey, "account=42|scope=org:7|authz=9|key=runtime.title") {
		t.Fatalf("cache key is not identity-bound: %q", cache.lastSetConfigKey)
	}
}

func TestClientListFiltersWithSameExposureAndSensitivityPolicy(t *testing.T) {
	repo := &fakeRepository{
		groupsByCode: map[string]*domain.ConfigGroup{
			"runtime": {ID: 20, GroupCode: "runtime", GroupName: "Runtime", Status: 1},
		},
		configsByID: map[int64]*domain.Config{
			21: {ID: 21, GroupID: 20, GroupCode: "runtime", ConfigKey: "public", ConfigValue: "p", ValueType: "STRING", Exposure: "PUBLIC", Sensitivity: "NORMAL", SchemaVersion: 1, IsEnabled: 1},
			22: {ID: 22, GroupID: 20, GroupCode: "runtime", ConfigKey: "account", ConfigValue: "a", ValueType: "STRING", Exposure: "AUTHENTICATED", Sensitivity: "NORMAL", SchemaVersion: 1, IsEnabled: 1},
			23: {ID: 23, GroupID: 20, GroupCode: "runtime", ConfigKey: "internal", ConfigValue: "i", ValueType: "STRING", Exposure: "INTERNAL", Sensitivity: "NORMAL", SchemaVersion: 1, IsEnabled: 1},
			24: {ID: 24, GroupID: 20, GroupCode: "runtime", ConfigKey: "secret", ConfigValue: "s", ValueType: "STRING", Exposure: "PUBLIC", Sensitivity: "SENSITIVE", IsSensitive: 1, SchemaVersion: 1, IsEnabled: 1},
		},
	}
	service := NewService(nil, repo, &fakeCacheStore{}, domain.NewService(), &fakeSecretCipher{}, &fakeRevealCipher{}, &fakeUserLookup{})

	anonymous, err := service.ListConfigsForClient(context.Background(), Actor{}, configfacade.ConfigClientListRequest{GroupCode: "runtime"})
	if err != nil {
		t.Fatalf("anonymous list: %v", err)
	}
	if len(anonymous) != 1 || anonymous["runtime.public"].Value != "p" {
		t.Fatalf("anonymous list leaked protected rows: %#v", anonymous)
	}
	authenticated, err := service.ListConfigsForClient(context.Background(), Actor{
		Authenticated: true,
		AccountID:     7,
		ScopeID:       "org:9",
		AuthzVersion:  3,
	}, configfacade.ConfigClientListRequest{GroupCode: "runtime"})
	if err != nil {
		t.Fatalf("authenticated list: %v", err)
	}
	if len(authenticated) != 2 || authenticated["runtime.account"].Value != "a" {
		t.Fatalf("authenticated list policy mismatch: %#v", authenticated)
	}
}

func TestConfigConsumerRegistryMatchesConnectedScalarRuntimeKeys(t *testing.T) {
	service := NewService(nil, &fakeRepository{}, &fakeCacheStore{}, domain.NewService(), nil, nil, nil)
	consumers := service.ListConfigConsumers(context.Background())
	byKey := make(map[string]configfacade.ConfigConsumerVO, len(consumers))
	for _, consumer := range consumers {
		byKey[consumer.FullyQualifiedKey] = consumer
	}
	for _, key := range []string{
		"SEVEN_FRONTEND_METADATA.title",
		"SEVEN_FRONTEND_METADATA.shortTitle",
		"SEVEN_FRONTEND_METADATA.themePrimaryColor",
		"SEVEN_FRONTEND_METADATA.loginLogo",
		"SEVEN_FRONTEND_METADATA.favicon",
	} {
		consumer, ok := byKey[key]
		if !ok || !consumer.Connected || consumer.Source != "sys_config" ||
			consumer.ActualConsumer == "" || consumer.Activation == "" || consumer.CacheRule == "" {
			t.Fatalf("expected complete connected runtime consumer for %s, got %#v", key, consumer)
		}
		groupCode, configKey, _ := strings.Cut(key, ".")
		if !configConnected(&domain.Config{GroupCode: groupCode, ConfigKey: configKey}) {
			t.Fatalf("expected management connected status for %s", key)
		}
	}
	if configConnected(&domain.Config{GroupCode: "SEVEN_FRONTEND_METADATA", ConfigKey: "helpEntry"}) {
		t.Fatal("unverified scalar key must remain unconnected")
	}
}

func TestConfigPaginationAcceptsLegacyPageNum(t *testing.T) {
	repo := &fakeRepository{}
	tx := &fakeGovernedTransactor{}
	service := NewService(tx, repo, &fakeCacheStore{}, domain.NewService(), &fakeSecretCipher{}, &fakeRevealCipher{}, &fakeUserLookup{})

	if _, err := service.GetConfigPage(context.Background(), Actor{IsAdmin: true}, configfacade.ConfigQueryRequest{
		PageNum:  3,
		PageSize: 25,
	}); err != nil {
		t.Fatalf("get config page: %v", err)
	}
	if repo.lastConfigQuery.Current != 3 || repo.lastConfigQuery.PageSize != 25 {
		t.Fatalf("expected legacy pageNum to drive config pagination, got %#v", repo.lastConfigQuery)
	}

	if _, err := service.GetConfigGroupPage(context.Background(), Actor{IsAdmin: true}, configfacade.ConfigGroupQueryRequest{
		PageNum:  4,
		PageSize: 15,
	}); err != nil {
		t.Fatalf("get group page: %v", err)
	}
	if repo.lastGroupQuery.Current != 4 || repo.lastGroupQuery.PageSize != 15 {
		t.Fatalf("expected legacy pageNum to drive group pagination, got %#v", repo.lastGroupQuery)
	}
	if tx.snapshotCalls != 2 || tx.ordinaryCalls != 0 || tx.consistentCalls != 0 {
		t.Fatalf("pagination transaction shape snapshot=%d ordinary=%d consistent=%d",
			tx.snapshotCalls, tx.ordinaryCalls, tx.consistentCalls)
	}
}

func TestConfigPagesFailClosedWithoutSnapshot(t *testing.T) {
	repo := &fakeRepository{}
	service := NewService(nil, repo, &fakeCacheStore{}, domain.NewService(), &fakeSecretCipher{}, &fakeRevealCipher{}, &fakeUserLookup{})

	if _, err := service.GetConfigPage(context.Background(), Actor{IsAdmin: true}, configfacade.ConfigQueryRequest{PageSize: 20}); err == nil {
		t.Fatal("config page unexpectedly ran without a read-only snapshot")
	}
	if _, err := service.GetConfigGroupPage(context.Background(), Actor{IsAdmin: true}, configfacade.ConfigGroupQueryRequest{PageSize: 20}); err == nil {
		t.Fatal("config group page unexpectedly ran without a read-only snapshot")
	}
	if repo.queryConfigsCalls != 0 || repo.queryGroupsCalls != 0 {
		t.Fatalf("page queries reached repository before snapshot guard: configs=%d groups=%d",
			repo.queryConfigsCalls, repo.queryGroupsCalls)
	}
}

func TestConfigScopeFiltersConfigAndGroupPages(t *testing.T) {
	repo := &fakeRepository{
		groupsByID: map[int64]*domain.ConfigGroup{
			1: {ID: 1, GroupCode: "public", GroupName: "Public", Status: 1},
			2: {ID: 2, GroupCode: "ops", GroupName: "Ops", Status: 1},
		},
		configsByID: map[int64]*domain.Config{
			101: {ID: 101, GroupID: 1, GroupCode: "public", GroupName: "Public", ConfigKey: "title", ConfigValue: "Seven", ValueType: "string", IsEnabled: 1},
			102: {ID: 102, GroupID: 2, GroupCode: "ops", GroupName: "Ops", ConfigKey: "token", ConfigValue: "hidden", ValueType: "string", IsEnabled: 1},
		},
		configScopeGrants: []domain.ConfigScopeGrant{
			{RoleID: 501, GroupCode: "public", CanRead: 1},
		},
	}
	service := NewService(&fakeGovernedTransactor{}, repo, &fakeCacheStore{}, domain.NewService(), &fakeSecretCipher{}, &fakeRevealCipher{}, &fakeUserLookup{})
	actor := Actor{UserID: 1001, RoleIDs: []int64{501}}

	configPage, err := service.GetConfigPage(context.Background(), actor, configfacade.ConfigQueryRequest{PageSize: 20})
	if err != nil {
		t.Fatalf("get config page: %v", err)
	}
	if len(configPage.Records) != 1 || configPage.Records[0].ConfigKey != "title" {
		t.Fatalf("expected only public title config, got %#v", configPage.Records)
	}
	if !configPage.Records[0].Access.CanRead || configPage.Records[0].Access.CanWrite {
		t.Fatalf("expected read-only access marker, got %#v", configPage.Records[0].Access)
	}

	groupPage, err := service.GetConfigGroupPage(context.Background(), actor, configfacade.ConfigGroupQueryRequest{PageSize: 20})
	if err != nil {
		t.Fatalf("get group page: %v", err)
	}
	if len(groupPage.Records) != 1 || groupPage.Records[0].GroupCode != "public" {
		t.Fatalf("expected only public group, got %#v", groupPage.Records)
	}
	if repo.scopeGrantListCalls != 2 {
		t.Fatalf("page access grants queried %d times, want once per page", repo.scopeGrantListCalls)
	}
}

func TestConfigScopeKeyGrantShowsGroupWithoutGroupWrite(t *testing.T) {
	groupName := "Public Updated"
	repo := &fakeRepository{
		groupsByID: map[int64]*domain.ConfigGroup{
			1: {ID: 1, GroupCode: "public", GroupName: "Public", Status: 1},
			2: {ID: 2, GroupCode: "ops", GroupName: "Ops", Status: 1},
		},
		configsByID: map[int64]*domain.Config{
			101: {ID: 101, GroupID: 1, GroupCode: "public", GroupName: "Public", ConfigKey: "title", ConfigValue: "Seven", ValueType: "string", IsEnabled: 1},
			102: {ID: 102, GroupID: 2, GroupCode: "ops", GroupName: "Ops", ConfigKey: "token", ConfigValue: "hidden", ValueType: "string", IsEnabled: 1},
		},
		configScopeGrants: []domain.ConfigScopeGrant{
			{RoleID: 501, GroupCode: "public", ConfigKey: "title", CanRead: 1, CanWrite: 1},
		},
	}
	service := NewService(&fakeGovernedTransactor{}, repo, &fakeCacheStore{}, domain.NewService(), &fakeSecretCipher{}, &fakeRevealCipher{}, &fakeUserLookup{})
	actor := Actor{UserID: 1001, RoleIDs: []int64{501}}

	groupPage, err := service.GetConfigGroupPage(context.Background(), actor, configfacade.ConfigGroupQueryRequest{PageSize: 20})
	if err != nil {
		t.Fatalf("get group page: %v", err)
	}
	if len(groupPage.Records) != 1 || groupPage.Records[0].GroupCode != "public" {
		t.Fatalf("expected key-scoped public group visibility, got %#v", groupPage.Records)
	}
	if !groupPage.Records[0].Access.CanRead || groupPage.Records[0].Access.CanWrite {
		t.Fatalf("expected key grant to expose group read-only, got %#v", groupPage.Records[0].Access)
	}

	err = service.UpdateConfigGroup(context.Background(), actor, configfacade.ConfigGroupUpdateRequest{
		ID:        1,
		GroupName: &groupName,
	})
	appErr := apperrors.From(err)
	if appErr == nil || appErr.Code() != apperrors.CodeForbidden {
		t.Fatalf("expected forbidden for group write with key-only grant, got %v", err)
	}
}

func TestConfigScopeRejectsDetailOutsideScope(t *testing.T) {
	repo := &fakeRepository{
		configsByID: map[int64]*domain.Config{
			102: {ID: 102, GroupID: 2, GroupCode: "ops", GroupName: "Ops", ConfigKey: "token", ConfigValue: "hidden", ValueType: "string", IsEnabled: 1},
		},
		configScopeGrants: []domain.ConfigScopeGrant{
			{RoleID: 501, GroupCode: "public", CanRead: 1},
		},
	}
	service := NewService(nil, repo, &fakeCacheStore{}, domain.NewService(), &fakeSecretCipher{}, &fakeRevealCipher{}, &fakeUserLookup{})

	_, err := service.GetConfigByID(context.Background(), Actor{UserID: 1001, RoleIDs: []int64{501}}, 102)
	appErr := apperrors.From(err)
	if appErr == nil || appErr.Code() != apperrors.CodeDataScopeDenied {
		t.Fatalf("expected data scope denied, got %v", err)
	}
}

func TestConfigScopeRejectsWriteWithReadOnlyGrant(t *testing.T) {
	nextValue := "new-title"
	repo := &fakeRepository{
		groupsByID: map[int64]*domain.ConfigGroup{
			1: {ID: 1, GroupCode: "public", GroupName: "Public", Status: 1},
		},
		configsByID: map[int64]*domain.Config{
			101: {ID: 101, GroupID: 1, GroupCode: "public", GroupName: "Public", ConfigKey: "title", ConfigValue: "Seven", ValueType: "string", EffectType: "realtime", IsEnabled: 1},
		},
		configScopeGrants: []domain.ConfigScopeGrant{
			{RoleID: 501, GroupCode: "public", CanRead: 1},
		},
	}
	service := NewService(nil, repo, &fakeCacheStore{}, domain.NewService(), &fakeSecretCipher{}, &fakeRevealCipher{}, &fakeUserLookup{})

	err := service.UpdateConfig(context.Background(), Actor{UserID: 1001, RoleIDs: []int64{501}}, configfacade.ConfigUpdateRequest{
		ID:          101,
		ConfigValue: &nextValue,
	})
	appErr := apperrors.From(err)
	if appErr == nil || appErr.Code() != apperrors.CodeForbidden {
		t.Fatalf("expected forbidden for read-only scope, got %v", err)
	}
}

func TestConfigScopeAllowsWriteGrant(t *testing.T) {
	nextValue := "new-title"
	repo := &fakeRepository{
		groupsByID: map[int64]*domain.ConfigGroup{
			1: {ID: 1, GroupCode: "public", GroupName: "Public", Status: 1},
		},
		configsByID: map[int64]*domain.Config{
			101: {ID: 101, GroupID: 1, GroupCode: "public", GroupName: "Public", ConfigKey: "title", ConfigValue: "Seven", ValueType: "string", EffectType: "realtime", IsEnabled: 1},
		},
		configScopeGrants: []domain.ConfigScopeGrant{
			{RoleID: 501, GroupCode: "public", CanRead: 1, CanWrite: 1},
		},
	}
	cache := &fakeCacheStore{}
	service := NewService(nil, repo, cache, domain.NewService(), &fakeSecretCipher{}, &fakeRevealCipher{}, &fakeUserLookup{})

	if err := service.UpdateConfig(context.Background(), Actor{UserID: 1001, RoleIDs: []int64{501}}, configfacade.ConfigUpdateRequest{
		ID:          101,
		ConfigValue: &nextValue,
	}); err != nil {
		t.Fatalf("update config with write grant: %v", err)
	}
	if got := repo.configsByID[101].ConfigValue; got != "new-title" {
		t.Fatalf("expected updated value, got %q", got)
	}
	if cache.bumpCount != 0 {
		t.Fatalf("legacy post-commit cache invalidation must be retired, got %d", cache.bumpCount)
	}
}

func TestConfigScopeRejectsSensitiveRevealOutsideScope(t *testing.T) {
	repo := &fakeRepository{
		configsByID: map[int64]*domain.Config{
			201: {
				ID:          201,
				GroupID:     2,
				GroupCode:   "ops",
				GroupName:   "Ops",
				ConfigKey:   "token",
				ValueType:   "string",
				IsSensitive: 1,
				IsEnabled:   1,
				ExtJSON: &domain.ConfigExtJSON{Secret: &domain.ConfigSecretValue{
					Plain: "plain-token",
				}},
			},
		},
		configScopeGrants: []domain.ConfigScopeGrant{
			{RoleID: 501, GroupCode: "public", CanRead: 1},
		},
	}
	service := NewService(nil, repo, &fakeCacheStore{}, domain.NewService(), &fakeSecretCipher{}, &fakeRevealCipher{}, &fakeUserLookup{})

	_, err := service.RevealSensitiveValue(context.Background(), Actor{
		UserID:      1001,
		RoleIDs:     []int64{501},
		Permissions: []string{"system:config:sensitive"},
		StepUpProof: validConfigStepUpProof("CONFIG_SENSITIVE_REVEAL", "config:201|reveal"),
	}, 201, configfacade.ConfigSensitiveRevealRequest{ObfuscatedClientPublicKey: "client-key"})
	appErr := apperrors.From(err)
	if appErr == nil || appErr.Code() != apperrors.CodeDataScopeDenied {
		t.Fatalf("expected data scope denied for sensitive reveal outside scope, got %v", err)
	}
}

func TestRevealSensitiveValueRequiresServiceProofMetadata(t *testing.T) {
	repo := &fakeRepository{
		configsByID: map[int64]*domain.Config{
			201: {
				ID:          201,
				GroupID:     2,
				GroupCode:   "ops",
				GroupName:   "Ops",
				ConfigKey:   "token",
				ValueType:   "string",
				IsSensitive: 1,
				IsEnabled:   1,
				ExtJSON: &domain.ConfigExtJSON{Secret: &domain.ConfigSecretValue{
					Plain: "plain-token",
				}},
			},
		},
	}
	service := NewService(nil, repo, &fakeCacheStore{}, domain.NewService(), &fakeSecretCipher{}, &fakeRevealCipher{}, &fakeUserLookup{})
	baseActor := Actor{UserID: 1001, IsAdmin: true, Permissions: []string{"system:config:sensitive"}}

	for _, tt := range []struct {
		name  string
		proof stepup.ProofMetadata
	}{
		{name: "missing proof"},
		{name: "wrong action", proof: stepup.ProofMetadata{
			BusinessAction:   "CONFIG_SCOPE_ASSIGN",
			OperationBinding: "config:201|reveal",
			ProofIdentifier:  "proof-jti",
		}},
		{name: "wrong binding", proof: stepup.ProofMetadata{
			BusinessAction:   "CONFIG_SENSITIVE_REVEAL",
			OperationBinding: "config:202|reveal",
			ProofIdentifier:  "proof-jti",
		}},
		{name: "missing proof id", proof: stepup.ProofMetadata{
			BusinessAction:   "CONFIG_SENSITIVE_REVEAL",
			OperationBinding: "config:201|reveal",
		}},
	} {
		t.Run(tt.name, func(t *testing.T) {
			actor := baseActor
			actor.StepUpProof = tt.proof

			_, err := service.RevealSensitiveValue(context.Background(), actor, 201, configfacade.ConfigSensitiveRevealRequest{ObfuscatedClientPublicKey: "client-key"})

			appErr := apperrors.From(err)
			if appErr == nil || appErr.Code() != apperrors.CodeForbidden {
				t.Fatalf("expected forbidden for invalid service proof metadata, got %v", err)
			}
		})
	}

	actor := baseActor
	actor.StepUpProof = validConfigStepUpProof("CONFIG_SENSITIVE_REVEAL", "config:201|reveal")
	result, err := service.RevealSensitiveValue(context.Background(), actor, 201, configfacade.ConfigSensitiveRevealRequest{ObfuscatedClientPublicKey: "client-key"})
	if err != nil {
		t.Fatalf("expected valid service proof metadata to allow reveal: %v", err)
	}
	if result == nil || result.EncryptedValue != "enc:plain-token" {
		t.Fatalf("unexpected reveal result: %#v", result)
	}
}

func TestAssignRoleConfigScopesValidatesAndReplacesGrants(t *testing.T) {
	repo := &fakeRepository{
		groupsByID: map[int64]*domain.ConfigGroup{
			1: {ID: 1, GroupCode: "public", GroupName: "Public", Status: 1},
		},
		configsByID: map[int64]*domain.Config{
			101: {ID: 101, GroupID: 1, GroupCode: "public", GroupName: "Public", ConfigKey: "title", ConfigValue: "Seven", ValueType: "string", IsEnabled: 1},
		},
	}
	tx := &fakeGovernedTransactor{}
	service := NewService(tx, repo, &fakeCacheStore{}, domain.NewService(), &fakeSecretCipher{}, &fakeRevealCipher{}, &fakeUserLookup{})
	service.BindRoleSecurity(nonRootRoleLookup{})

	err := service.AssignRoleConfigScopes(context.Background(), Actor{UserID: 1001, IsAdmin: true, StepUpProof: validConfigStepUpProof("CONFIG_SCOPE_ASSIGN", "config-scope:role:501|scopes:public.title:r0w1d0")}, 501, configfacade.AssignRoleConfigScopesRequest{
		Grants: []configfacade.ConfigScopeGrantVO{
			{GroupCode: " public ", ConfigKey: " title ", CanWrite: 1},
		},
	})
	if err != nil {
		t.Fatalf("assign role config scopes: %v", err)
	}
	if len(repo.replacedConfigScope) != 1 {
		t.Fatalf("expected one replaced grant, got %#v", repo.replacedConfigScope)
	}
	grant := repo.replacedConfigScope[0]
	if grant.GroupCode != "public" || grant.ConfigKey != "title" || grant.CanRead != 1 || grant.CanWrite != 1 {
		t.Fatalf("unexpected normalized grant: %#v", grant)
	}
	if repo.listGroupsByCodesCalls != 1 || repo.listConfigsByRefsCalls != 1 ||
		repo.findGroupByCodeCalls != 0 || repo.findConfigByGroupKeyCalls != 0 {
		t.Fatalf("scope validation query shape groups=%d configs=%d findGroup=%d findConfig=%d",
			repo.listGroupsByCodesCalls, repo.listConfigsByRefsCalls,
			repo.findGroupByCodeCalls, repo.findConfigByGroupKeyCalls)
	}
	if tx.consistentCalls != 1 || tx.ordinaryCalls != 0 {
		t.Fatalf("scope assignment transaction shape consistent=%d ordinary=%d", tx.consistentCalls, tx.ordinaryCalls)
	}

	err = service.AssignRoleConfigScopes(context.Background(), Actor{UserID: 1001, IsAdmin: true, StepUpProof: validConfigStepUpProof("CONFIG_SCOPE_ASSIGN", "config-scope:role:501|scopes:public.missing:r1w0d0")}, 501, configfacade.AssignRoleConfigScopesRequest{
		Grants: []configfacade.ConfigScopeGrantVO{
			{GroupCode: "public", ConfigKey: "missing", CanRead: 1},
		},
	})
	appErr := apperrors.From(err)
	if appErr == nil || appErr.Code() != apperrors.CodeParamsError {
		t.Fatalf("expected params error for missing config key, got %v", err)
	}
	if tx.consistentCalls != 2 {
		t.Fatalf("scope validation must run in the consistent transaction, calls=%d", tx.consistentCalls)
	}
}

func TestAssignRoleConfigScopesFailsClosedWithoutConsistentTransaction(t *testing.T) {
	repo := &fakeRepository{
		groupsByID: map[int64]*domain.ConfigGroup{
			1: {ID: 1, GroupCode: "public", GroupName: "Public", Status: 1},
		},
	}
	service := NewService(nil, repo, &fakeCacheStore{}, domain.NewService(), &fakeSecretCipher{}, &fakeRevealCipher{}, &fakeUserLookup{})

	err := service.AssignRoleConfigScopes(context.Background(), Actor{
		UserID:      1001,
		IsAdmin:     true,
		StepUpProof: validConfigStepUpProof("CONFIG_SCOPE_ASSIGN", "config-scope:role:501|scopes:public.:r1w0d0"),
	}, 501, configfacade.AssignRoleConfigScopesRequest{
		Grants: []configfacade.ConfigScopeGrantVO{{GroupCode: "public", CanRead: 1}},
	})
	if err == nil {
		t.Fatal("scope assignment unexpectedly ran without a consistent transaction")
	}
	if repo.listGroupsByCodesCalls != 0 || len(repo.replacedConfigScope) != 0 {
		t.Fatalf("scope assignment reached persistence before transaction guard: groups=%d replaced=%d",
			repo.listGroupsByCodesCalls, len(repo.replacedConfigScope))
	}
}

func TestAssignRoleConfigScopesFailsClosedWithoutRoleSecurity(t *testing.T) {
	repo := &fakeRepository{}
	service := NewService(&fakeGovernedTransactor{}, repo, &fakeCacheStore{}, domain.NewService(), &fakeSecretCipher{}, &fakeRevealCipher{}, &fakeUserLookup{})
	err := service.AssignRoleConfigScopes(context.Background(), Actor{
		UserID:      1001,
		IsAdmin:     true,
		StepUpProof: validConfigStepUpProof("CONFIG_SCOPE_ASSIGN", "config-scope:role:501|scopes:"),
	}, 501, configfacade.AssignRoleConfigScopesRequest{})
	if err == nil {
		t.Fatal("scope assignment unexpectedly ran without role security")
	}
	if len(repo.replacedConfigScope) != 0 {
		t.Fatalf("scope assignment reached persistence without role security: %#v", repo.replacedConfigScope)
	}
}

func TestNormalizeRoleConfigScopesRejectsUnboundedAndDuplicateInputBeforeQueries(t *testing.T) {
	repo := &fakeRepository{}
	service := NewService(nil, repo, &fakeCacheStore{}, domain.NewService(), &fakeSecretCipher{}, &fakeRevealCipher{}, &fakeUserLookup{})
	tooMany := make([]authorizationfacade.RoleConfigScopeGrantVO, roleConfigScopeGrantMax+1)
	for index := range tooMany {
		tooMany[index] = authorizationfacade.RoleConfigScopeGrantVO{GroupCode: "group-" + strconv.Itoa(index)}
	}
	if _, err := service.NormalizeRoleConfigScopes(context.Background(), tooMany); err == nil {
		t.Fatal("expected more than 100 grants to fail closed")
	}
	duplicates := []authorizationfacade.RoleConfigScopeGrantVO{
		{GroupCode: " app ", ConfigKey: " title "},
		{GroupCode: "app", ConfigKey: "title"},
	}
	if _, err := service.NormalizeRoleConfigScopes(context.Background(), duplicates); err == nil {
		t.Fatal("expected normalized duplicate grants to fail closed")
	}
	if repo.listGroupsByCodesCalls != 0 || repo.listConfigsByRefsCalls != 0 {
		t.Fatalf("invalid input reached batch queries: groups=%d configs=%d", repo.listGroupsByCodesCalls, repo.listConfigsByRefsCalls)
	}
}

func TestAssignRoleConfigScopesRejectsAuthorizationRoot(t *testing.T) {
	service := NewService(&fakeGovernedTransactor{}, &fakeRepository{}, &fakeCacheStore{}, domain.NewService(), &fakeSecretCipher{}, &fakeRevealCipher{}, &fakeUserLookup{})
	service.BindRoleSecurity(rootRoleLookup{})
	err := service.AssignRoleConfigScopes(context.Background(), Actor{UserID: 1001, StepUpProof: validConfigStepUpProof("CONFIG_SCOPE_ASSIGN", "config-scope:role:1|scopes:")}, 1, configfacade.AssignRoleConfigScopesRequest{})
	if appErr := apperrors.From(err); appErr == nil || appErr.Code() != apperrors.CodeOperateError {
		t.Fatalf("expected authorization root config scopes to be immutable, got %v", err)
	}
}

type rootRoleLookup struct{}

func (rootRoleLookup) GetRole(context.Context, int64) (*authorizationfacade.RoleVO, error) {
	return &authorizationfacade.RoleVO{RoleID: 1, AuthorizationRoot: true}, nil
}

func (rootRoleLookup) AdvanceRoleGrantRevision(context.Context, int64, int64) error { return nil }

type nonRootRoleLookup struct{}

func (nonRootRoleLookup) GetRole(_ context.Context, roleID int64) (*authorizationfacade.RoleVO, error) {
	return &authorizationfacade.RoleVO{RoleID: roleID}, nil
}

func (nonRootRoleLookup) AdvanceRoleGrantRevision(context.Context, int64, int64) error { return nil }

func validConfigStepUpProof(action, binding string) stepup.ProofMetadata {
	return stepup.ProofMetadata{
		BusinessAction:        action,
		OperationBinding:      binding,
		ProofIdentifier:       "proof-jti",
		ChallengeIdentifier:   "challenge-id",
		AssuranceLevel:        "AAL2",
		AuthenticationMethods: []string{"TIME_BASED_ONE_TIME_PASSWORD"},
	}
}

func TestUpdateSensitiveConfigIgnoresMaskedPlaceholderValue(t *testing.T) {
	repo := &fakeRepository{
		groupsByID: map[int64]*domain.ConfigGroup{
			1: {ID: 1, GroupCode: "app", GroupName: "App", Status: 1},
		},
		configsByID: map[int64]*domain.Config{
			80: {
				ID:          80,
				GroupID:     1,
				GroupCode:   "app",
				GroupName:   "App",
				ConfigKey:   "secret",
				ValueType:   "string",
				ConfigDesc:  "old desc",
				IsSensitive: 1,
				IsEnabled:   1,
				EffectType:  "realtime",
				ExtJSON: &domain.ConfigExtJSON{Secret: &domain.ConfigSecretValue{
					Plain:         "real-secret",
					CiphertextB64: "cipher:real-secret",
					EDEKB64:       "edek",
					WrapKeyRef:    "key-1",
				}},
			},
		},
	}
	service := NewService(nil, repo, &fakeCacheStore{}, domain.NewService(), &fakeSecretCipher{}, &fakeRevealCipher{}, &fakeUserLookup{})

	masked := "******"
	nextDesc := "new desc"
	sensitive := 1
	err := service.UpdateConfig(context.Background(), Actor{UserID: 1001, IsAdmin: true, Permissions: []string{"system:config:sensitive"}}, configfacade.ConfigUpdateRequest{
		ID:          80,
		ConfigValue: &masked,
		ConfigDesc:  &nextDesc,
		IsSensitive: &sensitive,
	})
	if err != nil {
		t.Fatalf("update config: %v", err)
	}
	updated := repo.configsByID[80]
	if updated == nil || updated.ExtJSON == nil || updated.ExtJSON.Secret == nil {
		t.Fatalf("expected sensitive value to remain in secret storage, got %#v", updated)
	}
	if updated.ExtJSON.Secret.Plain != "real-secret" {
		t.Fatalf("expected masked placeholder to preserve secret, got %#v", updated.ExtJSON.Secret)
	}
	if updated.ConfigDesc != "new desc" {
		t.Fatalf("expected metadata update, got %#v", updated)
	}
	if len(repo.insertedLogs) != 0 {
		t.Fatalf("expected no value change log for masked placeholder, got %#v", repo.insertedLogs)
	}
}

func TestApplyPendingConfigsUsesLatestPendingPerConfig(t *testing.T) {
	base := time.Now().UTC()
	repo := &fakeRepository{
		configsByID: map[int64]*domain.Config{
			21: {ID: 21, GroupID: 1, GroupCode: "app", GroupName: "App", ConfigKey: "theme", ConfigValue: "old", ValueType: "string", EffectType: "restart", IsEnabled: 1},
		},
		changeLogsByID: map[int64]*domain.ConfigChangeLog{
			301: {ID: 301, ConfigID: 21, ConfigKey: "theme", OperationType: "UPDATE", NewValue: "v1", EffectType: "restart", Status: "pending", OperationTime: ptrTime(base.Add(-time.Minute))},
			302: {ID: 302, ConfigID: 21, ConfigKey: "theme", OperationType: "UPDATE", NewValue: "v2", EffectType: "restart", Status: "pending", OperationTime: ptrTime(base)},
		},
		pendingLogs: []domain.ConfigChangeLog{
			{ID: 301, ConfigID: 21, ConfigKey: "theme", OperationType: "UPDATE", NewValue: "v1", EffectType: "restart", Status: "pending", OperationTime: ptrTime(base.Add(-time.Minute))},
			{ID: 302, ConfigID: 21, ConfigKey: "theme", OperationType: "UPDATE", NewValue: "v2", EffectType: "restart", Status: "pending", OperationTime: ptrTime(base)},
		},
	}
	cache := &fakeCacheStore{}
	service := NewService(nil, repo, cache, domain.NewService(), &fakeSecretCipher{}, &fakeRevealCipher{}, &fakeUserLookup{})

	count, err := service.ApplyPendingConfigs(context.Background(), Actor{UserID: 1001, Username: "admin", IsAdmin: true, StepUpProof: validConfigStepUpProof("CONFIG_APPLY_PENDING", "config:apply-pending")}, false)
	if err != nil {
		t.Fatalf("apply pending configs: %v", err)
	}
	if count != 1 {
		t.Fatalf("expected one applied config, got %d", count)
	}
	updated := repo.configsByID[21]
	if updated.ConfigValue != "v2" {
		t.Fatalf("expected latest pending value applied, got %#v", updated)
	}
	if repo.changeLogsByID[302].Status != "applied" {
		t.Fatalf("expected latest pending log applied, got %#v", repo.changeLogsByID[302])
	}
	if len(repo.insertedLogs) == 0 || repo.insertedLogs[len(repo.insertedLogs)-1].OperationType != "APPLY" {
		t.Fatalf("expected apply log inserted, got %#v", repo.insertedLogs)
	}
	if cache.bumpCount != 0 {
		t.Fatalf("legacy cache invalidation must not run after apply, got %d", cache.bumpCount)
	}
	if repo.listConfigsCalls != 1 || repo.findConfigCalls != 0 || repo.findChangeLogCalls != 0 ||
		repo.applyPendingBatchCalls != 1 || repo.claimPendingCalls != 0 {
		t.Fatalf("unexpected apply query shape: listConfigs=%d findConfig=%d findLog=%d batch=%d claims=%d",
			repo.listConfigsCalls, repo.findConfigCalls, repo.findChangeLogCalls, repo.applyPendingBatchCalls, repo.claimPendingCalls)
	}
}

func TestApplyPendingConfigsUsesHigherLogIDWhenTimestampsMatch(t *testing.T) {
	base := time.Now().UTC().Truncate(time.Second)
	repo := &fakeRepository{
		configsByID: map[int64]*domain.Config{
			22: {ID: 22, GroupID: 1, GroupCode: "app", GroupName: "App", ConfigKey: "theme", ConfigValue: "old", ValueType: "string", EffectType: "restart", IsEnabled: 1},
		},
		changeLogsByID: map[int64]*domain.ConfigChangeLog{
			401: {ID: 401, ConfigID: 22, ConfigKey: "theme", OperationType: "CREATE", NewValue: "old", EffectType: "restart", Status: "pending", OperationTime: ptrTime(base)},
			402: {ID: 402, ConfigID: 22, ConfigKey: "theme", OperationType: "UPDATE", NewValue: "new", EffectType: "restart", Status: "pending", OperationTime: ptrTime(base)},
		},
		pendingLogs: []domain.ConfigChangeLog{
			{ID: 401, ConfigID: 22, ConfigKey: "theme", OperationType: "CREATE", NewValue: "old", EffectType: "restart", Status: "pending", OperationTime: ptrTime(base)},
			{ID: 402, ConfigID: 22, ConfigKey: "theme", OperationType: "UPDATE", NewValue: "new", EffectType: "restart", Status: "pending", OperationTime: ptrTime(base)},
		},
	}
	service := NewService(nil, repo, &fakeCacheStore{}, domain.NewService(), &fakeSecretCipher{}, &fakeRevealCipher{}, &fakeUserLookup{})

	count, err := service.ApplyPendingConfigs(context.Background(), Actor{UserID: 1001, Username: "admin", IsAdmin: true, StepUpProof: validConfigStepUpProof("CONFIG_APPLY_PENDING", "config:apply-pending")}, false)
	if err != nil {
		t.Fatalf("apply pending configs: %v", err)
	}
	if count != 1 {
		t.Fatalf("expected one applied config, got %d", count)
	}
	if got := repo.configsByID[22].ConfigValue; got != "new" {
		t.Fatalf("expected highest log id pending value applied, got %q", got)
	}
	if repo.changeLogsByID[402].Status != "applied" {
		t.Fatalf("expected latest pending log applied, got %#v", repo.changeLogsByID[402])
	}
}

func TestApplyPendingConfigsSkipsLogClaimedByConcurrentWorker(t *testing.T) {
	base := time.Now().UTC()
	repo := &fakeRepository{
		configsByID: map[int64]*domain.Config{
			23: {ID: 23, GroupID: 1, GroupCode: "app", ConfigKey: "theme", ConfigValue: "old", ValueType: "string", Version: 1},
		},
		changeLogsByID: map[int64]*domain.ConfigChangeLog{
			403: {ID: 403, ConfigID: 23, ConfigKey: "theme", NewValue: "new", Status: "applied", OperationTime: ptrTime(base)},
		},
		pendingLogs: []domain.ConfigChangeLog{
			{ID: 403, ConfigID: 23, ConfigKey: "theme", NewValue: "new", Status: "pending", OperationTime: ptrTime(base)},
		},
	}
	service := NewService(nil, repo, &fakeCacheStore{}, domain.NewService(), &fakeSecretCipher{}, &fakeRevealCipher{}, &fakeUserLookup{})

	count, err := service.ApplyPendingConfigs(context.Background(), Actor{UserID: 1001, IsAdmin: true, StepUpProof: validConfigStepUpProof("CONFIG_APPLY_PENDING", "config:apply-pending")}, false)
	if err != nil {
		t.Fatalf("apply pending configs: %v", err)
	}
	if count != 0 || repo.configsByID[23].ConfigValue != "old" || len(repo.insertedLogs) != 0 {
		t.Fatalf("concurrently claimed log was applied again: count=%d config=%#v logs=%#v", count, repo.configsByID[23], repo.insertedLogs)
	}
}

func TestApplyPendingConfigsDoesNotDependOnLegacyPostCommitCacheInvalidation(t *testing.T) {
	base := time.Now().UTC()
	repo := &fakeRepository{
		configsByID: map[int64]*domain.Config{
			24: {ID: 24, GroupID: 1, GroupCode: "app", ConfigKey: "theme", ConfigValue: "old", ValueType: "string", Version: 1},
		},
		changeLogsByID: map[int64]*domain.ConfigChangeLog{
			404: {ID: 404, ConfigID: 24, ConfigKey: "theme", NewValue: "new", Status: "pending", OperationTime: ptrTime(base)},
		},
		pendingLogs: []domain.ConfigChangeLog{
			{ID: 404, ConfigID: 24, ConfigKey: "theme", NewValue: "new", Status: "pending", OperationTime: ptrTime(base)},
		},
	}
	cacheErr := errors.New("cache unavailable")
	cache := &fakeCacheStore{invalidateBatchErr: cacheErr}
	service := NewService(nil, repo, cache, domain.NewService(), &fakeSecretCipher{}, &fakeRevealCipher{}, &fakeUserLookup{})
	actor := Actor{UserID: 1001, IsAdmin: true, StepUpProof: validConfigStepUpProof("CONFIG_APPLY_PENDING", "config:apply-pending")}

	count, err := service.ApplyPendingConfigs(context.Background(), actor, false)
	if count != 1 || err != nil {
		t.Fatalf("committed batch must not depend on legacy cache invalidation: count=%d err=%v", count, err)
	}
	if repo.configsByID[24].ConfigValue != "new" || repo.changeLogsByID[404].Status != "applied" {
		t.Fatalf("database mutation did not remain committed: config=%#v log=%#v", repo.configsByID[24], repo.changeLogsByID[404])
	}
	retryCount, retryErr := service.ApplyPendingConfigs(context.Background(), actor, false)
	if retryErr != nil || retryCount != 0 {
		t.Fatalf("retry after cache failure must be idempotent: count=%d err=%v", retryCount, retryErr)
	}
}

func TestRollbackConfigChangeRejectsWhenCurrentValueDrifted(t *testing.T) {
	base := time.Now().UTC()
	repo := &fakeRepository{
		configsByID: map[int64]*domain.Config{
			31: {ID: 31, GroupID: 1, GroupCode: "app", GroupName: "App", ConfigKey: "color", ConfigValue: "newer", ValueType: "string", EffectType: "realtime", IsEnabled: 1},
		},
		changeLogsByID: map[int64]*domain.ConfigChangeLog{
			401: {ID: 401, ConfigID: 31, ConfigKey: "color", OperationType: "UPDATE", OldValue: "old", NewValue: "new", EffectType: "realtime", Status: "applied", OperationTime: ptrTime(base)},
		},
	}
	service := NewService(nil, repo, &fakeCacheStore{}, domain.NewService(), &fakeSecretCipher{}, &fakeRevealCipher{}, &fakeUserLookup{})

	err := service.RollbackConfigChange(context.Background(), Actor{UserID: 1001, IsAdmin: true, StepUpProof: validConfigStepUpProof("CONFIG_ROLLBACK", "config:rollback:401")}, 401, "test rollback")
	appErr := apperrors.From(err)
	if appErr == nil || appErr.Code() != apperrors.CodeParamsError {
		t.Fatalf("expected params error for drifted current value, got %v", err)
	}
}

func TestRollbackConfigAssetChangeRestoresPrivatePriorBinding(t *testing.T) {
	const configID int64 = 910
	const logID int64 = 500
	stablePath := filefacade.ConfigAssetStablePath(configID)
	oldSnapshot := assetSnapshotForTest(configID, filefacade.ConfigAssetBindingBound, 101)
	newSnapshot := assetSnapshotForTest(configID, filefacade.ConfigAssetBindingBound, 202)
	changeLog := &domain.ConfigChangeLog{
		ID: logID, ConfigID: configID, ConfigKey: "loginLogo", OperationType: "UPDATE",
		OldValue: stablePath, NewValue: stablePath, EffectType: "realtime", Status: "applied",
	}
	if err := changeLog.SetPrivateAssetSnapshots(&oldSnapshot, &newSnapshot); err != nil {
		t.Fatalf("set private asset snapshots: %v", err)
	}
	repo := &fakeRepository{
		configsByID: map[int64]*domain.Config{
			configID: {ID: configID, GroupID: 1, GroupCode: "assets", ConfigKey: "loginLogo", ConfigValue: stablePath, ValueType: "IMAGE", Exposure: "PUBLIC", Sensitivity: "NORMAL", EffectType: "realtime", IsEnabled: 1, Version: 4},
		},
		changeLogsByID: map[int64]*domain.ConfigChangeLog{logID: changeLog},
	}
	assets := &fakeConfigAssetFacade{}
	service := NewService(nil, repo, &fakeCacheStore{}, domain.NewService(), &fakeSecretCipher{}, &fakeRevealCipher{}, &fakeUserLookup{})
	service.BindConfigAssets(assets)
	actor := Actor{UserID: 1001, IsAdmin: true, StepUpProof: validConfigStepUpProof("CONFIG_ROLLBACK", "config:rollback:500")}
	if err := service.RollbackConfigChange(context.Background(), actor, logID, "restore original logo"); err != nil {
		t.Fatalf("rollback CONFIG_ASSET replacement: %v", err)
	}
	if assets.restoreCalls != 1 || assets.restoreCommand == nil {
		t.Fatalf("rollback did not invoke private asset restore exactly once: calls=%d command=%#v", assets.restoreCalls, assets.restoreCommand)
	}
	if got := assets.restoreCommand.Expected; got.State != filefacade.ConfigAssetBindingBound || got.FileID != 202 || got.ConfigID != configID {
		t.Fatalf("rollback expected current B binding, got %#v", got)
	}
	if got := assets.restoreCommand.Restore; got.State != filefacade.ConfigAssetBindingBound || got.FileID != 101 || got.ConfigID != configID {
		t.Fatalf("rollback did not restore original A binding, got %#v", got)
	}
	if got := repo.changeLogsByID[logID].Status; got != string(domain.ConfigStatusRolledBack) {
		t.Fatalf("original change log status=%q, want rolled_back", got)
	}
	if len(repo.insertedLogs) != 1 {
		t.Fatalf("rollback audit logs=%d, want 1", len(repo.insertedLogs))
	}
	rollbackOld, rollbackNew, err := repo.insertedLogs[0].PrivateAssetSnapshots()
	if err != nil || rollbackOld == nil || rollbackNew == nil || rollbackOld.FileID != 202 || rollbackNew.FileID != 101 {
		t.Fatalf("rollback audit snapshots are not reversible: old=%#v new=%#v err=%v", rollbackOld, rollbackNew, err)
	}
	payload, err := json.Marshal(repo.insertedLogs[0])
	if err != nil || strings.Contains(string(payload), "fileId") || strings.Contains(string(payload), "scopeId") {
		t.Fatalf("private asset snapshot leaked from change log JSON: payload=%s err=%v", payload, err)
	}
	history, err := service.GetConfigChangeHistory(context.Background(), Actor{UserID: 1001, IsAdmin: true}, configID, 20)
	if err != nil {
		t.Fatalf("get history: %v", err)
	}
	historyPayload, err := json.Marshal(history)
	if err != nil || strings.Contains(string(historyPayload), "fileId") || strings.Contains(string(historyPayload), "scopeId") {
		t.Fatalf("private asset snapshot leaked from history VO: payload=%s err=%v", historyPayload, err)
	}
}

func TestRollbackConfigAssetClearRestoresPrivatePriorBinding(t *testing.T) {
	const configID int64 = 911
	const logID int64 = 501
	stablePath := filefacade.ConfigAssetStablePath(configID)
	oldSnapshot := assetSnapshotForTest(configID, filefacade.ConfigAssetBindingBound, 101)
	newSnapshot := assetSnapshotForTest(configID, filefacade.ConfigAssetBindingEmpty, 0)
	changeLog := &domain.ConfigChangeLog{ID: logID, ConfigID: configID, ConfigKey: "loginLogo", OperationType: "UPDATE", OldValue: stablePath, NewValue: "", EffectType: "realtime", Status: "applied"}
	if err := changeLog.SetPrivateAssetSnapshots(&oldSnapshot, &newSnapshot); err != nil {
		t.Fatalf("set private clear snapshots: %v", err)
	}
	repo := &fakeRepository{
		configsByID:    map[int64]*domain.Config{configID: {ID: configID, GroupID: 1, GroupCode: "assets", ConfigKey: "loginLogo", ConfigValue: "", ValueType: "IMAGE", Exposure: "PUBLIC", Sensitivity: "NORMAL", EffectType: "realtime", IsEnabled: 1, Version: 5}},
		changeLogsByID: map[int64]*domain.ConfigChangeLog{logID: changeLog},
	}
	assets := &fakeConfigAssetFacade{}
	service := NewService(nil, repo, &fakeCacheStore{}, domain.NewService(), &fakeSecretCipher{}, &fakeRevealCipher{}, &fakeUserLookup{})
	service.BindConfigAssets(assets)
	if err := service.RollbackConfigChange(context.Background(), Actor{UserID: 1001, IsAdmin: true, StepUpProof: validConfigStepUpProof("CONFIG_ROLLBACK", "config:rollback:501")}, logID, "restore cleared logo"); err != nil {
		t.Fatalf("rollback CONFIG_ASSET clear: %v", err)
	}
	if assets.restoreCommand == nil || assets.restoreCommand.Expected.State != filefacade.ConfigAssetBindingEmpty || assets.restoreCommand.Restore.FileID != 101 {
		t.Fatalf("clear rollback did not restore A from empty state: %#v", assets.restoreCommand)
	}
}

func TestRollbackConfigAssetPolicyChangeRestoresPriorExposure(t *testing.T) {
	const configID int64 = 913
	const logID int64 = 504
	stablePath := filefacade.ConfigAssetStablePath(configID)
	oldSnapshot := domain.NewConfigAssetBindingSnapshot(
		configID, string(filefacade.ConfigAssetBindingBound), 101, "org:1",
		string(filefacade.ConfigAssetImage), string(filefacade.ConfigAssetPublic),
	)
	newSnapshot := domain.NewConfigAssetBindingSnapshot(
		configID, string(filefacade.ConfigAssetBindingBound), 101, "org:1",
		string(filefacade.ConfigAssetImage), string(filefacade.ConfigAssetAuthenticated),
	)
	changeLog := &domain.ConfigChangeLog{
		ID: logID, ConfigID: configID, ConfigKey: "loginLogo", OperationType: "UPDATE",
		OldValue: stablePath, NewValue: stablePath, EffectType: "realtime", Status: "applied",
	}
	if err := changeLog.SetPrivateAssetSnapshots(&oldSnapshot, &newSnapshot); err != nil {
		t.Fatalf("set private policy snapshots: %v", err)
	}
	repo := &fakeRepository{
		configsByID: map[int64]*domain.Config{
			configID: {
				ID: configID, GroupID: 1, GroupCode: "assets", ConfigKey: "loginLogo", ConfigValue: stablePath,
				ValueType: "IMAGE", Exposure: "AUTHENTICATED", Sensitivity: "NORMAL", EffectType: "realtime", IsEnabled: 1, Version: 6,
			},
		},
		changeLogsByID: map[int64]*domain.ConfigChangeLog{logID: changeLog},
	}
	assets := &fakeConfigAssetFacade{}
	service := NewService(nil, repo, &fakeCacheStore{}, domain.NewService(), &fakeSecretCipher{}, &fakeRevealCipher{}, &fakeUserLookup{})
	service.BindConfigAssets(assets)
	if err := service.RollbackConfigChange(context.Background(), Actor{
		UserID: 1001, IsAdmin: true, StepUpProof: validConfigStepUpProof("CONFIG_ROLLBACK", "config:rollback:504"),
	}, logID, "restore public policy"); err != nil {
		t.Fatalf("rollback CONFIG_ASSET policy: %v", err)
	}
	if assets.restoreCommand == nil ||
		assets.restoreCommand.Expected.Exposure != filefacade.ConfigAssetAuthenticated ||
		assets.restoreCommand.Restore.Exposure != filefacade.ConfigAssetPublic ||
		assets.restoreCommand.Expected.FileID != assets.restoreCommand.Restore.FileID {
		t.Fatalf("policy rollback did not derive expected/current and prior exposure states: %#v", assets.restoreCommand)
	}
	if got := repo.configsByID[configID].Exposure; got != string(domain.ConfigExposurePublic) {
		t.Fatalf("policy rollback did not restore sys_config exposure: got %q", got)
	}
	if len(repo.insertedLogs) != 1 {
		t.Fatalf("policy rollback audit logs=%d, want 1", len(repo.insertedLogs))
	}
	rollbackOld, rollbackNew, err := repo.insertedLogs[0].PrivateAssetSnapshots()
	if err != nil || rollbackOld == nil || rollbackNew == nil ||
		rollbackOld.Exposure != string(filefacade.ConfigAssetAuthenticated) || rollbackNew.Exposure != string(filefacade.ConfigAssetPublic) {
		t.Fatalf("policy rollback audit snapshots were not reversed: old=%+v new=%+v err=%v", rollbackOld, rollbackNew, err)
	}
}

func TestRollbackConfigAssetRejectsMissingOrMalformedPrivateSnapshotWithoutStateChange(t *testing.T) {
	tests := []struct {
		name      string
		logID     int64
		configure func(*domain.ConfigChangeLog)
	}{
		{
			name:      "legacy-missing-snapshot",
			logID:     502,
			configure: func(*domain.ConfigChangeLog) {},
		},
		{
			name:  "malformed-snapshot",
			logID: 503,
			configure: func(log *domain.ConfigChangeLog) {
				log.HydratePrivateAssetSnapshotPayloads("not-json", "also-not-json")
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			stablePath := filefacade.ConfigAssetStablePath(912)
			changeLog := &domain.ConfigChangeLog{ID: tt.logID, ConfigID: 912, ConfigKey: "loginLogo", OperationType: "UPDATE", OldValue: stablePath, NewValue: stablePath, EffectType: "realtime", Status: "applied"}
			tt.configure(changeLog)
			repo := &fakeRepository{
				configsByID:    map[int64]*domain.Config{912: {ID: 912, GroupID: 1, GroupCode: "assets", ConfigKey: "loginLogo", ConfigValue: stablePath, ValueType: "IMAGE", Exposure: "PUBLIC", Sensitivity: "NORMAL", EffectType: "realtime", IsEnabled: 1, Version: 9}},
				changeLogsByID: map[int64]*domain.ConfigChangeLog{tt.logID: changeLog},
			}
			assets := &fakeConfigAssetFacade{}
			service := NewService(nil, repo, &fakeCacheStore{}, domain.NewService(), &fakeSecretCipher{}, &fakeRevealCipher{}, &fakeUserLookup{})
			service.BindConfigAssets(assets)
			err := service.RollbackConfigChange(context.Background(), Actor{UserID: 1001, IsAdmin: true, StepUpProof: validConfigStepUpProof("CONFIG_ROLLBACK", "config:rollback:"+strconv.FormatInt(tt.logID, 10))}, tt.logID, "legacy rollback")
			if err == nil {
				t.Fatal("legacy CONFIG_ASSET rollback unexpectedly succeeded")
			}
			if assets.restoreCalls != 0 || repo.changeLogsByID[tt.logID].Status != string(domain.ConfigStatusApplied) || repo.configsByID[912].Version != 9 {
				t.Fatalf("legacy rollback changed state: restores=%d log=%#v config=%#v", assets.restoreCalls, repo.changeLogsByID[tt.logID], repo.configsByID[912])
			}
		})
	}
}

func assetSnapshotForTest(configID int64, state filefacade.ConfigAssetBindingKind, fileID int64) domain.ConfigAssetBindingSnapshot {
	return domain.NewConfigAssetBindingSnapshot(configID, string(state), fileID, "org:1", string(filefacade.ConfigAssetImage), string(filefacade.ConfigAssetPublic))
}

func TestChangeEnabledAcceptsZeroValue(t *testing.T) {
	repo := &fakeRepository{
		configsByID: map[int64]*domain.Config{
			51: {ID: 51, GroupID: 1, GroupCode: "app", GroupName: "App", ConfigKey: "banner", ConfigValue: "hello", ValueType: "string", IsEnabled: 1},
		},
	}
	service := NewService(nil, repo, &fakeCacheStore{}, domain.NewService(), &fakeSecretCipher{}, &fakeRevealCipher{}, &fakeUserLookup{})
	disabled := 0

	if err := service.ChangeEnabled(context.Background(), Actor{UserID: 1001, IsAdmin: true}, 51, configfacade.ConfigEnabledRequest{IsEnabled: &disabled}); err != nil {
		t.Fatalf("change enabled: %v", err)
	}
	if got := repo.configsByID[51].IsEnabled; got != 0 {
		t.Fatalf("expected config disabled, got %d", got)
	}
}

func TestGetPendingConfigsMasksSensitiveValues(t *testing.T) {
	base := time.Now().UTC()
	repo := &fakeRepository{
		configsByID: map[int64]*domain.Config{
			61: {
				ID:          61,
				GroupID:     1,
				GroupCode:   "app",
				GroupName:   "App",
				ConfigKey:   "secret",
				ConfigValue: "",
				ValueType:   "string",
				IsSensitive: 1,
				IsEnabled:   1,
				ExtJSON: &domain.ConfigExtJSON{Secret: &domain.ConfigSecretValue{
					Plain: "runtime-plain",
				}},
			},
		},
		pendingLogs: []domain.ConfigChangeLog{
			{ID: 601, ConfigID: 61, ConfigKey: "secret", OperationType: "UPDATE", NewValue: "pending-plain", EffectType: "restart", Status: "pending", OperationTime: ptrTime(base)},
		},
		configScopeGrants: []domain.ConfigScopeGrant{
			{RoleID: 501, GroupCode: "app", CanRead: 1},
		},
	}
	service := NewService(nil, repo, &fakeCacheStore{}, domain.NewService(), &fakeSecretCipher{}, &fakeRevealCipher{}, &fakeUserLookup{})

	items, err := service.GetPendingConfigs(context.Background(), Actor{UserID: 1001, RoleIDs: []int64{501}})
	if err != nil {
		t.Fatalf("get pending configs: %v", err)
	}
	if len(items) != 1 {
		t.Fatalf("expected one pending config, got %d", len(items))
	}
	if items[0].CurrentValue != "******" || items[0].PendingValue != "******" {
		t.Fatalf("expected masked pending values, got %#v", items[0])
	}
}

func TestGetConfigChangeHistoryMasksSensitiveValues(t *testing.T) {
	base := time.Now().UTC()
	repo := &fakeRepository{
		configsByID: map[int64]*domain.Config{
			71: {ID: 71, GroupID: 1, GroupCode: "app", GroupName: "App", ConfigKey: "secret", IsSensitive: 1},
		},
		changeLogsByID: map[int64]*domain.ConfigChangeLog{
			701: {ID: 701, ConfigID: 71, ConfigKey: "secret", OperationType: "UPDATE", OldValue: "old-secret", NewValue: "new-secret", EffectType: "realtime", Status: "applied", OperationTime: ptrTime(base)},
		},
		configScopeGrants: []domain.ConfigScopeGrant{
			{RoleID: 501, GroupCode: "app", CanRead: 1},
		},
	}
	service := NewService(nil, repo, &fakeCacheStore{}, domain.NewService(), &fakeSecretCipher{}, &fakeRevealCipher{}, &fakeUserLookup{})

	items, err := service.GetConfigChangeHistory(context.Background(), Actor{UserID: 1001, RoleIDs: []int64{501}}, 71, 20)
	if err != nil {
		t.Fatalf("get config change history: %v", err)
	}
	if len(items) != 1 {
		t.Fatalf("expected one history record, got %d", len(items))
	}
	if items[0].OldValue != "******" || items[0].NewValue != "******" {
		t.Fatalf("expected masked history values, got %#v", items[0])
	}
}

func TestGetConfigChangeHistoryMasksHistoricalSensitiveValuesWithoutPermission(t *testing.T) {
	base := time.Now().UTC()
	repo := &fakeRepository{
		configsByID: map[int64]*domain.Config{
			72: {ID: 72, GroupID: 1, GroupCode: "app", GroupName: "App", ConfigKey: "secret", IsSensitive: 0},
		},
		changeLogsByID: map[int64]*domain.ConfigChangeLog{
			702: {ID: 702, ConfigID: 72, ConfigKey: "secret", OperationType: "UPDATE", OldValue: "old-secret", NewValue: "new-secret", EffectType: "realtime", Status: "applied", OperationTime: ptrTime(base)},
		},
		configScopeGrants: []domain.ConfigScopeGrant{
			{RoleID: 501, GroupCode: "app", CanRead: 1},
		},
	}
	service := NewService(nil, repo, &fakeCacheStore{}, domain.NewService(), &fakeSecretCipher{}, &fakeRevealCipher{}, &fakeUserLookup{})

	items, err := service.GetConfigChangeHistory(context.Background(), Actor{UserID: 1001, RoleIDs: []int64{501}}, 72, 20)
	if err != nil {
		t.Fatalf("get config change history: %v", err)
	}
	if len(items) != 1 {
		t.Fatalf("expected one history record, got %d", len(items))
	}
	if items[0].OldValue != "******" || items[0].NewValue != "******" {
		t.Fatalf("expected masked history values for query-only reader, got %#v", items[0])
	}
}

type fakeRepository struct {
	groupsByID                map[int64]*domain.ConfigGroup
	groupsByCode              map[string]*domain.ConfigGroup
	configsByID               map[int64]*domain.Config
	configsByGroupKey         map[string]*domain.Config
	rawConfigs                map[string][]domain.Config
	configKeyCount            map[string]int64
	pendingLogs               []domain.ConfigChangeLog
	changeLogsByID            map[int64]*domain.ConfigChangeLog
	configScopeGrants         []domain.ConfigScopeGrant
	replacedConfigScope       []domain.ConfigScopeGrant
	insertedLogs              []domain.ConfigChangeLog
	lastGroupQuery            domain.ConfigGroupPageQuery
	lastConfigQuery           domain.ConfigPageQuery
	nextConfigID              int64
	nextGroupID               int64
	findConfigCalls           int
	findChangeLogCalls        int
	listConfigsCalls          int
	claimPendingCalls         int
	applyPendingBatchCalls    int
	scopeGrantListCalls       int
	findGroupByCodeCalls      int
	findConfigByGroupKeyCalls int
	listGroupsByCodesCalls    int
	listConfigsByRefsCalls    int
	queryGroupsCalls          int
	queryConfigsCalls         int
}

func (f *fakeRepository) FindGroupByID(_ context.Context, id int64) (*domain.ConfigGroup, error) {
	if f.groupsByID == nil {
		return nil, nil
	}
	item := f.groupsByID[id]
	if item == nil {
		return nil, nil
	}
	copyItem := *item
	return &copyItem, nil
}

func (f *fakeRepository) FindGroupByCode(_ context.Context, groupCode string) (*domain.ConfigGroup, error) {
	f.findGroupByCodeCalls++
	item := f.groupsByCode[groupCode]
	if item == nil {
		for _, candidate := range f.groupsByID {
			if candidate != nil && candidate.GroupCode == groupCode {
				item = candidate
				break
			}
		}
	}
	if item == nil {
		return nil, nil
	}
	copyItem := *item
	return &copyItem, nil
}

func (f *fakeRepository) ListGroupsByCodes(_ context.Context, groupCodes []string) ([]domain.ConfigGroup, error) {
	f.listGroupsByCodesCalls++
	result := make([]domain.ConfigGroup, 0, len(groupCodes))
	for _, code := range groupCodes {
		if item := f.groupsByCode[code]; item != nil {
			result = append(result, *item)
			continue
		}
		for _, candidate := range f.groupsByID {
			if candidate != nil && candidate.GroupCode == code {
				result = append(result, *candidate)
				break
			}
		}
	}
	return result, nil
}

func (f *fakeRepository) CountGroupByCode(context.Context, string, int64) (int64, error) {
	return 0, nil
}
func (f *fakeRepository) InsertGroup(_ context.Context, item *domain.ConfigGroup) (int64, error) {
	f.nextGroupID++
	if f.nextGroupID == 0 {
		f.nextGroupID = 100
	}
	copyItem := *item
	copyItem.ID = f.nextGroupID
	if f.groupsByID == nil {
		f.groupsByID = map[int64]*domain.ConfigGroup{}
	}
	f.groupsByID[copyItem.ID] = &copyItem
	return copyItem.ID, nil
}
func (f *fakeRepository) UpdateGroup(_ context.Context, item *domain.ConfigGroup) error {
	copyItem := *item
	if f.groupsByID == nil {
		f.groupsByID = map[int64]*domain.ConfigGroup{}
	}
	f.groupsByID[item.ID] = &copyItem
	return nil
}
func (f *fakeRepository) QueryGroups(_ context.Context, query domain.ConfigGroupPageQuery) (*domain.ConfigGroupPage, error) {
	f.queryGroupsCalls++
	f.lastGroupQuery = query
	records := make([]domain.ConfigGroup, 0, len(f.groupsByID))
	for _, item := range f.groupsByID {
		if item == nil || item.IsDeleted == 1 {
			continue
		}
		copyItem := *item
		records = append(records, copyItem)
	}
	return &domain.ConfigGroupPage{Current: query.Current, Size: query.PageSize, Total: int64(len(records)), Records: records}, nil
}
func (f *fakeRepository) CountConfigsByGroupID(_ context.Context, groupID int64) (int64, error) {
	var count int64
	for _, item := range f.configsByID {
		if item != nil && item.GroupID == groupID && item.IsDeleted == 0 {
			count++
		}
	}
	return count, nil
}
func (f *fakeRepository) ShiftGroupSort(context.Context, int64, int, int) error { return nil }

func (f *fakeRepository) FindConfigByID(_ context.Context, id int64) (*domain.Config, error) {
	f.findConfigCalls++
	if f.configsByID == nil {
		return nil, nil
	}
	item := f.configsByID[id]
	if item == nil {
		return nil, nil
	}
	copyItem := *item
	if item.ExtJSON != nil {
		copyItem.ExtJSON = item.ExtJSON.Copy()
	}
	return &copyItem, nil
}

func (f *fakeRepository) FindConfigByGroupAndKey(_ context.Context, groupID int64, configKey string, _ bool) (*domain.Config, error) {
	f.findConfigByGroupKeyCalls++
	item := f.configsByGroupKey[groupKey(groupID, configKey)]
	if item == nil {
		for _, candidate := range f.configsByID {
			if candidate != nil && candidate.GroupID == groupID && candidate.ConfigKey == configKey {
				item = candidate
				break
			}
		}
	}
	if item == nil {
		return nil, nil
	}
	copyItem := *item
	if item.ExtJSON != nil {
		copyItem.ExtJSON = item.ExtJSON.Copy()
	}
	return &copyItem, nil
}

func (f *fakeRepository) ListConfigsByGroupAndKeys(_ context.Context, refs []domain.ConfigKeyRef) ([]domain.Config, error) {
	f.listConfigsByRefsCalls++
	result := make([]domain.Config, 0, len(refs))
	for _, ref := range refs {
		if item := f.configsByGroupKey[groupKey(ref.GroupID, ref.ConfigKey)]; item != nil {
			result = append(result, *item)
			continue
		}
		for _, candidate := range f.configsByID {
			if candidate != nil && candidate.GroupID == ref.GroupID && candidate.ConfigKey == ref.ConfigKey {
				result = append(result, *candidate)
				break
			}
		}
	}
	return result, nil
}

func (f *fakeRepository) FindConfigsByRawKey(_ context.Context, configKey string, _ bool) ([]domain.Config, error) {
	return append([]domain.Config(nil), f.rawConfigs[configKey]...), nil
}

func (f *fakeRepository) CountConfigByGroupAndKey(_ context.Context, groupID int64, configKey string, _ int64) (int64, error) {
	if f.configKeyCount == nil {
		return 0, nil
	}
	return f.configKeyCount[groupKey(groupID, configKey)], nil
}

func (f *fakeRepository) InsertConfig(_ context.Context, item *domain.Config) (int64, error) {
	f.nextConfigID++
	if f.nextConfigID == 0 {
		f.nextConfigID = 200
	}
	copyItem := *item
	copyItem.ID = f.nextConfigID
	if f.configsByID == nil {
		f.configsByID = map[int64]*domain.Config{}
	}
	if f.configsByGroupKey == nil {
		f.configsByGroupKey = map[string]*domain.Config{}
	}
	f.configsByID[copyItem.ID] = &copyItem
	f.configsByGroupKey[groupKey(copyItem.GroupID, copyItem.ConfigKey)] = &copyItem
	return copyItem.ID, nil
}

func (f *fakeRepository) UpdateConfig(_ context.Context, item *domain.Config) error {
	copyItem := *item
	if f.configsByID == nil {
		f.configsByID = map[int64]*domain.Config{}
	}
	if f.configsByGroupKey == nil {
		f.configsByGroupKey = map[string]*domain.Config{}
	}
	f.configsByID[item.ID] = &copyItem
	f.configsByGroupKey[groupKey(item.GroupID, item.ConfigKey)] = &copyItem
	return nil
}

func (f *fakeRepository) QueryConfigs(_ context.Context, query domain.ConfigPageQuery) (*domain.ConfigPage, error) {
	f.queryConfigsCalls++
	f.lastConfigQuery = query
	records := make([]domain.Config, 0, len(f.configsByID))
	for _, item := range f.configsByID {
		if item == nil || item.IsDeleted == 1 {
			continue
		}
		copyItem := *item
		if item.ExtJSON != nil {
			copyItem.ExtJSON = item.ExtJSON.Copy()
		}
		records = append(records, copyItem)
	}
	return &domain.ConfigPage{Current: query.Current, Size: query.PageSize, Total: int64(len(records)), Records: records}, nil
}

func (f *fakeRepository) ListConfigsByIDs(_ context.Context, ids []int64) ([]domain.Config, error) {
	f.listConfigsCalls++
	result := make([]domain.Config, 0, len(ids))
	for _, id := range ids {
		if item := f.configsByID[id]; item != nil {
			copyItem := *item
			result = append(result, copyItem)
		}
	}
	return result, nil
}

func (f *fakeRepository) InsertChangeLog(_ context.Context, item *domain.ConfigChangeLog) (int64, error) {
	copyItem := *item
	if f.changeLogsByID == nil {
		f.changeLogsByID = map[int64]*domain.ConfigChangeLog{}
	}
	if copyItem.ID == 0 {
		copyItem.ID = int64(len(f.insertedLogs) + 500)
		for f.changeLogsByID[copyItem.ID] != nil {
			copyItem.ID++
		}
	}
	f.insertedLogs = append(f.insertedLogs, copyItem)
	f.changeLogsByID[copyItem.ID] = &copyItem
	return copyItem.ID, nil
}

func (f *fakeRepository) UpdateChangeLog(_ context.Context, item *domain.ConfigChangeLog) error {
	copyItem := *item
	if f.changeLogsByID == nil {
		f.changeLogsByID = map[int64]*domain.ConfigChangeLog{}
	}
	f.changeLogsByID[item.ID] = &copyItem
	return nil
}

func (f *fakeRepository) ClaimPendingChangeLog(_ context.Context, id int64, appliedBy int64, appliedTime time.Time, operatorName string) (bool, error) {
	f.claimPendingCalls++
	if f.changeLogsByID == nil {
		return false, nil
	}
	item := f.changeLogsByID[id]
	if item == nil || item.Status != string(domain.ConfigStatusPending) {
		return false, nil
	}
	item.Status = string(domain.ConfigStatusApplied)
	item.AppliedBy = &appliedBy
	item.AppliedTime = &appliedTime
	item.OperatorName = operatorName
	return true, nil
}

func (f *fakeRepository) ApplyPendingConfigBatch(_ context.Context, items []domain.PendingConfigApply) ([]int64, error) {
	f.applyPendingBatchCalls++
	applied := make([]int64, 0, len(items))
	for _, item := range items {
		log := f.changeLogsByID[item.PendingLogID]
		if log == nil || log.Status != string(domain.ConfigStatusPending) {
			continue
		}
		log.Status = string(domain.ConfigStatusApplied)
		configCopy := item.Config
		configCopy.Version++
		f.configsByID[configCopy.ID] = &configCopy
		auditCopy := item.ApplyLog
		if auditCopy.ID == 0 {
			auditCopy.ID = int64(len(f.insertedLogs) + 500)
		}
		f.insertedLogs = append(f.insertedLogs, auditCopy)
		f.changeLogsByID[auditCopy.ID] = &auditCopy
		applied = append(applied, item.PendingLogID)
	}
	return applied, nil
}

func (f *fakeRepository) FindChangeLogByID(_ context.Context, id int64) (*domain.ConfigChangeLog, error) {
	f.findChangeLogCalls++
	if f.changeLogsByID == nil {
		return nil, nil
	}
	item := f.changeLogsByID[id]
	if item == nil {
		return nil, nil
	}
	copyItem := *item
	return &copyItem, nil
}

func (f *fakeRepository) ListPendingLogs(context.Context) ([]domain.ConfigChangeLog, error) {
	return append([]domain.ConfigChangeLog(nil), f.pendingLogs...), nil
}

func (f *fakeRepository) ListHistoryByConfigID(_ context.Context, configID int64, limit int) ([]domain.ConfigChangeLog, error) {
	result := make([]domain.ConfigChangeLog, 0)
	for _, item := range f.changeLogsByID {
		if item != nil && item.ConfigID == configID {
			result = append(result, *item)
		}
	}
	return result, nil
}

func (f *fakeRepository) ListAuditLogs(context.Context, domain.AuditLogQuery) ([]domain.ConfigChangeLog, error) {
	result := make([]domain.ConfigChangeLog, 0, len(f.changeLogsByID))
	for _, item := range f.changeLogsByID {
		if item != nil {
			result = append(result, *item)
		}
	}
	return result, nil
}

func (f *fakeRepository) ListChangeLogsByIDs(_ context.Context, ids []int64) ([]domain.ConfigChangeLog, error) {
	result := make([]domain.ConfigChangeLog, 0, len(ids))
	for _, id := range ids {
		if item := f.changeLogsByID[id]; item != nil {
			result = append(result, *item)
		}
	}
	return result, nil
}

func (f *fakeRepository) ListChangeLogsReferencing(_ context.Context, ids []int64) ([]domain.ConfigChangeLog, error) {
	result := make([]domain.ConfigChangeLog, 0)
	for _, item := range f.changeLogsByID {
		if item == nil {
			continue
		}
		for _, id := range ids {
			if (item.ParentLogID != nil && *item.ParentLogID == id) || (item.RelatedLogID != nil && *item.RelatedLogID == id) {
				result = append(result, *item)
				break
			}
		}
	}
	return result, nil
}

func (f *fakeRepository) ListConfigScopeGrantsByRoleIDs(_ context.Context, roleIDs []int64) ([]domain.ConfigScopeGrant, error) {
	f.scopeGrantListCalls++
	allowed := map[int64]struct{}{}
	for _, id := range roleIDs {
		allowed[id] = struct{}{}
	}
	result := make([]domain.ConfigScopeGrant, 0)
	for _, item := range f.configScopeGrants {
		if _, ok := allowed[item.RoleID]; ok {
			result = append(result, item)
		}
	}
	return result, nil
}

func (f *fakeRepository) ListConfigScopeGrantsByRoleID(_ context.Context, roleID int64) ([]domain.ConfigScopeGrant, error) {
	return f.ListConfigScopeGrantsByRoleIDs(context.Background(), []int64{roleID})
}

func (f *fakeRepository) ReplaceRoleConfigScopes(_ context.Context, roleID int64, grants []domain.ConfigScopeGrant, _ int64, _ func() int64) error {
	f.replacedConfigScope = append([]domain.ConfigScopeGrant(nil), grants...)
	next := make([]domain.ConfigScopeGrant, 0, len(f.configScopeGrants)+len(grants))
	for _, item := range f.configScopeGrants {
		if item.RoleID != roleID {
			next = append(next, item)
		}
	}
	next = append(next, grants...)
	f.configScopeGrants = next
	return nil
}

type fakeGovernedTransactor struct {
	ordinaryCalls   int
	snapshotCalls   int
	consistentCalls int
}

func (f *fakeGovernedTransactor) Enabled() bool { return f != nil }

func (f *fakeGovernedTransactor) WithinTransaction(context.Context, func(context.Context) error) error {
	f.ordinaryCalls++
	return errors.New("ordinary transaction is not valid for governed composed reads or writes")
}

func (f *fakeGovernedTransactor) WithinReadOnlySnapshot(ctx context.Context, fn func(context.Context) error) error {
	f.snapshotCalls++
	return fn(ctx)
}

func (f *fakeGovernedTransactor) WithinConsistentTransaction(ctx context.Context, fn func(context.Context) error) error {
	f.consistentCalls++
	return fn(ctx)
}

type fakeConfigAssetFacade struct {
	bindCalls      int
	clearCalls     int
	captureCalls   int
	restoreCalls   int
	captureStates  []filefacade.ConfigAssetBindingState
	restoreCommand *filefacade.RestoreConfigAssetBindingCommand
	restoreErr     error
	openCalls      int
	openResult     *filefacade.ConfigAssetOpenResult
}

func (f *fakeConfigAssetFacade) BindConfigAsset(context.Context, filefacade.BindConfigAssetCommand) error {
	f.bindCalls++
	return nil
}

func (*fakeConfigAssetFacade) UpdateConfigAssetPolicy(context.Context, filefacade.UpdateConfigAssetPolicyCommand) error {
	return nil
}

func (f *fakeConfigAssetFacade) ClearConfigAsset(context.Context, int64) error {
	f.clearCalls++
	return nil
}

func (f *fakeConfigAssetFacade) CaptureConfigAssetBinding(_ context.Context, command filefacade.CaptureConfigAssetBindingCommand) (filefacade.ConfigAssetBindingState, error) {
	state := filefacade.ConfigAssetBindingState{
		ConfigID: command.ConfigID, State: filefacade.ConfigAssetBindingEmpty, ScopeID: "org:1",
		AssetType: command.AssetType, Exposure: command.Exposure,
	}
	if f.captureCalls < len(f.captureStates) {
		state = f.captureStates[f.captureCalls]
	}
	f.captureCalls++
	return state, nil
}

func (f *fakeConfigAssetFacade) RestoreConfigAssetBinding(_ context.Context, command filefacade.RestoreConfigAssetBindingCommand) error {
	f.restoreCalls++
	copyCommand := command
	f.restoreCommand = &copyCommand
	return f.restoreErr
}

func (f *fakeConfigAssetFacade) OpenConfigAsset(context.Context, int64) (*filefacade.ConfigAssetOpenResult, error) {
	f.openCalls++
	return f.openResult, nil
}

type fakeCacheStore struct {
	bumpCount          int
	lastSetConfigKey   string
	lastBatchKey       string
	invalidateBatchErr error
}

func (f *fakeCacheStore) GetConfigByKey(context.Context, string) (*domain.Config, bool, error) {
	return nil, false, nil
}
func (f *fakeCacheStore) SetConfigByKey(_ context.Context, key string, _ *domain.Config) error {
	f.lastSetConfigKey = key
	return nil
}
func (f *fakeCacheStore) GetGroupByCode(context.Context, string) (*domain.ConfigGroup, bool, error) {
	return nil, false, nil
}
func (f *fakeCacheStore) SetGroupByCode(context.Context, string, *domain.ConfigGroup) error {
	return nil
}
func (f *fakeCacheStore) GetListByGroup(context.Context, int64) ([]domain.Config, bool, error) {
	return nil, false, nil
}
func (f *fakeCacheStore) SetListByGroup(context.Context, int64, []domain.Config) error { return nil }
func (f *fakeCacheStore) GetBatch(context.Context, string) (map[string]domain.Config, bool, error) {
	return nil, false, nil
}
func (f *fakeCacheStore) SetBatch(_ context.Context, key string, _ map[string]domain.Config) error {
	f.lastBatchKey = key
	return nil
}
func (f *fakeCacheStore) CurrentBatchVersion(context.Context) (int64, error) { return 0, nil }
func (f *fakeCacheStore) BumpBatchVersion(context.Context) error {
	f.bumpCount++
	return nil
}
func (f *fakeCacheStore) InvalidateConfig(context.Context, string) error   { return nil }
func (f *fakeCacheStore) InvalidateGroup(context.Context, string) error    { return nil }
func (f *fakeCacheStore) InvalidateGroupList(context.Context, int64) error { return nil }
func (f *fakeCacheStore) InvalidateConfigBatch(context.Context, []domain.Config) error {
	f.bumpCount++
	return f.invalidateBatchErr
}

// sonicClassifiedCacheStore deliberately mirrors the serialization boundary
// of the real governed cache: the test would fail if the application cached a
// ConfigValueDTO with Value any rather than a typed cache record.
type sonicClassifiedCacheStore struct {
	fakeCacheStore
	entries map[string][]byte
	loads   int
}

func (*sonicClassifiedCacheStore) ClassifiedEnabled() bool { return true }

func (f *sonicClassifiedCacheStore) GetOrLoadClassified(ctx context.Context, request cachepolicy.ReadRequest, dest any, loader func(context.Context) (cachepolicy.CacheableValue, error)) (bool, error) {
	if f.entries == nil {
		f.entries = make(map[string][]byte)
	}
	cacheKey := request.KeyMaterial()
	if payload, ok := f.entries[cacheKey]; ok {
		if err := sonic.Unmarshal(payload, dest); err != nil {
			return false, err
		}
		return true, nil
	}
	f.loads++
	loaded, err := loader(ctx)
	if err != nil || loaded.Value == nil {
		return false, err
	}
	payload, err := sonic.Marshal(loaded.Value)
	if err != nil {
		return false, err
	}
	if loaded.Cacheable {
		f.entries[cacheKey] = payload
	}
	if err := sonic.Unmarshal(payload, dest); err != nil {
		return false, err
	}
	return true, nil
}

type fakeSecretCipher struct{}

func (f *fakeSecretCipher) EncryptString(_ context.Context, plain string) (domain.ConfigSecretValue, error) {
	return domain.ConfigSecretValue{
		Plain:         plain,
		CiphertextB64: "cipher:" + plain,
		EDEKB64:       "edek",
		WrapKeyRef:    "key-1",
	}, nil
}

func (f *fakeSecretCipher) DecryptString(_ context.Context, value domain.ConfigSecretValue) (string, error) {
	if value.Plain != "" {
		return value.Plain, nil
	}
	return strings.TrimPrefix(value.CiphertextB64, "cipher:"), nil
}

type fakeUserLookup struct{}

func (f *fakeUserLookup) FindNicknames(_ context.Context, userIDs []int64) (map[int64]string, error) {
	result := make(map[int64]string, len(userIDs))
	for _, userID := range userIDs {
		if userID > 0 {
			result[userID] = "user-" + strconv.FormatInt(userID, 10)
		}
	}
	return result, nil
}

type fakeRevealCipher struct{}

func (f *fakeRevealCipher) EncryptForClient(_ string, plain string) (string, error) {
	return "enc:" + plain, nil
}

func groupKey(groupID int64, configKey string) string {
	return strconv.FormatInt(groupID, 10) + "|" + configKey
}

func ptrTime(value time.Time) *time.Time {
	return &value
}
