package handlers

import (
	"context"
	"errors"
	"testing"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/request"
	"github.com/aws/aws-sdk-go/service/sqs"
)

type healthSQSFake struct {
	err      error
	queueURL string
}

func (f *healthSQSFake) GetQueueAttributesWithContext(
	_ context.Context,
	input *sqs.GetQueueAttributesInput,
	_ ...request.Option,
) (*sqs.GetQueueAttributesOutput, error) {
	f.queueURL = aws.StringValue(input.QueueUrl)
	return &sqs.GetQueueAttributesOutput{}, f.err
}

func TestCheckSQS(t *testing.T) {
	client := &healthSQSFake{}
	if err := checkSQS(context.Background(), client, "queue-url"); err != nil {
		t.Fatal(err)
	}
	if client.queueURL != "queue-url" {
		t.Fatalf("queue URL=%q", client.queueURL)
	}
}

func TestCheckSQSReturnsDependencyError(t *testing.T) {
	want := errors.New("sqs unavailable")
	client := &healthSQSFake{err: want}
	if err := checkSQS(context.Background(), client, "queue-url"); !errors.Is(err, want) {
		t.Fatalf("error=%v want=%v", err, want)
	}
}
