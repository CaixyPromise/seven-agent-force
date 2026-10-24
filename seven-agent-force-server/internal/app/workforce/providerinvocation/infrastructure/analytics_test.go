package infrastructure

import "testing"

func TestPercentChange(t *testing.T) {
	if value := percentChange(15, 10); value == nil || *value != 50 {
		t.Fatalf("percentChange(15,10)=%v, want 50", value)
	}
	if value := percentChange(5, 10); value == nil || *value != -50 {
		t.Fatalf("percentChange(5,10)=%v, want -50", value)
	}
	if value := percentChange(1, 0); value != nil {
		t.Fatalf("percentChange without baseline=%v, want nil", *value)
	}
}
