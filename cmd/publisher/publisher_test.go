package main

import (
	"testing"
	"time"

	"github.com/jeffotoni/jungle-backend/internal/application/ports"
)

func TestRetryDelay(t *testing.T) {
	tests := []struct {
		name     string
		attempts int
		want     time.Duration
	}{
		{name: "first retry", attempts: 0, want: time.Second},
		{name: "second retry", attempts: 1, want: 2 * time.Second},
		{name: "capped retry", attempts: 10, want: time.Minute},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := retryDelay(time.Second, time.Minute, tt.attempts); got != tt.want {
				t.Fatalf("retryDelay()=%s want=%s", got, tt.want)
			}
		})
	}
}

func TestMessageInputForFIFOQueue(t *testing.T) {
	publisher := &Publisher{
		queueURL: "http://localhost:4566/000000000000/events.fifo",
		groupID:  "wallet-events",
	}
	input := publisher.messageInput(ports.OutboxRecord{
		ID:      "event-1",
		Payload: []byte(`{"eventId":"event-1"}`),
	})

	if input.MessageGroupId == nil || *input.MessageGroupId != "wallet-events" {
		t.Fatalf("unexpected message group: %v", input.MessageGroupId)
	}
	if input.MessageDeduplicationId == nil || *input.MessageDeduplicationId != "event-1" {
		t.Fatalf("unexpected deduplication id: %v", input.MessageDeduplicationId)
	}
}

func TestMessageInputForStandardQueue(t *testing.T) {
	publisher := &Publisher{queueURL: "http://localhost:4566/000000000000/events"}
	input := publisher.messageInput(ports.OutboxRecord{
		ID:      "event-1",
		Payload: []byte(`{"eventId":"event-1"}`),
	})

	if input.MessageGroupId != nil || input.MessageDeduplicationId != nil {
		t.Fatal("standard queue must not receive FIFO attributes")
	}
}
