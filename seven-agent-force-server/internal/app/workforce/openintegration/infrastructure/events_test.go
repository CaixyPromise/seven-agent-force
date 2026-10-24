package infrastructure

import (
	"strings"
	"testing"

	"github.com/CaixyPromise/seven-agent/force/internal/app/workforce/openintegration/domain"
	secretvalue "github.com/CaixyPromise/seven-agent/force/internal/infrastructure/security/crypto/secretvalue"
)

func TestAttachSigningCredentialsRequiresOneCurrentAndOptionalGrace(t *testing.T) {
	claims := []DeliveryClaim{{Delivery: domain.Delivery{
		ID: 8_600_000_000_000_000_001, TenantID: 8_600_000_000_000_000_002, ApplicationID: 8_600_000_000_000_000_003,
	}}}
	credentials := []signingCredentialRecord{
		{ID: 8_600_000_000_000_000_004, TenantID: claims[0].Delivery.TenantID, ApplicationID: claims[0].Delivery.ApplicationID, Status: domain.CredentialCurrent, Secret: secretvalue.SecretValue{Plain: "current"}},
		{ID: 8_600_000_000_000_000_005, TenantID: claims[0].Delivery.TenantID, ApplicationID: claims[0].Delivery.ApplicationID, Status: domain.CredentialGrace, Secret: secretvalue.SecretValue{Plain: "grace"}},
	}
	if err := attachSigningCredentials(claims, credentials); err != nil {
		t.Fatal(err)
	}
	claim := claims[0]
	if claim.CurrentSigningCredential.ID != credentials[0].ID || claim.Delivery.SigningCredentialID != credentials[0].ID {
		t.Fatalf("current credential not attached: %+v", claim)
	}
	if claim.GraceSigningCredential == nil || claim.GraceSigningCredential.ID != credentials[1].ID || claim.Delivery.GraceSigningCredentialID == nil || *claim.Delivery.GraceSigningCredentialID != credentials[1].ID {
		t.Fatalf("grace credential not attached: %+v", claim)
	}
}

func TestAttachSigningCredentialsFailsClosedWithoutUniqueCurrent(t *testing.T) {
	baseClaim := DeliveryClaim{Delivery: domain.Delivery{ID: 101, TenantID: 102, ApplicationID: 103}}
	tests := []struct {
		name        string
		credentials []signingCredentialRecord
		want        string
	}{
		{name: "missing", credentials: []signingCredentialRecord{{ID: 105, TenantID: 102, ApplicationID: 103, Status: domain.CredentialGrace}}, want: "CURRENT row is missing"},
		{name: "duplicate", credentials: []signingCredentialRecord{{ID: 104, TenantID: 102, ApplicationID: 103, Status: domain.CredentialCurrent}, {ID: 105, TenantID: 102, ApplicationID: 103, Status: domain.CredentialCurrent}}, want: "duplicate CURRENT"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			claims := []DeliveryClaim{baseClaim}
			err := attachSigningCredentials(claims, test.credentials)
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("error=%v want %q", err, test.want)
			}
			if claims[0].Delivery.SigningCredentialID != 0 || claims[0].CurrentSigningCredential.ID != 0 {
				t.Fatalf("failed claim retained signing credential: %+v", claims[0])
			}
		})
	}
}

func TestDistinctDeliveryApplicationIDsDeduplicatesBatch(t *testing.T) {
	claims := []DeliveryClaim{
		{Delivery: domain.Delivery{ApplicationID: 3}},
		{Delivery: domain.Delivery{ApplicationID: 3}},
		{Delivery: domain.Delivery{ApplicationID: 7}},
	}
	got := distinctDeliveryApplicationIDs(claims)
	if len(got) != 2 || got[0] != 3 || got[1] != 7 {
		t.Fatalf("application IDs=%v", got)
	}
}

func TestDeliverySigningCredentialIDsIncludesCurrentThenGrace(t *testing.T) {
	graceID := int64(9_007_199_254_740_994)
	got := deliverySigningCredentialIDs(domain.Delivery{
		SigningCredentialID: 9_007_199_254_740_995, GraceSigningCredentialID: &graceID,
	})
	if len(got) != 2 || got[0] != 9_007_199_254_740_995 || got[1] != graceID {
		t.Fatalf("signing credential IDs=%v", got)
	}
	got = deliverySigningCredentialIDs(domain.Delivery{SigningCredentialID: 9_007_199_254_740_995})
	if len(got) != 1 || got[0] != 9_007_199_254_740_995 {
		t.Fatalf("current-only signing credential IDs=%v", got)
	}
}

func TestAttachSigningCredentialsRejectsDuplicateGraceAndCrossTenantRows(t *testing.T) {
	claim := DeliveryClaim{Delivery: domain.Delivery{ID: 101, TenantID: 102, ApplicationID: 103}}
	tests := []struct {
		name        string
		credentials []signingCredentialRecord
		want        string
	}{
		{
			name: "duplicate active grace",
			credentials: []signingCredentialRecord{
				{ID: 104, TenantID: 102, ApplicationID: 103, Status: domain.CredentialCurrent},
				{ID: 105, TenantID: 102, ApplicationID: 103, Status: domain.CredentialGrace},
				{ID: 106, TenantID: 102, ApplicationID: 103, Status: domain.CredentialGrace},
			},
			want: "duplicate active GRACE",
		},
		{
			name: "cross tenant",
			credentials: []signingCredentialRecord{
				{ID: 104, TenantID: 999, ApplicationID: 103, Status: domain.CredentialCurrent},
			},
			want: "scope is invalid",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			claims := []DeliveryClaim{claim}
			err := attachSigningCredentials(claims, test.credentials)
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("error=%v want %q", err, test.want)
			}
		})
	}
}
