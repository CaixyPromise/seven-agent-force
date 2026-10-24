package domain

import "testing"

func TestNotificationBindingGrantSetCannotAcquireLogin(t *testing.T) {
	_, err := NormalizeRequestedGrants([]EnterpriseGrantType{
		EnterpriseGrantIdentityLink,
		EnterpriseGrantNotificationDirect,
		EnterpriseGrantNotificationMention,
		EnterpriseGrantLogin,
	}, AuthorizationPurposeNotificationBind)
	if err == nil {
		t.Fatal("notification binding must never acquire LOGIN")
	}
}

func TestNotificationBindingRequiresIndependentDirectAndMentionGrants(t *testing.T) {
	_, err := NormalizeRequestedGrants([]EnterpriseGrantType{
		EnterpriseGrantIdentityLink,
		EnterpriseGrantNotificationDirect,
	}, AuthorizationPurposeNotificationBind)
	if err == nil {
		t.Fatal("default binding must explicitly freeze DIRECT and MENTION")
	}

	grants, err := NormalizeRequestedGrants([]EnterpriseGrantType{
		EnterpriseGrantNotificationMention,
		EnterpriseGrantIdentityLink,
		EnterpriseGrantNotificationDirect,
	}, AuthorizationPurposeNotificationBind)
	if err != nil {
		t.Fatalf("NormalizeRequestedGrants() error=%v", err)
	}
	if len(grants) != 3 || grants[0] != EnterpriseGrantIdentityLink || grants[1] != EnterpriseGrantNotificationDirect || grants[2] != EnterpriseGrantNotificationMention {
		t.Fatalf("grants=%v", grants)
	}
}

func TestEnterpriseProviderSubjectKindsAreConnectionScopedOpaqueKinds(t *testing.T) {
	if got := SubjectKindForProvider(EnterpriseProviderFeishu); got != "FEISHU_OPEN_ID" {
		t.Fatalf("feishu kind=%q", got)
	}
	if got := SubjectKindForProvider(EnterpriseProviderWeCom); got != "WECOM_USERID" {
		t.Fatalf("wecom kind=%q", got)
	}
}
