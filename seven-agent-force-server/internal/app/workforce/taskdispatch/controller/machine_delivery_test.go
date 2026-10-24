package controller

import (
	"errors"
	"net/http"
	"testing"

	deliveryapp "github.com/CaixyPromise/seven-agent/force/internal/app/workforce/delivery/application"
	deliverydomain "github.com/CaixyPromise/seven-agent/force/internal/app/workforce/delivery/domain"
)

func TestDeliveryErrorStatusSeparatesBusinessConflictsFromInfrastructureFailures(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name       string
		err        error
		wantStatus int
		wantCode   string
	}{
		{name: "not found", err: deliveryapp.ErrNotFound, wantStatus: http.StatusNotFound, wantCode: "DELIVERY_NOT_FOUND"},
		{name: "policy", err: deliverydomain.ErrPolicyDenied, wantStatus: http.StatusConflict, wantCode: "DELIVERY_POLICY_DENIED"},
		{name: "stale owner", err: errors.New("DELIVERY_OWNER_STALE"), wantStatus: http.StatusConflict, wantCode: "DELIVERY_OWNER_STALE"},
		{name: "sql", err: errors.New("failed to encode args[6]: cannot encode text"), wantStatus: http.StatusInternalServerError, wantCode: "DELIVERY_PERSIST_FAILED"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			status, code := deliveryErrorStatus(tc.err)
			if status != tc.wantStatus || code != tc.wantCode {
				t.Fatalf("got (%d, %q), want (%d, %q)", status, code, tc.wantStatus, tc.wantCode)
			}
		})
	}
}
