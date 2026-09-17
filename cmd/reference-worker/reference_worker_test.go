package main

import (
	"testing"
	"time"

	"github.com/jeffotoni/jungle-backend/internal/application/ports"
)

func TestReferenceRetryDelay(t *testing.T) {
	base := time.Second
	maximum := 5 * time.Second
	tests := []struct {
		attempts int
		want     time.Duration
	}{
		{attempts: 1, want: time.Second},
		{attempts: 2, want: 2 * time.Second},
		{attempts: 3, want: 4 * time.Second},
		{attempts: 4, want: maximum},
	}
	for _, test := range tests {
		if got := referenceRetryDelay(base, maximum, test.attempts); got != test.want {
			t.Fatalf("attempts=%d got=%s want=%s", test.attempts, got, test.want)
		}
	}
}

func TestReferenceExpired(t *testing.T) {
	pendingAt := time.Date(2026, 9, 16, 0, 0, 0, 0, time.UTC)
	worker := &ReferenceWorker{referenceTTL: time.Hour}

	if worker.referenceExpired(ports.WagerRecord{ReferencePendingAt: &pendingAt}, pendingAt.Add(59*time.Minute)) {
		t.Fatal("reference should still be within TTL")
	}
	if !worker.referenceExpired(ports.WagerRecord{ReferencePendingAt: &pendingAt}, pendingAt.Add(time.Hour)) {
		t.Fatal("reference should be expired at TTL")
	}
}
