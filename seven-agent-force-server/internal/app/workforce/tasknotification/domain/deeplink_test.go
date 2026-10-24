package domain

import "testing"

func TestBuildTaskDeepLinkForDeliveryAndReview(t *testing.T) {
	t.Parallel()
	for _, test := range []struct {
		source SourceType
		want   string
	}{
		{SourceDeliveryRequest, "/workforce/tasks/9007199254740993?turn=9007199254740995&focus=delivery&delivery=9007199254740997"},
		{SourceReviewOperation, "/workforce/tasks/9007199254740993?turn=9007199254740995&focus=delivery&review=9007199254740997"},
	} {
		got, err := BuildTaskDeepLinkForSource(9007199254740993, 9007199254740995, FocusDelivery, test.source, 9007199254740997)
		if err != nil {
			t.Fatal(err)
		}
		if got != test.want {
			t.Fatalf("source=%s got=%s want=%s", test.source, got, test.want)
		}
	}
}
