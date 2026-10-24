package notification

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"strconv"
	"strings"
	"testing"
	"time"

	notificationapp "github.com/CaixyPromise/seven-agent/force/internal/app/notification/application"
	notificationdomain "github.com/CaixyPromise/seven-agent/force/internal/app/notification/domain"
	notificationhandler "github.com/CaixyPromise/seven-agent/force/internal/app/notification/handler"
	adminfacade "github.com/CaixyPromise/seven-agent/force/internal/app/system/admin/facade"
	apperrors "github.com/CaixyPromise/seven-agent/force/internal/common/errors"
	secretvalueinfra "github.com/CaixyPromise/seven-agent/force/internal/infrastructure/security/crypto/secretvalue"
	"github.com/CaixyPromise/seven-agent/force/internal/shared/config"
	"github.com/CaixyPromise/seven-agent/force/internal/shared/securitycontext"
	"github.com/CaixyPromise/seven-agent/force/internal/shared/xcontext"
	"github.com/cloudwego/hertz/pkg/app"
	"github.com/cloudwego/hertz/pkg/app/server"
	"github.com/cloudwego/hertz/pkg/common/ut"
)

func TestNotificationScopeIDUsesDeploymentIdentityNotOrganization(t *testing.T) {
	cases := []struct {
		name string
		cfg  config.Config
		want string
	}{
		{name: "default local", want: "local"},
		{name: "hub", cfg: config.Config{Platform: config.PlatformConfig{Mode: config.PlatformModeHub}}, want: "hub"},
		{name: "node", cfg: config.Config{Platform: config.PlatformConfig{Mode: config.PlatformModeNode, Node: config.PlatformNodeConfig{Code: "edge-a"}}}, want: "node:edge-a"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := notificationScopeID(tc.cfg)
			if err != nil || got != tc.want {
				t.Fatalf("notificationScopeID() = %q, %v; want %q", got, err, tc.want)
			}
		})
	}
	if _, err := notificationScopeID(config.Config{Platform: config.PlatformConfig{Mode: config.PlatformModeNode}}); err == nil {
		t.Fatal("node scope unexpectedly accepted an empty node code")
	}
}

func TestNotificationOutboundPoliciesAllowFakeIPOnlyInLocalDevelopment(t *testing.T) {
	policies := []config.NotificationOutboundPolicyConfig{{
		Name:             "local-fake",
		Mode:             "FAKE_IP_PROXY",
		AllowedHostnames: []string{"receiver.example"},
		AllowedCIDRs:     []string{"198.18.0.0/15"},
		AllowedPorts:     []int{443},
		ProxyURL:         "https://proxy.example:8443",
	}}
	if _, err := notificationOutboundPolicyRegistry(config.Config{
		Seven:        config.SevenConfig{Env: "prod"},
		Platform:     config.PlatformConfig{Mode: config.PlatformModeHub},
		Notification: config.NotificationConfig{Outbound: config.NotificationOutboundConfig{Policies: policies}},
	}); err == nil {
		t.Fatal("production-like notification runtime accepted fake-IP proxy policy")
	}
	registry, err := notificationOutboundPolicyRegistry(config.Config{
		Seven:        config.SevenConfig{Env: "dev"},
		Platform:     config.PlatformConfig{Mode: config.PlatformModeLocal},
		Notification: config.NotificationConfig{Outbound: config.NotificationOutboundConfig{Policies: policies}},
	})
	if err != nil {
		t.Fatalf("local development policy registry error = %v", err)
	}
	if _, err := registry.Resolve("local-fake"); err != nil {
		t.Fatalf("local fake-IP policy was not available: %v", err)
	}
}

func TestTemplateOperationAuditRetainsOnlySafeMetadata(t *testing.T) {
	module := &Module{scopeID: "scope-a"}
	spec := module.templateOperationLogSpec(adminfacade.OperationTypeConfigUpdate, "发布通知模板版本", "PUBLISH", "/notification/template-revisions/:revisionId/publish")
	if spec.IncludeParams || spec.IncludeResult || !spec.OmitQuery || len(spec.CompletionEnrichers) != 1 {
		t.Fatalf("template operation log spec must disable generic body capture: %+v", spec)
	}

	reqCtx := &app.RequestContext{}
	reqCtx.Request.SetRequestURI("/notification/template-revisions/829/publish")
	reqCtx.Response.SetBodyString(`{"code":0,"data":{"templateCode":"account_notice","currentPublished":{"revisionNo":2,"textTemplate":"full private content","variables":[{"sampleValue":"one-time-preview-value"}]}}}`)
	entry := &adminfacade.OperationLogEntry{}
	templateOperationAuditCompletionEnricher{scopeID: "scope-a", action: "PUBLISH", route: "/notification/template-revisions/:revisionId/publish"}.Enrich(context.Background(), reqCtx, entry)
	assertTemplateOperationAuditEntry(t, entry, "PUBLISH", "scope-a", "account_notice", 2, 0)

	errorCtx := &app.RequestContext{}
	errorCtx.Request.SetRequestURI("/notification/template-revisions/829/publish")
	xcontext.SetResponseError(errorCtx, 40017, "full private content")
	errorCtx.Response.SetBodyString(`{"code":40017,"data":{"detail":"one-time-preview-value"},"message":"full private content"}`)
	errorEntry := &adminfacade.OperationLogEntry{}
	templateOperationAuditCompletionEnricher{scopeID: "scope-a", action: "PUBLISH", route: "/notification/template-revisions/:revisionId/publish"}.Enrich(context.Background(), errorCtx, errorEntry)
	assertTemplateOperationAuditEntry(t, errorEntry, "PUBLISH", "scope-a", "", 0, 40017)
	if errorEntry.ErrorMsg != "template operation failed" {
		t.Fatalf("unsafe provider/template error message leaked into audit: %q", errorEntry.ErrorMsg)
	}

	for _, forbidden := range []string{"full private content", "one-time-preview-value", "textTemplate", "variables", "detail", "message"} {
		if strings.Contains(entry.RequestParams+entry.ResponseResult+entry.ErrorMsg+errorEntry.RequestParams+errorEntry.ResponseResult+errorEntry.ErrorMsg, forbidden) {
			t.Fatalf("operation audit leaked %q: success=%+v error=%+v", forbidden, entry, errorEntry)
		}
	}
}

func TestDeliveryDiagnosticOperationAuditRetainsOnlySafeMetadata(t *testing.T) {
	module := &Module{scopeID: "scope-a"}
	spec := module.deliveryDiagnosticOperationLogSpec()
	if spec.IncludeParams || spec.IncludeResult || !spec.OmitQuery || len(spec.CompletionEnrichers) != 1 {
		t.Fatalf("delivery diagnostic operation log spec must disable generic body capture: %+v", spec)
	}

	reqCtx := &app.RequestContext{}
	reqCtx.Request.SetRequestURI("/notification/deliveries/delivery-private/diagnostic-content")
	reqCtx.Response.SetBodyString(`{"code":0,"data":{"deliveryId":"delivery-private","reasonCode":"INCIDENT","ticketReference":"INC-42","subject":"private subject","text":"private text"}}`)
	entry := &adminfacade.OperationLogEntry{}
	deliveryDiagnosticOperationAuditCompletionEnricher{scopeID: "scope-a"}.Enrich(context.Background(), reqCtx, entry)
	assertDeliveryDiagnosticOperationAuditEntry(t, entry, "scope-a", 0)

	errorCtx := &app.RequestContext{}
	errorCtx.Request.SetRequestURI("/notification/deliveries/delivery-private/diagnostic-content")
	xcontext.SetResponseError(errorCtx, 40017, "provider returned private body")
	errorCtx.Response.SetBodyString(`{"code":40017,"message":"provider returned private body","data":{"subject":"private subject","text":"private text"}}`)
	errorEntry := &adminfacade.OperationLogEntry{}
	deliveryDiagnosticOperationAuditCompletionEnricher{scopeID: "scope-a"}.Enrich(context.Background(), errorCtx, errorEntry)
	assertDeliveryDiagnosticOperationAuditEntry(t, errorEntry, "scope-a", 40017)
	if errorEntry.ErrorMsg != "delivery diagnostic failed" {
		t.Fatalf("unsafe diagnostic error message leaked into operation audit: %q", errorEntry.ErrorMsg)
	}

	for _, forbidden := range []string{
		"delivery-private", "INCIDENT", "INC-42", "private subject", "private text", "provider returned private body", "subject", "ticketReference",
	} {
		if strings.Contains(entry.RequestParams+entry.ResponseResult+entry.ErrorMsg+errorEntry.RequestParams+errorEntry.ResponseResult+errorEntry.ErrorMsg, forbidden) {
			t.Fatalf("delivery diagnostic operation audit leaked %q: success=%+v error=%+v", forbidden, entry, errorEntry)
		}
	}
}

func TestDeliveryDiagnosticRejectsUnauthorizedRequestsBeforeDeliveryLookup(t *testing.T) {
	repo := &diagnosticRouteRepository{scopeID: "local", delivery: &notificationdomain.Delivery{
		DeliveryID:  "delivery-known",
		ContentTier: notificationdomain.DeliveryContentTierSensitive,
	}}
	service := notificationapp.NewService(notificationModuleTestTransactor{}, repo, nil, inboxRouteSecretService{}, nil, nil, nil, nil)
	service.SetScopeID("local")
	engine := server.Default()
	engine.Use(func(ctx context.Context, reqCtx *app.RequestContext) {
		securitycontext.Set(reqCtx, &securitycontext.UserContext{UserID: 42, Username: "operator-without-diagnostic-capability"})
		reqCtx.Next(ctx)
	})
	module := &Module{handler: notificationhandler.NewHandler(service)}
	module.Mount(engine.Engine)

	body := `{"reasonCode":"INCIDENT","ticketReference":"INC-42"}`
	known := ut.PerformRequest(engine.Engine, "POST", "/notification/deliveries/delivery-known/diagnostic-content", &ut.Body{Body: strings.NewReader(body), Len: len(body)}, ut.Header{Key: "Content-Type", Value: "application/json"})
	missing := ut.PerformRequest(engine.Engine, "POST", "/notification/deliveries/delivery-missing/diagnostic-content", &ut.Body{Body: strings.NewReader(body), Len: len(body)}, ut.Header{Key: "Content-Type", Value: "application/json"})
	assertNotificationBusinessCode(t, known, apperrors.CodeForbidden)
	assertNotificationBusinessCode(t, missing, apperrors.CodeForbidden)
	if repo.lookupCalls != 0 {
		t.Fatalf("unauthorized diagnostic requests must not probe delivery existence; lookupCalls=%d", repo.lookupCalls)
	}
	if len(repo.audits) != 2 {
		t.Fatalf("each denied diagnostic request must be audited: %#v", repo.audits)
	}
}

func assertDeliveryDiagnosticOperationAuditEntry(t *testing.T, entry *adminfacade.OperationLogEntry, scopeID string, errorCode int) {
	t.Helper()
	const route = "/notification/deliveries/:deliveryId/diagnostic-content"
	if entry.MethodName != route || entry.RequestURL != route {
		t.Fatalf("delivery diagnostic operation audit did not preserve route template: %+v", entry)
	}
	var metadata map[string]any
	if err := json.Unmarshal([]byte(entry.RequestParams), &metadata); err != nil {
		t.Fatalf("decode delivery diagnostic operation metadata: %v payload=%s", err, entry.RequestParams)
	}
	if metadata["action"] != "READ_CONTENT" || metadata["scopeId"] != scopeID {
		t.Fatalf("unexpected delivery diagnostic operation metadata: %#v", metadata)
	}
	if len(metadata) != 2+mapBoolToInt(errorCode != 0) {
		t.Fatalf("delivery diagnostic operation metadata contains unsafe fields: %#v", metadata)
	}
	if errorCode == 0 {
		if _, found := metadata["errorCode"]; found {
			t.Fatalf("successful diagnostic operation unexpectedly has error metadata: %#v", metadata)
		}
	} else if metadata["errorCode"] != float64(errorCode) {
		t.Fatalf("missing diagnostic error code: %#v", metadata)
	}
	if entry.ResponseResult != `{"code":`+strconv.Itoa(errorCode)+`}` {
		t.Fatalf("diagnostic operation result must contain only the code: %s", entry.ResponseResult)
	}
}

func mapBoolToInt(value bool) int {
	if value {
		return 1
	}
	return 0
}

func TestSceneOperationAuditRetainsOnlySafeMetadata(t *testing.T) {
	module := &Module{scopeID: "scope-a"}
	spec := module.templateOperationLogSpec(adminfacade.OperationTypeConfigUpdate, "发布新版通知场景", "PUBLISH", "/notification/scene-revisions/:revisionId/publish")
	if spec.IncludeParams || spec.IncludeResult || !spec.OmitQuery || len(spec.CompletionEnrichers) != 1 {
		t.Fatalf("scene operation log spec must disable generic body capture: %+v", spec)
	}

	reqCtx := &app.RequestContext{}
	reqCtx.Response.SetBodyString(`{"code":0,"data":{"sceneCode":"invoice_ready","currentPublished":{"revisionNo":2,"connectionRef":"feishu-production","templateRevisionId":"761234567890123456"}}}`)
	entry := &adminfacade.OperationLogEntry{}
	templateOperationAuditCompletionEnricher{scopeID: "scope-a", action: "PUBLISH", route: "/notification/scene-revisions/:revisionId/publish"}.Enrich(context.Background(), reqCtx, entry)
	if entry.MethodName != "/notification/scene-revisions/:revisionId/publish" || entry.RequestURL != entry.MethodName {
		t.Fatalf("scene operation audit did not preserve route template: %+v", entry)
	}
	var metadata map[string]any
	if err := json.Unmarshal([]byte(entry.RequestParams), &metadata); err != nil {
		t.Fatalf("decode scene operation metadata: %v payload=%s", err, entry.RequestParams)
	}
	if metadata["action"] != "PUBLISH" || metadata["scopeId"] != "scope-a" || metadata["sceneCode"] != "invoice_ready" || metadata["revisionNo"] != float64(2) {
		t.Fatalf("unexpected scene operation metadata: %#v", metadata)
	}
	for _, forbidden := range []string{"feishu-production", "templateRevisionId", "761234567890123456"} {
		if strings.Contains(entry.RequestParams+entry.ResponseResult+entry.ErrorMsg, forbidden) {
			t.Fatalf("scene operation audit leaked %q: %+v", forbidden, entry)
		}
	}
}

func assertTemplateOperationAuditEntry(t *testing.T, entry *adminfacade.OperationLogEntry, action, scopeID, templateCode string, revisionNo, errorCode int) {
	t.Helper()
	if entry.MethodName != "/notification/template-revisions/:revisionId/publish" || entry.RequestURL != entry.MethodName {
		t.Fatalf("operation audit did not preserve route template: %+v", entry)
	}
	var metadata map[string]any
	if err := json.Unmarshal([]byte(entry.RequestParams), &metadata); err != nil {
		t.Fatalf("decode operation metadata: %v payload=%s", err, entry.RequestParams)
	}
	if metadata["action"] != action || metadata["scopeId"] != scopeID {
		t.Fatalf("unexpected operation metadata: %#v", metadata)
	}
	if templateCode == "" {
		if _, found := metadata["templateCode"]; found {
			t.Fatalf("failed operation must not retain response data: %#v", metadata)
		}
	} else if metadata["templateCode"] != templateCode {
		t.Fatalf("missing template code: %#v", metadata)
	}
	if revisionNo == 0 {
		if _, found := metadata["revisionNo"]; found {
			t.Fatalf("failed operation must not retain response revision: %#v", metadata)
		}
	} else if metadata["revisionNo"] != float64(revisionNo) {
		t.Fatalf("missing revision number: %#v", metadata)
	}
	if errorCode == 0 {
		if _, found := metadata["errorCode"]; found {
			t.Fatalf("successful operation unexpectedly has error metadata: %#v", metadata)
		}
	} else if metadata["errorCode"] != float64(errorCode) {
		t.Fatalf("missing error code: %#v", metadata)
	}
	if entry.ResponseResult != `{"code":`+strconv.Itoa(errorCode)+`}` {
		t.Fatalf("operation result must contain only the code: %s", entry.ResponseResult)
	}
}

func TestDisabledNotificationBackgroundDoesNotStartSharedConsumerOrRealtime(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	broker := &backgroundProbeBroker{started: make(chan struct{}, 1)}
	realtime := &backgroundProbeRealtime{}

	startNotificationBackground(ctx, true, broker, backgroundProbeDispatchHandler{}, realtime)
	select {
	case <-broker.started:
		t.Fatal("disabled notification background started a shared RabbitMQ consumer")
	case <-time.After(50 * time.Millisecond):
	}
	if realtime.starts != 0 {
		t.Fatalf("disabled notification background started realtime %d times", realtime.starts)
	}
}

func TestEnabledNotificationBackgroundStartsConsumerAndRealtime(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	broker := &backgroundProbeBroker{started: make(chan struct{}, 1)}
	realtime := &backgroundProbeRealtime{}

	startNotificationBackground(ctx, false, broker, backgroundProbeDispatchHandler{}, realtime)
	select {
	case <-broker.started:
	case <-time.After(time.Second):
		t.Fatal("enabled notification background did not start the shared RabbitMQ consumer")
	}
	if realtime.starts != 1 {
		t.Fatalf("enabled notification background started realtime %d times", realtime.starts)
	}
}

func TestInboxRoutesRequireLoginButNotNotificationManagementPermission(t *testing.T) {
	repo := &inboxRouteRepository{}
	service := notificationapp.NewService(notificationModuleTestTransactor{}, repo, nil, inboxRouteSecretService{}, nil, nil, nil, nil)
	service.SetScopeID("local")

	engine := server.Default()
	engine.Use(func(ctx context.Context, reqCtx *app.RequestContext) {
		if string(reqCtx.Request.Header.Peek("X-Test-Login")) == "true" {
			securitycontext.Set(reqCtx, &securitycontext.UserContext{UserID: 42, Username: "member"})
		}
		reqCtx.Next(ctx)
	})
	module := &Module{handler: notificationhandler.NewHandler(service)}
	module.Mount(engine.Engine)

	assertNotificationBusinessCode(t, ut.PerformRequest(engine.Engine, "GET", "/notification/inbox", nil), apperrors.CodeNotLogin)
	assertNotificationBusinessCode(t, ut.PerformRequest(engine.Engine, "GET", "/notification/inbox", nil,
		ut.Header{Key: "X-Test-Login", Value: "true"},
	), apperrors.CodeSuccess)
	if repo.lastUserID != 42 {
		t.Fatalf("inbox route used userID=%d, want authenticated user 42", repo.lastUserID)
	}
}

func TestInboxReadRoutesKeepContentBehindExplicitDetail(t *testing.T) {
	const (
		recipientID = "nrc_route_42"
		title       = "账号安全设置已更新"
		content     = "这段完整正文只能在用户主动打开消息详情后返回。"
		deepLink    = "/account/security"
	)
	now := time.Date(2026, 7, 22, 12, 0, 0, 0, time.UTC)
	repo := &inboxRouteRepository{recipient: &notificationdomain.Recipient{
		ID:             4201,
		RecipientID:    recipientID,
		NotificationID: 4101,
		ScopeID:        "local",
		UserID:         42,
		EventKey:       "account.security.changed",
		Category:       "ACCOUNT",
		Priority:       "NORMAL",
		Title:          title,
		Content:        content,
		DeepLink:       deepLink,
		MailboxVersion: 2,
		CreateTime:     now,
		UpdateTime:     now,
	}}
	service := notificationapp.NewService(notificationModuleTestTransactor{}, repo, nil, inboxRouteSecretService{}, nil, nil, nil, nil)
	service.SetScopeID("local")

	engine := server.Default()
	engine.Use(func(ctx context.Context, reqCtx *app.RequestContext) {
		securitycontext.Set(reqCtx, &securitycontext.UserContext{UserID: 42, Username: "member"})
		reqCtx.Next(ctx)
	})
	module := &Module{handler: notificationhandler.NewHandler(service)}
	module.Mount(engine.Engine)

	assertInboxRouteDoesNotContain(t, ut.PerformRequest(engine.Engine, "GET", "/notification/inbox/unread-count", nil), content, deepLink, title, recipientID)
	assertInboxRouteDoesNotContain(t, ut.PerformRequest(engine.Engine, "GET", "/notification/inbox/unread-preview", nil), content, deepLink)
	assertInboxRouteDoesNotContain(t, ut.PerformRequest(engine.Engine, "GET", "/notification/inbox", nil), content, deepLink)
	assertInboxRouteDoesNotContain(t, ut.PerformRequest(engine.Engine, "GET", "/notification/inbox/changes", nil), content, deepLink)

	detail := ut.PerformRequest(engine.Engine, "GET", "/notification/inbox/"+recipientID, nil)
	if detail.Code != 200 || !strings.Contains(detail.Body.String(), content) || !strings.Contains(detail.Body.String(), deepLink) {
		t.Fatalf("explicit detail must contain the full body and deep link: status=%d body=%s", detail.Code, detail.Body.String())
	}
}

type inboxRouteRepository struct {
	notificationdomain.Repository
	lastUserID int64
	recipient  *notificationdomain.Recipient
}

type diagnosticRouteRepository struct {
	notificationdomain.Repository
	scopeID     string
	delivery    *notificationdomain.Delivery
	lookupCalls int
	audits      []notificationdomain.DeliveryDiagnosticAudit
}

func (r *diagnosticRouteRepository) ListDeliverySummaries(context.Context, notificationdomain.DeliveryQuery) ([]notificationdomain.DeliverySummary, int64, error) {
	return nil, 0, nil
}

func (r *diagnosticRouteRepository) FindDeliveryForDiagnostic(_ context.Context, scopeID, deliveryID string) (*notificationdomain.Delivery, error) {
	r.lookupCalls++
	if r.delivery == nil || r.scopeID != scopeID || r.delivery.DeliveryID != deliveryID {
		return nil, nil
	}
	copy := *r.delivery
	return &copy, nil
}

func (*diagnosticRouteRepository) FindDeliveryEphemeralContent(context.Context, string, string) (*notificationdomain.DeliveryEphemeralContent, error) {
	return nil, nil
}

func (*diagnosticRouteRepository) InsertDeliveryEphemeralContent(context.Context, *notificationdomain.DeliveryEphemeralContent) error {
	return nil
}

func (r *diagnosticRouteRepository) InsertDeliveryDiagnosticAudit(_ context.Context, item *notificationdomain.DeliveryDiagnosticAudit) error {
	if item != nil {
		copy := *item
		r.audits = append(r.audits, copy)
	}
	return nil
}

type backgroundProbeBroker struct {
	started chan struct{}
}

func (*backgroundProbeBroker) Enabled() bool { return true }

func (*backgroundProbeBroker) Reconnect(context.Context) error { return nil }

func (b *backgroundProbeBroker) ConsumeDispatch(ctx context.Context, _ string, _ func(context.Context, notificationdomain.DeliveryMessage) error) error {
	select {
	case b.started <- struct{}{}:
	default:
	}
	<-ctx.Done()
	return ctx.Err()
}

type backgroundProbeDispatchHandler struct{}

func (backgroundProbeDispatchHandler) HandleDispatchMessage(context.Context, notificationdomain.DeliveryMessage) error {
	return nil
}

type backgroundProbeRealtime struct {
	starts int
}

func (r *backgroundProbeRealtime) Start(context.Context) {
	r.starts++
}

func (r *inboxRouteRepository) ListInboxRecipients(_ context.Context, query notificationdomain.InboxQuery) ([]notificationdomain.Recipient, error) {
	r.lastUserID = query.UserID
	if r.recipient != nil && r.recipient.ScopeID == query.ScopeID && r.recipient.UserID == query.UserID {
		return []notificationdomain.Recipient{*r.recipient}, nil
	}
	return []notificationdomain.Recipient{}, nil
}

func (r *inboxRouteRepository) ListUnreadInboxRecipients(_ context.Context, scopeID string, userID int64, _ int) ([]notificationdomain.Recipient, error) {
	if r.recipient != nil && r.recipient.ScopeID == scopeID && r.recipient.UserID == userID && r.recipient.ReadAt == nil && r.recipient.ArchivedAt == nil {
		return []notificationdomain.Recipient{*r.recipient}, nil
	}
	return []notificationdomain.Recipient{}, nil
}

func (r *inboxRouteRepository) ListInboxRecipientChanges(_ context.Context, query notificationdomain.InboxChangeQuery) ([]notificationdomain.Recipient, error) {
	if r.recipient != nil && r.recipient.ScopeID == query.ScopeID && r.recipient.UserID == query.UserID && r.recipient.MailboxVersion > query.AfterSequence && r.recipient.MailboxVersion <= query.UntilSequence {
		return []notificationdomain.Recipient{*r.recipient}, nil
	}
	return []notificationdomain.Recipient{}, nil
}

func (r *inboxRouteRepository) FindInboxRecipient(_ context.Context, scopeID string, userID int64, recipientID string) (*notificationdomain.Recipient, error) {
	if r.recipient == nil || r.recipient.ScopeID != scopeID || r.recipient.UserID != userID || r.recipient.RecipientID != recipientID {
		return nil, nil
	}
	copy := *r.recipient
	return &copy, nil
}

func (r *inboxRouteRepository) CountUnreadInboxRecipients(_ context.Context, scopeID string, userID int64) (int64, error) {
	if r.recipient != nil && r.recipient.ScopeID == scopeID && r.recipient.UserID == userID && r.recipient.ReadAt == nil && r.recipient.ArchivedAt == nil {
		return 1, nil
	}
	return 0, nil
}

func (r *inboxRouteRepository) LockMailbox(_ context.Context, scopeID string, userID int64, mailboxKey string) (*notificationdomain.Mailbox, error) {
	return &notificationdomain.Mailbox{ScopeID: scopeID, UserID: userID, MailboxKey: mailboxKey, ChangeSequence: 2}, nil
}

type notificationModuleTestTransactor struct{}

func (notificationModuleTestTransactor) Enabled() bool { return true }

func (notificationModuleTestTransactor) WithinTransaction(ctx context.Context, fn func(context.Context) error) error {
	return fn(ctx)
}

type inboxRouteSecretService struct{}

func (inboxRouteSecretService) EncryptString(_ context.Context, plain string) (secretvalueinfra.SecretValue, error) {
	return secretvalueinfra.SecretValue{CiphertextB64: base64.RawURLEncoding.EncodeToString([]byte(plain)), EDEKB64: "test-edek", WrapKeyRef: "test-key"}, nil
}

func (inboxRouteSecretService) DecryptString(_ context.Context, value secretvalueinfra.SecretValue) (string, error) {
	raw, err := base64.RawURLEncoding.DecodeString(value.CiphertextB64)
	if err != nil {
		return "", err
	}
	return string(raw), nil
}

func assertNotificationBusinessCode(t *testing.T, recorder *ut.ResponseRecorder, expected int) {
	t.Helper()
	if recorder.Code != 200 {
		t.Fatalf("unexpected HTTP status: %d body=%s", recorder.Code, recorder.Body.String())
	}
	var result struct {
		Code int `json:"code"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &result); err != nil {
		t.Fatalf("decode response: %v body=%s", err, recorder.Body.String())
	}
	if result.Code != expected {
		t.Fatalf("expected business code %d, got %d body=%s", expected, result.Code, recorder.Body.String())
	}
}

func assertInboxRouteDoesNotContain(t *testing.T, recorder *ut.ResponseRecorder, forbidden ...string) {
	t.Helper()
	if recorder.Code != 200 {
		t.Fatalf("unexpected HTTP status: %d body=%s", recorder.Code, recorder.Body.String())
	}
	var result struct {
		Code int `json:"code"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &result); err != nil {
		t.Fatalf("decode response: %v body=%s", err, recorder.Body.String())
	}
	if result.Code != apperrors.CodeSuccess {
		t.Fatalf("expected success, got %d body=%s", result.Code, recorder.Body.String())
	}
	for _, value := range forbidden {
		if strings.Contains(recorder.Body.String(), value) {
			t.Fatalf("response leaked %q: %s", value, recorder.Body.String())
		}
	}
}
