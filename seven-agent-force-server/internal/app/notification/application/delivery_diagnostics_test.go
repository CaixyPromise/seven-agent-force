package application

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/CaixyPromise/seven-agent/force/internal/app/notification/domain"
	"github.com/CaixyPromise/seven-agent/force/internal/app/notification/facade"
	"github.com/CaixyPromise/seven-agent/force/internal/shared/stepup"
)

func TestDeliveryDiagnosticsKeepsListContentFreeAndAuditsSensitiveRead(t *testing.T) {
	repo := newExternalTestRepository()
	now := time.Date(2026, 7, 27, 9, 30, 0, 0, time.UTC)
	repo.deliveries["delivery-sensitive"] = &domain.Delivery{
		ID: 101, DeliveryID: "delivery-sensitive", SceneCode: "billing.ready", ChannelCode: "email-default",
		ChannelType: domain.ChannelTypeEmail, TemplateCode: "billing-template", Target: "private@example.com",
		TargetMasked: "p***e@example.com", PayloadJSON: `{"privateVariable":"do-not-return"}`,
		RenderedSubject: "客户账单", RenderedText: "敏感正文", ContentTier: domain.DeliveryContentTierSensitive,
		Status: domain.DeliveryStatusFailed, LastError: "provider rejected private@example.com", CreateTime: now, UpdateTime: now,
	}
	service := newExternalTestService(t, repo, nil)

	page, err := service.ListDeliveries(context.Background(), domain.DeliveryQuery{Current: 1, PageSize: 20})
	if err != nil || page == nil || len(page.Records) != 1 {
		t.Fatalf("ListDeliveries() page=%#v err=%v", page, err)
	}
	rawList, err := json.Marshal(page.Records[0])
	if err != nil {
		t.Fatalf("marshal summary: %v", err)
	}
	for _, forbidden := range []string{"客户账单", "敏感正文", "privateVariable", "private@example.com", "provider rejected"} {
		if strings.Contains(string(rawList), forbidden) {
			t.Fatalf("normal delivery list leaked %q: %s", forbidden, rawList)
		}
	}
	if page.Records[0].FailureCode == "" || page.Records[0].FailureMessage == "" {
		t.Fatalf("safe failure hint missing: %#v", page.Records[0])
	}

	proof := stepup.ProofMetadata{
		BusinessAction:        DeliveryDiagnosticBusinessAction(),
		OperationBinding:      DeliveryDiagnosticOperationBinding("delivery-sensitive", domain.DeliveryDiagnosticReasonIncident, "INC-42"),
		ProofIdentifier:       "proof-42",
		ChallengeIdentifier:   "challenge-42",
		AssuranceLevel:        "AAL2",
		AuthenticationMethods: []string{"TOTP"},
	}
	content, err := service.ReadDeliveryDiagnosticContent(context.Background(), DeliveryDiagnosticReadCommand{
		DeliveryID: "delivery-sensitive", ReasonCode: domain.DeliveryDiagnosticReasonIncident, TicketReference: "INC-42",
		ActorID: 7, GrantedPermission: domain.DeliveryDiagnosticPermission(domain.DeliveryContentTierSensitive), StepUpProof: proof,
	})
	if err != nil || content == nil || content.Subject != "客户账单" || content.Text != "敏感正文" {
		t.Fatalf("ReadDeliveryDiagnosticContent() content=%#v err=%v", content, err)
	}
	if len(repo.diagnosticAudits) != 1 || repo.diagnosticAudits[0].ResultCode != domain.DeliveryDiagnosticResultAllowed || repo.diagnosticAudits[0].TicketReference != "INC-42" {
		t.Fatalf("diagnostic audit=%#v", repo.diagnosticAudits)
	}

	foreignScope := newExternalTestService(t, repo, nil)
	foreignScope.SetScopeID("other-scope")
	foreignContent, foreignErr := foreignScope.ReadDeliveryDiagnosticContent(context.Background(), DeliveryDiagnosticReadCommand{
		DeliveryID: "delivery-sensitive", ReasonCode: domain.DeliveryDiagnosticReasonIncident, TicketReference: "INC-42",
		ActorID: 8, GrantedPermission: domain.DeliveryDiagnosticPermission(domain.DeliveryContentTierSensitive), StepUpProof: proof,
	})
	if foreignErr == nil || foreignContent != nil {
		t.Fatalf("foreign scope read content=%#v err=%v, want controlled denial", foreignContent, foreignErr)
	}
	if len(repo.diagnosticAudits) != 2 || repo.diagnosticAudits[1].ResultCode != domain.DeliveryDiagnosticResultNotFound || repo.diagnosticAudits[1].ScopeID != "other-scope" {
		t.Fatalf("foreign scope diagnostic audit=%#v", repo.diagnosticAudits)
	}
}

func TestChallengeOTPNeverUsesNormalDeliveryContentAndExpiresBeforeDispatch(t *testing.T) {
	repo := newExternalTestRepository()
	repo.channels["otp-email"] = &domain.Channel{
		ID: 1, ChannelCode: "otp-email", ChannelName: "OTP email", ChannelType: domain.ChannelTypeEmail,
		ScopeID: "local", Status: domain.ChannelStatusEnabled,
	}
	repo.templates["otp-template"] = &domain.Template{
		ID: 2, TemplateCode: "otp-template", TemplateName: "OTP", ScopeID: "local",
		SceneCode: domain.SceneChallengeOTP, ChannelType: domain.ChannelTypeEmail,
		SubjectTemplate: "验证码", TextTemplate: "验证码 {{.Code}}", Status: domain.ChannelStatusEnabled,
	}
	repo.sceneBindings["local\x00"+domain.SceneChallengeOTP] = &domain.SceneBinding{
		ID: 3, ScopeID: "local", SceneCode: domain.SceneChallengeOTP, SceneName: "验证码",
		ChannelCode: "otp-email", TemplateCode: "otp-template", Enabled: true, MaxRetry: 2,
	}
	service := newExternalTestService(t, repo, nil)
	if err := service.EnqueueChallengeOTP(context.Background(), facade.ChallengeOTPRequest{
		ToEmail: "member@example.com", Code: "493281", Scene: "RESET_PASSWORD", TTL: time.Minute,
	}); err != nil {
		t.Fatalf("EnqueueChallengeOTP() error=%v", err)
	}
	if len(repo.deliveries) != 1 || len(repo.ephemeralContents) != 1 || len(repo.outbox) != 1 {
		t.Fatalf("OTP persistence deliveries=%d ephemeral=%d outbox=%d", len(repo.deliveries), len(repo.ephemeralContents), len(repo.outbox))
	}
	var delivery *domain.Delivery
	for _, item := range repo.deliveries {
		delivery = item
	}
	if delivery == nil || delivery.Presentation != domain.DeliveryPresentationText || delivery.ContentTier != domain.DeliveryContentTierSecretEphemeral || delivery.PayloadJSON != "" || delivery.RenderedSubject != "" || delivery.RenderedText != "" || delivery.RenderedHTML != "" || delivery.RenderedMarkdown != "" {
		t.Fatalf("normal OTP delivery leaked content: %#v", delivery)
	}
	proof := stepup.ProofMetadata{
		BusinessAction:        DeliveryDiagnosticBusinessAction(),
		OperationBinding:      DeliveryDiagnosticOperationBinding(delivery.DeliveryID, domain.DeliveryDiagnosticReasonSecurityReview, "SEC-OTP-1"),
		ProofIdentifier:       "proof-otp-1",
		ChallengeIdentifier:   "challenge-otp-1",
		AssuranceLevel:        "AAL2",
		AuthenticationMethods: []string{"TOTP"},
	}
	content, err := service.ReadDeliveryDiagnosticContent(context.Background(), DeliveryDiagnosticReadCommand{
		DeliveryID: delivery.DeliveryID, ReasonCode: domain.DeliveryDiagnosticReasonSecurityReview, TicketReference: "SEC-OTP-1",
		ActorID: 7, GrantedPermission: domain.DeliveryDiagnosticPermission(domain.DeliveryContentTierSecretEphemeral), StepUpProof: proof,
	})
	if err != nil || content == nil || content.Text != "验证码 493281" || content.ExpiresAt == nil {
		t.Fatalf("ReadDeliveryDiagnosticContent(secret) content=%#v err=%v", content, err)
	}
	if len(repo.diagnosticAudits) != 1 || repo.diagnosticAudits[0].ResultCode != domain.DeliveryDiagnosticResultAllowed {
		t.Fatalf("secret diagnostic audit=%#v", repo.diagnosticAudits)
	}
	normalRaw, _ := json.Marshal(delivery)
	outboxRaw, _ := json.Marshal(repo.outbox[0])
	for _, raw := range []string{string(normalRaw), string(outboxRaw), repo.ephemeralContents[delivery.DeliveryID].Ciphertext} {
		if strings.Contains(raw, "493281") {
			t.Fatalf("OTP code escaped protected envelope: %s", raw)
		}
	}
	service.now = func() time.Time { return time.Date(2026, 7, 23, 12, 2, 0, 0, time.UTC) }
	if err := service.dispatch(context.Background(), delivery.DeliveryID); err == nil || !isDeliveryAsyncHandled(err) {
		t.Fatalf("expired OTP dispatch error=%v, want handled terminal failure", err)
	}
	stored := repo.deliveries[delivery.DeliveryID]
	if stored.Status != domain.DeliveryStatusFailed || stored.LastError != domain.DeliveryDiagnosticResultExpired {
		t.Fatalf("expired OTP delivery=%#v", stored)
	}
	expiredContent, expiredErr := service.ReadDeliveryDiagnosticContent(context.Background(), DeliveryDiagnosticReadCommand{
		DeliveryID: delivery.DeliveryID, ReasonCode: domain.DeliveryDiagnosticReasonSecurityReview, TicketReference: "SEC-OTP-1",
		ActorID: 7, GrantedPermission: domain.DeliveryDiagnosticPermission(domain.DeliveryContentTierSecretEphemeral), StepUpProof: proof,
	})
	if expiredErr == nil || expiredContent != nil || !strings.Contains(expiredErr.Error(), "短期秘密已过期") {
		t.Fatalf("expired secret diagnostic content=%#v err=%v", expiredContent, expiredErr)
	}
	if len(repo.diagnosticAudits) != 2 || repo.diagnosticAudits[1].ResultCode != domain.DeliveryDiagnosticResultExpired {
		t.Fatalf("expired secret diagnostic audit=%#v", repo.diagnosticAudits)
	}
}
