package domain

import "testing"

func TestResolveWeComParametersDropsBroadcastMention(t *testing.T) {
	resolved, provided, warnings := ResolveProviderParameters(
		ChannelTypeWeComApp,
		nil,
		map[string]any{ProviderParameterMentionedList: []string{"@all"}},
	)
	if len(resolved) != 0 || len(provided) != 0 {
		t.Fatalf("broadcast mention must not become a provider snapshot: resolved=%#v provided=%#v", resolved, provided)
	}
	if len(warnings) != 1 || warnings[0].Provider != ChannelTypeWeComApp || warnings[0].Key != ProviderParameterMentionedList || warnings[0].Reason != "DISALLOWED_VALUE" {
		t.Fatalf("warnings=%#v, want value-free disallowed-parameter warning", warnings)
	}
}

func TestNormalizeExternalMemberSubjectRejectsWeComAggregateTargets(t *testing.T) {
	for _, subject := range []string{"@all", "member-a|member-b", "member-a,member-b", "member-a;member-b", "member-a\nmember-b", "member a"} {
		if _, err := NormalizeExternalMemberSubject(ExternalIdentityWeComUserID, subject); err == nil {
			t.Fatalf("aggregate WeCom target %q must be rejected", subject)
		}
	}
	if got, err := NormalizeExternalMemberSubject(ExternalIdentityWeComUserID, "member-a"); err != nil || got != "member-a" {
		t.Fatalf("single WeCom target=%q err=%v, want member-a", got, err)
	}
}

func TestFeishuApplicationSupportsDistinctUserAndGroupIdentityKinds(t *testing.T) {
	if !SupportsEnterpriseApplicationIdentityKind(ChannelTypeFeishuApp, ExternalIdentityFeishuOpenID) {
		t.Fatal("Feishu application must support an application-scoped user open_id")
	}
	if !SupportsEnterpriseApplicationIdentityKind(ChannelTypeFeishuApp, ExternalIdentityFeishuChatID) {
		t.Fatal("Feishu application must support one dynamically supplied chat_id")
	}
	if SupportsEnterpriseApplicationIdentityKind(ChannelTypeWeComApp, ExternalIdentityFeishuChatID) {
		t.Fatal("WeCom application must not accept a Feishu chat_id")
	}
	if got, err := NormalizeExternalTargetSubject(ExternalIdentityFeishuChatID, "  oc_group_123  "); err != nil || got != "oc_group_123" {
		t.Fatalf("normalized Feishu chat target=%q err=%v, want oc_group_123", got, err)
	}
}

func TestNormalizeProviderParameterSettingsRejectsBroadcastDefaultEvenWhenDisabled(t *testing.T) {
	_, err := NormalizeProviderParameterSettings(ChannelTypeWeComApp, []ProviderParameterSetting{{
		Key:          ProviderParameterMentionedList,
		Enabled:      false,
		DefaultValue: []string{"@all"},
	}})
	if err == nil {
		t.Fatal("disallowed provider default must not be retained while disabled")
	}
}
