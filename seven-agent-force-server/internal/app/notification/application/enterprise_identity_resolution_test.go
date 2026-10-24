package application

import (
	"context"
	"testing"
	"time"

	"github.com/CaixyPromise/seven-agent/force/internal/app/notification/facade"
)

func TestResolveEnterprisePersonalRecipientsRequiresOrganizationBoundary(t *testing.T) {
	service := &Service{enterpriseIdentities: enterpriseIdentityResolverStub{}}
	_, _, err := service.resolveEnterprisePersonalRecipients(context.Background(), facade.PublishRequest{
		Audience:      facade.Audience{UserIDs: []int64{201}},
		ReceiverKinds: []facade.ReceiverKind{facade.ReceiverKindFeishuOpenID},
	})
	if err == nil {
		t.Fatal("personal enterprise recipient resolved without organization boundary")
	}
}

func TestResolveEnterprisePersonalRecipientsRejectsCallerSuppliedSubject(t *testing.T) {
	service := &Service{enterpriseIdentities: enterpriseIdentityResolverStub{}}
	_, _, err := service.resolveEnterprisePersonalRecipients(context.Background(), facade.PublishRequest{
		OrganizationID: 101,
		Audience:       facade.Audience{UserIDs: []int64{201}},
		ReceiverKinds:  []facade.ReceiverKind{facade.ReceiverKindFeishuOpenID},
		ExternalRecipients: []facade.ExternalRecipient{{
			IdentityKind: facade.ExternalIdentityFeishuOpenID,
			Subject:      "caller-controlled-open-id",
		}},
	})
	if err == nil {
		t.Fatal("caller supplied enterprise subject was accepted")
	}
}

func TestResolveEnterprisePersonalRecipientsUsesPlatformAuthority(t *testing.T) {
	resolver := enterpriseIdentityResolverStub{identityKind: "FEISHU_OPEN_ID", subject: "resolved-open-id"}
	service := &Service{enterpriseIdentities: resolver}
	items, authorities, err := service.resolveEnterprisePersonalRecipients(context.Background(), facade.PublishRequest{
		OrganizationID: 101,
		Audience:       facade.Audience{UserIDs: []int64{201}},
		ReceiverKinds:  []facade.ReceiverKind{facade.ReceiverKindFeishuOpenID},
	})
	if err != nil {
		t.Fatalf("resolve recipient: %v", err)
	}
	if len(items) != 1 || items[0].Subject != "resolved-open-id" || items[0].IdentityKind != facade.ExternalIdentityFeishuOpenID {
		t.Fatalf("resolved items = %#v", items)
	}
	if snapshot := authorities[enterpriseAuthorityKey("FEISHU_OPEN_ID", "resolved-open-id")]; snapshot.externalIdentityID != 401 || snapshot.enterpriseConnectionID != 301 {
		t.Fatalf("authority snapshot = %#v", snapshot)
	}
}

type enterpriseIdentityResolverStub struct {
	identityKind string
	subject      string
}

func (enterpriseIdentityResolverStub) ResolveEnterpriseReadiness(context.Context, int64, int64, string) (bool, bool, bool, string, error) {
	return true, true, true, "", nil
}

func (s enterpriseIdentityResolverStub) ResolveEnterpriseNotificationAuthority(context.Context, int64, int64, string, string) (string, string, string, string, int64, int64, int64, int64, error) {
	return s.identityKind, s.subject, "authority-digest", "connection-digest", 401, 1, 301, 1, nil
}

func (enterpriseIdentityResolverStub) ValidateEnterpriseNotificationAuthority(context.Context, int64, int64, int64, int64, int64, int64, string, string) error {
	return nil
}

func (enterpriseIdentityResolverStub) AcquireEnterpriseApplicationCredential(context.Context, int64, int64, int64, string) (string, string, string, string, string, int64, time.Time, error) {
	return "FEISHU", "app", "", "", "secret", 1, time.Now().Add(time.Minute), nil
}
