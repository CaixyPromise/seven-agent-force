package errors

import (
	"net/http"
	"testing"
)

func TestServiceUnavailableUsesNumericCodeAndHTTP503(t *testing.T) {
	err := ServiceUnavailable("")
	if err.Code() != CodeServiceUnavailable {
		t.Fatalf("code=%d want %d", err.Code(), CodeServiceUnavailable)
	}
	if got := HTTPStatus(err); got != http.StatusServiceUnavailable {
		t.Fatalf("http status=%d want %d", got, http.StatusServiceUnavailable)
	}
}
