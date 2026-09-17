package main

import (
	"context"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/request"
	"github.com/aws/aws-sdk-go/service/sqs"
	"github.com/jeffotoni/log"
)

type retrySQSFake struct {
	input *sqs.ChangeMessageVisibilityInput
}

func (f *retrySQSFake) ReceiveMessageWithContext(context.Context, *sqs.ReceiveMessageInput, ...request.Option) (*sqs.ReceiveMessageOutput, error) {
	return nil, nil
}

func (f *retrySQSFake) DeleteMessageWithContext(context.Context, *sqs.DeleteMessageInput, ...request.Option) (*sqs.DeleteMessageOutput, error) {
	return nil, nil
}

func (f *retrySQSFake) ChangeMessageVisibilityWithContext(_ context.Context, input *sqs.ChangeMessageVisibilityInput, _ ...request.Option) (*sqs.ChangeMessageVisibilityOutput, error) {
	f.input = input
	return &sqs.ChangeMessageVisibilityOutput{}, nil
}

func TestReceiveCount(t *testing.T) {
	message := &sqs.Message{Attributes: map[string]*string{
		"ApproximateReceiveCount": aws.String("3"),
	}}
	if got := receiveCount(message); got != 3 {
		t.Fatalf("receive count=%d want=3", got)
	}
	if got := receiveCount(&sqs.Message{Attributes: map[string]*string{
		"ApproximateReceiveCount": aws.String("invalid"),
	}}); got != 1 {
		t.Fatalf("invalid receive count=%d want=1", got)
	}
}

func TestRetryDelayUsesCappedExponentialBackoff(t *testing.T) {
	base := 2 * time.Second
	max := 5 * time.Second
	tests := []struct {
		attempt int
		want    time.Duration
	}{
		{attempt: 1, want: 2 * time.Second},
		{attempt: 2, want: 4 * time.Second},
		{attempt: 3, want: 5 * time.Second},
		{attempt: 4, want: 5 * time.Second},
	}
	for _, test := range tests {
		if got := retryDelay(test.attempt, base, max); got != test.want {
			t.Fatalf("attempt=%d delay=%s want=%s", test.attempt, got, test.want)
		}
	}
}

func TestRetryMessageChangesVisibilityUsingReceiveCount(t *testing.T) {
	client := &retrySQSFake{}
	consumer := &Consumer{
		client:    client,
		logger:    log.New(log.Config{Level: log.DEBUG, ServiceName: "consumer-test"}),
		queueURL:  "queue-url",
		retryBase: 2 * time.Second,
		retryMax:  10 * time.Second,
	}
	message := &sqs.Message{
		MessageId:     aws.String("message-1"),
		ReceiptHandle: aws.String("receipt-1"),
		Attributes: map[string]*string{
			"ApproximateReceiveCount": aws.String("2"),
		},
	}

	consumer.retryMessage(context.Background(), message)

	if client.input == nil {
		t.Fatal("ChangeMessageVisibility was not called")
	}
	if aws.StringValue(client.input.QueueUrl) != "queue-url" ||
		aws.StringValue(client.input.ReceiptHandle) != "receipt-1" ||
		aws.Int64Value(client.input.VisibilityTimeout) != 4 {
		t.Fatalf("unexpected visibility input: %+v", client.input)
	}
}
