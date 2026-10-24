package infrastructure

import (
	"strings"
	"testing"
)

func TestNotificationPostgresRendererQuotesActionableMessageColumns(t *testing.T) {
	query := `SELECT primaryActionLabelTemplate, presentationType, actionType, actionLabelSnapshot, actionStyle, actionUrlSnapshot, renderedPayloadDigest FROM sys_notification_delivery`
	rendered := notificationPostgresRenderer.RenderPostgres(query)

	for _, identifier := range []string{
		"primaryActionLabelTemplate",
		"presentationType",
		"actionType",
		"actionLabelSnapshot",
		"actionStyle",
		"actionUrlSnapshot",
		"renderedPayloadDigest",
	} {
		if !strings.Contains(rendered, `"`+identifier+`"`) {
			t.Fatalf("expected %s to be quoted in %s", identifier, rendered)
		}
	}
}

func TestNotificationPostgresRendererQuotesEnterpriseTargetAuthorityColumns(t *testing.T) {
	query := `INSERT INTO sys_notification_external_target (externalIdentityId, externalIdentityVersion, authorityDigest, grantType) VALUES (?, ?, ?, ?)`
	rendered := notificationPostgresRenderer.RenderPostgres(query)

	for _, identifier := range []string{
		"externalIdentityId",
		"externalIdentityVersion",
		"authorityDigest",
		"grantType",
	} {
		if !strings.Contains(rendered, `"`+identifier+`"`) {
			t.Fatalf("expected %s to be quoted in %s", identifier, rendered)
		}
	}
}
