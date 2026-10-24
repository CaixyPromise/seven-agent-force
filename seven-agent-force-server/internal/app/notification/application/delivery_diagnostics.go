package application

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/CaixyPromise/seven-agent/force/internal/app/notification/domain"
	"github.com/CaixyPromise/seven-agent/force/internal/app/notification/facade"
	apperrors "github.com/CaixyPromise/seven-agent/force/internal/common/errors"
	secretvalueinfra "github.com/CaixyPromise/seven-agent/force/internal/infrastructure/security/crypto/secretvalue"
	"github.com/CaixyPromise/seven-agent/force/internal/shared/stepup"
)

const deliveryDiagnosticBusinessAction = "NOTIFICATION_DELIVERY_CONTENT_VIEW"

// DeliveryDiagnosticReadCommand is an internal application command. The HTTP
// adapter derives the actor, trace, confirmed tier capability and fresh proof;
// none of those fields are caller-controlled API payload values.
type DeliveryDiagnosticReadCommand struct {
	DeliveryID        string
	ReasonCode        string
	TicketReference   string
	ActorID           int64
	TraceID           string
	GrantedPermission string
	StepUpProof       stepup.ProofMetadata
}

// DeliveryDiagnosticAuditCommand records an attempted read that stopped at
// the HTTP security boundary, such as a missing capability or insecure
// transport. It contains only low-cardinality, content-free metadata.
type DeliveryDiagnosticAuditCommand struct {
	DeliveryID      string
	ReasonCode      string
	TicketReference string
	ActorID         int64
	TraceID         string
	ContentTier     string
	ResultCode      string
}

type ephemeralRenderedContent struct {
	Subject  string `json:"subject,omitempty"`
	Text     string `json:"text,omitempty"`
	HTML     string `json:"html,omitempty"`
	Markdown string `json:"markdown,omitempty"`
}

// DeliveryDiagnosticOperationBinding binds a fresh proof to one delivery and
// its stated purpose. The binding deliberately contains no message content,
// target, provider value, credential, or secret.
func DeliveryDiagnosticOperationBinding(deliveryID, reasonCode, ticketReference string) string {
	return "delivery:" + strings.TrimSpace(deliveryID) +
		"|reason:" + strings.ToUpper(strings.TrimSpace(reasonCode)) +
		"|ticket:" + strings.TrimSpace(ticketReference)
}

// DeliveryDiagnosticBusinessAction exposes the stable step-up action without
// allowing callers to choose an arbitrary protected operation.
func DeliveryDiagnosticBusinessAction() string {
	return deliveryDiagnosticBusinessAction
}

// ListDeliveries returns only the safe delivery list. Detailed content is
// intentionally available through ReadDeliveryDiagnosticContent alone.
func (s *Service) ListDeliveries(ctx context.Context, query domain.DeliveryQuery) (*facade.PageResult[facade.DeliverySummaryRecord], error) {
	repo, err := s.deliveryDiagnosticsRepository()
	if err != nil {
		return nil, err
	}
	query.ScopeID = strings.TrimSpace(s.scopeID)
	items, total, err := repo.ListDeliverySummaries(ctx, query)
	if err != nil {
		return nil, err
	}
	records := make([]facade.DeliverySummaryRecord, 0, len(items))
	for _, item := range items {
		records = append(records, *mapDeliverySummary(item))
	}
	current, pageSize := normalizePage(query.Current, query.PageSize)
	return &facade.PageResult[facade.DeliverySummaryRecord]{Records: records, Total: total, Current: current, PageSize: pageSize}, nil
}

// DeliveryDiagnosticTier resolves one delivery in the service-owned scope so
// the HTTP adapter can choose the correct capability and proof boundary. A
// missing or cross-scope row uses the same controlled not-found result.
func (s *Service) DeliveryDiagnosticTier(ctx context.Context, deliveryID string) (string, error) {
	repo, err := s.deliveryDiagnosticsRepository()
	if err != nil {
		return "", err
	}
	delivery, err := repo.FindDeliveryForDiagnostic(ctx, strings.TrimSpace(s.scopeID), strings.TrimSpace(deliveryID))
	if err != nil {
		return "", err
	}
	if delivery == nil {
		return "", apperrors.NotFound("投递记录不存在")
	}
	return domain.NormalizeDeliveryContentTier(delivery.ContentTier), nil
}

// AuditDeliveryDiagnosticAttempt records a controlled denied, expired,
// missing or transport-refused diagnostic request. Audit persistence failing
// is returned to the caller so content access cannot fail open.
func (s *Service) AuditDeliveryDiagnosticAttempt(ctx context.Context, command DeliveryDiagnosticAuditCommand) error {
	reasonCode, ticketReference, err := domain.ValidateDeliveryDiagnosticReason(command.ReasonCode, command.TicketReference)
	if err != nil {
		return err
	}
	if strings.TrimSpace(command.DeliveryID) == "" || command.ActorID <= 0 || strings.TrimSpace(command.ResultCode) == "" {
		return fmt.Errorf("notification delivery diagnostic audit is invalid")
	}
	return s.insertDeliveryDiagnosticAudit(ctx, domain.DeliveryDiagnosticAudit{
		ID:              s.nextID(),
		ScopeID:         strings.TrimSpace(s.scopeID),
		DeliveryID:      strings.TrimSpace(command.DeliveryID),
		ActorID:         command.ActorID,
		ContentTier:     domain.NormalizeDeliveryContentTier(command.ContentTier),
		ReasonCode:      reasonCode,
		TicketReference: ticketReference,
		ResultCode:      strings.TrimSpace(command.ResultCode),
		TraceID:         strings.TrimSpace(command.TraceID),
	})
}

// ReadDeliveryDiagnosticContent returns one rendered delivery only after the
// handler has checked the tier-specific capability. Sensitive and ephemeral
// reads additionally require an AAL2 proof bound to this delivery and reason.
// The method always writes an allow/deny/expired audit record and never
// returns markup, raw payload JSON, raw target, provider response or secret
// envelope fields.
func (s *Service) ReadDeliveryDiagnosticContent(ctx context.Context, command DeliveryDiagnosticReadCommand) (*facade.DeliveryDiagnosticContent, error) {
	reasonCode, ticketReference, err := domain.ValidateDeliveryDiagnosticReason(command.ReasonCode, command.TicketReference)
	if err != nil {
		return nil, apperrors.Params(err.Error())
	}
	if command.ActorID <= 0 || strings.TrimSpace(command.DeliveryID) == "" {
		return nil, apperrors.Unauthorized("未登录或登录信息失效")
	}
	repo, err := s.deliveryDiagnosticsRepository()
	if err != nil {
		return nil, err
	}
	deliveryID := strings.TrimSpace(command.DeliveryID)
	delivery, err := repo.FindDeliveryForDiagnostic(ctx, strings.TrimSpace(s.scopeID), deliveryID)
	if err != nil {
		return nil, err
	}
	if delivery == nil {
		if auditErr := s.insertDeliveryDiagnosticAudit(ctx, domain.DeliveryDiagnosticAudit{
			ID:              s.nextID(),
			ScopeID:         strings.TrimSpace(s.scopeID),
			DeliveryID:      deliveryID,
			ActorID:         command.ActorID,
			ContentTier:     domain.DeliveryContentTierSensitive,
			ReasonCode:      reasonCode,
			TicketReference: ticketReference,
			ResultCode:      domain.DeliveryDiagnosticResultNotFound,
			TraceID:         strings.TrimSpace(command.TraceID),
		}); auditErr != nil {
			return nil, auditErr
		}
		return nil, apperrors.NotFound("投递记录不存在")
	}
	tier := domain.NormalizeDeliveryContentTier(delivery.ContentTier)
	expectedPermission := domain.DeliveryDiagnosticPermission(tier)
	if strings.TrimSpace(command.GrantedPermission) != expectedPermission {
		if auditErr := s.insertDeliveryDiagnosticAudit(ctx, domain.DeliveryDiagnosticAudit{
			ID: s.nextID(), ScopeID: strings.TrimSpace(s.scopeID), DeliveryID: deliveryID, ActorID: command.ActorID,
			ContentTier: tier, ReasonCode: reasonCode, TicketReference: ticketReference,
			ResultCode: domain.DeliveryDiagnosticResultDenied, TraceID: strings.TrimSpace(command.TraceID),
		}); auditErr != nil {
			return nil, auditErr
		}
		return nil, apperrors.PermissionDenied(expectedPermission)
	}
	if domain.DeliveryDiagnosticRequiresStepUp(tier) {
		binding := DeliveryDiagnosticOperationBinding(deliveryID, reasonCode, ticketReference)
		if err := stepup.Require(command.StepUpProof, deliveryDiagnosticBusinessAction, binding); err != nil {
			if auditErr := s.insertDeliveryDiagnosticAudit(ctx, domain.DeliveryDiagnosticAudit{
				ID: s.nextID(), ScopeID: strings.TrimSpace(s.scopeID), DeliveryID: deliveryID, ActorID: command.ActorID,
				ContentTier: tier, ReasonCode: reasonCode, TicketReference: ticketReference,
				ResultCode: domain.DeliveryDiagnosticResultStepUpRequired, TraceID: strings.TrimSpace(command.TraceID),
			}); auditErr != nil {
				return nil, auditErr
			}
			return nil, err
		}
	}

	content := &facade.DeliveryDiagnosticContent{DeliveryID: deliveryID, ContentTier: tier}
	if tier == domain.DeliveryContentTierSecretEphemeral {
		ephemeral, findErr := repo.FindDeliveryEphemeralContent(ctx, strings.TrimSpace(s.scopeID), deliveryID)
		if findErr != nil {
			return nil, findErr
		}
		if ephemeral == nil || !ephemeral.ExpiresAt.After(s.now()) {
			if auditErr := s.insertDeliveryDiagnosticAudit(ctx, domain.DeliveryDiagnosticAudit{
				ID: s.nextID(), ScopeID: strings.TrimSpace(s.scopeID), DeliveryID: deliveryID, ActorID: command.ActorID,
				ContentTier: tier, ReasonCode: reasonCode, TicketReference: ticketReference,
				ResultCode: domain.DeliveryDiagnosticResultExpired, TraceID: strings.TrimSpace(command.TraceID),
			}); auditErr != nil {
				return nil, auditErr
			}
			return nil, apperrors.Operation("短期秘密已过期").WithDetails(map[string]string{"reasonCode": domain.DeliveryDiagnosticResultExpired})
		}
		payload, decryptErr := s.decryptEphemeralDeliveryContent(ctx, *ephemeral)
		if decryptErr != nil {
			if auditErr := s.insertDeliveryDiagnosticAudit(ctx, domain.DeliveryDiagnosticAudit{
				ID: s.nextID(), ScopeID: strings.TrimSpace(s.scopeID), DeliveryID: deliveryID, ActorID: command.ActorID,
				ContentTier: tier, ReasonCode: reasonCode, TicketReference: ticketReference,
				ResultCode: domain.DeliveryDiagnosticResultDenied, TraceID: strings.TrimSpace(command.TraceID),
			}); auditErr != nil {
				return nil, auditErr
			}
			return nil, decryptErr
		}
		content.Subject = payload.Subject
		content.Text = payload.Text
		expiresAt := ephemeral.ExpiresAt
		content.ExpiresAt = &expiresAt
	} else {
		content.Subject = delivery.RenderedSubject
		content.Text = delivery.RenderedText
	}
	if auditErr := s.insertDeliveryDiagnosticAudit(ctx, domain.DeliveryDiagnosticAudit{
		ID: s.nextID(), ScopeID: strings.TrimSpace(s.scopeID), DeliveryID: deliveryID, ActorID: command.ActorID,
		ContentTier: tier, ReasonCode: reasonCode, TicketReference: ticketReference,
		ResultCode: domain.DeliveryDiagnosticResultAllowed, TraceID: strings.TrimSpace(command.TraceID),
	}); auditErr != nil {
		return nil, auditErr
	}
	return content, nil
}

func (s *Service) deliveryDiagnosticsRepository() (domain.DeliveryDiagnosticsRepository, error) {
	repo, ok := s.repo.(domain.DeliveryDiagnosticsRepository)
	if !ok || repo == nil {
		return nil, fmt.Errorf("notification delivery diagnostics repository is not configured")
	}
	return repo, nil
}

func (s *Service) insertDeliveryDiagnosticAudit(ctx context.Context, item domain.DeliveryDiagnosticAudit) error {
	repo, err := s.deliveryDiagnosticsRepository()
	if err != nil {
		return err
	}
	if item.CreateTime.IsZero() {
		item.CreateTime = s.now().UTC()
	}
	return repo.InsertDeliveryDiagnosticAudit(ctx, &item)
}

func (s *Service) persistEphemeralDeliveryContent(ctx context.Context, deliveryID string, expiresAt time.Time, content ephemeralRenderedContent) (*domain.DeliveryEphemeralContent, error) {
	if s == nil || s.secrets == nil {
		return nil, fmt.Errorf("notification ephemeral content encryption is not configured")
	}
	repo, err := s.deliveryDiagnosticsRepository()
	if err != nil {
		return nil, err
	}
	if strings.TrimSpace(deliveryID) == "" || strings.TrimSpace(s.scopeID) == "" || !expiresAt.After(s.now()) {
		return nil, fmt.Errorf("notification ephemeral content is invalid")
	}
	raw, err := json.Marshal(content)
	if err != nil {
		return nil, err
	}
	encrypted, err := s.secrets.EncryptString(ctx, string(raw))
	if err != nil {
		return nil, err
	}
	if strings.TrimSpace(encrypted.CiphertextB64) == "" || strings.TrimSpace(encrypted.EDEKB64) == "" || strings.TrimSpace(encrypted.WrapKeyRef) == "" {
		return nil, fmt.Errorf("notification ephemeral encryption returned an incomplete envelope")
	}
	item := &domain.DeliveryEphemeralContent{
		ID:         s.nextID(),
		DeliveryID: deliveryID,
		ScopeID:    strings.TrimSpace(s.scopeID),
		Ciphertext: encrypted.CiphertextB64,
		EDEK:       encrypted.EDEKB64,
		WrapKeyRef: encrypted.WrapKeyRef,
		ExpiresAt:  expiresAt.UTC(),
	}
	if err := repo.InsertDeliveryEphemeralContent(ctx, item); err != nil {
		return nil, err
	}
	return item, nil
}

func (s *Service) decryptEphemeralDeliveryContent(ctx context.Context, item domain.DeliveryEphemeralContent) (ephemeralRenderedContent, error) {
	if s == nil || s.secrets == nil {
		return ephemeralRenderedContent{}, fmt.Errorf("notification ephemeral content encryption is not configured")
	}
	plain, err := s.secrets.DecryptString(ctx, secretvalueinfra.SecretValue{
		CiphertextB64: item.Ciphertext,
		EDEKB64:       item.EDEK,
		WrapKeyRef:    item.WrapKeyRef,
	})
	if err != nil {
		return ephemeralRenderedContent{}, err
	}
	var content ephemeralRenderedContent
	if err := json.Unmarshal([]byte(plain), &content); err != nil {
		return ephemeralRenderedContent{}, fmt.Errorf("notification ephemeral payload is invalid")
	}
	return content, nil
}
