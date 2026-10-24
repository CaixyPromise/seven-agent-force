package infrastructure

import "testing"

func TestDeliveryIntentStatusConvergesFromAuthoritativeDelivery(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name, requestStatus, requestReason, mode, reviewStatus, want string
	}{
		{"branch delivered", "SUCCEEDED", "REMOTE_REF_VERIFIED", "NEW_BRANCH", "", "DELIVERED"},
		{"draft review delivered", "SUCCEEDED", "REMOTE_REF_VERIFIED", "NEW_BRANCH_AND_DRAFT_REVIEW", "SUCCEEDED", "DELIVERED"},
		{"draft review still running", "SUCCEEDED", "REMOTE_REF_VERIFIED", "NEW_BRANCH_AND_DRAFT_REVIEW", "RUNNING", "DELIVERY_RUNNING"},
		{"draft review failed", "SUCCEEDED", "REMOTE_REF_VERIFIED", "NEW_BRANCH_AND_DRAFT_REVIEW", "MANUAL_REVIEW", "DELIVERY_FAILED"},
		{"push failed", "FAILED", "PUSH_FAILED", "NEW_BRANCH", "", "DELIVERY_FAILED"},
		{"unknown push", "OUTCOME_UNKNOWN", "REMOTE_REF_UNAVAILABLE", "NEW_BRANCH", "", "OUTCOME_UNKNOWN"},
		{"unknown draft review", "SUCCEEDED", "REMOTE_REF_VERIFIED", "NEW_BRANCH_AND_DRAFT_REVIEW", "OUTCOME_UNKNOWN", "OUTCOME_UNKNOWN"},
		{"skipped", "CANCELLED", "USER_SKIPPED", "NEW_BRANCH", "", "SKIPPED"},
		{"cancelled", "CANCELLED", "USER_CANCELLED", "NEW_BRANCH", "", "CANCELLED"},
		{"awaiting approval", "WAITING_APPROVAL", "", "NEW_BRANCH", "", "WAITING_USER_DECISION"},
		{"running", "CONFIRMING", "", "NEW_BRANCH", "", "DELIVERY_RUNNING"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if got := deliveryIntentStatus(tc.requestStatus, tc.requestReason, tc.mode, tc.reviewStatus); got != tc.want {
				t.Fatalf("deliveryIntentStatus(%q,%q,%q,%q)=%q, want %q", tc.requestStatus, tc.requestReason, tc.mode, tc.reviewStatus, got, tc.want)
			}
		})
	}
}
