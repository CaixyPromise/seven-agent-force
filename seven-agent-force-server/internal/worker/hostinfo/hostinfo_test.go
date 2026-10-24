package hostinfo

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestSampleCPUUsage(t *testing.T) {
	tests := []struct {
		name      string
		values    []float64
		err       error
		want      float64
		available bool
	}{
		{name: "measured zero", values: []float64{0}, want: 0, available: true},
		{name: "measured usage", values: []float64{42.5}, want: 42.5, available: true},
		{name: "sampler error", err: errors.New("sample failed")},
		{name: "missing value", values: nil},
		{name: "out of range", values: []float64{101}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			gotInterval := time.Duration(0)
			got, available := sampleCPUUsage(context.Background(), func(_ context.Context, interval time.Duration, perCPU bool) ([]float64, error) {
				gotInterval = interval
				if perCPU {
					t.Fatal("combined CPU usage must be requested")
				}
				return test.values, test.err
			})
			if gotInterval != time.Second {
				t.Fatalf("sample interval = %s, want 1s", gotInterval)
			}
			if got != test.want || available != test.available {
				t.Fatalf("sampleCPUUsage() = (%v, %v), want (%v, %v)", got, available, test.want, test.available)
			}
		})
	}
}
