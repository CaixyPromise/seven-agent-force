package domain

import (
	"fmt"
	"regexp"
	"strings"
)

const (
	DeliveryDiagnosticReasonIncident        = "INCIDENT"
	DeliveryDiagnosticReasonCustomerSupport = "CUSTOMER_SUPPORT"
	DeliveryDiagnosticReasonSecurityReview  = "SECURITY_REVIEW"
	DeliveryDiagnosticReasonOther           = "OTHER"
)

const (
	DeliveryDiagnosticResultAllowed         = "ALLOWED"
	DeliveryDiagnosticResultDenied          = "DENIED"
	DeliveryDiagnosticResultExpired         = "CONTENT_EXPIRED"
	DeliveryDiagnosticResultStepUpRequired  = "STEP_UP_REQUIRED"
	DeliveryDiagnosticResultTransportDenied = "TRANSPORT_DENIED"
	DeliveryDiagnosticResultNotFound        = "NOT_FOUND"
)

var deliveryDiagnosticTicketReferencePattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._:/#-]{0,127}$`)

// NormalizeDeliveryContentTier returns the fail-closed diagnostic tier for a
// delivery. Historical or malformed values become SENSITIVE rather than
// accidentally becoming public.
func NormalizeDeliveryContentTier(value string) string {
	switch strings.ToUpper(strings.TrimSpace(value)) {
	case DeliveryContentTierPublic:
		return DeliveryContentTierPublic
	case DeliveryContentTierSecretEphemeral:
		return DeliveryContentTierSecretEphemeral
	case DeliveryContentTierSensitive:
		fallthrough
	default:
		return DeliveryContentTierSensitive
	}
}

// ContentTierForTemplateVariables derives the least-visible diagnostic tier
// from a published template's immutable variable classification. A template
// without declared variables remains SENSITIVE: literal content has no
// explicit public declaration, so it must not become readable by accident.
// SECRET_EPHEMERAL is intentionally not derived here; regular versioned
// templates reject that type and OTP-like flows use the separate short-lived
// encrypted payload path.
func ContentTierForTemplateVariables(variables []TemplateVariable) string {
	if len(variables) == 0 {
		return DeliveryContentTierSensitive
	}
	for _, variable := range variables {
		if strings.ToUpper(strings.TrimSpace(variable.Classification)) != TemplateVariableClassificationPublic {
			return DeliveryContentTierSensitive
		}
	}
	return DeliveryContentTierPublic
}

// ValidateDeliveryDiagnosticReason validates the small, content-free reason
// contract that accompanies every diagnostic read.
func ValidateDeliveryDiagnosticReason(reasonCode, ticketReference string) (string, string, error) {
	reasonCode = strings.ToUpper(strings.TrimSpace(reasonCode))
	switch reasonCode {
	case DeliveryDiagnosticReasonIncident,
		DeliveryDiagnosticReasonCustomerSupport,
		DeliveryDiagnosticReasonSecurityReview,
		DeliveryDiagnosticReasonOther:
	default:
		return "", "", fmt.Errorf("诊断用途不合法")
	}
	ticketReference = strings.TrimSpace(ticketReference)
	if ticketReference != "" && !deliveryDiagnosticTicketReferencePattern.MatchString(ticketReference) {
		return "", "", fmt.Errorf("工单编号格式不合法")
	}
	return reasonCode, ticketReference, nil
}

// DeliveryDiagnosticPermission returns the additional capability required to
// read the resolved content tier. The general diagnostic permission is
// checked separately by the HTTP boundary.
func DeliveryDiagnosticPermission(contentTier string) string {
	switch NormalizeDeliveryContentTier(contentTier) {
	case DeliveryContentTierPublic:
		return "system:notification:delivery:content:public"
	case DeliveryContentTierSecretEphemeral:
		return "system:notification:delivery:content:secret-ephemeral"
	default:
		return "system:notification:delivery:content:sensitive"
	}
}

// DeliveryDiagnosticRequiresStepUp keeps public content usable for the
// smallest diagnostic role while requiring a new AAL2 proof for sensitive and
// short-lived-secret content.
func DeliveryDiagnosticRequiresStepUp(contentTier string) bool {
	return NormalizeDeliveryContentTier(contentTier) != DeliveryContentTierPublic
}
