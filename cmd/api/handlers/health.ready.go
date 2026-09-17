package handlers

import (
	"context"
	"net/http"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/request"
	"github.com/aws/aws-sdk-go/service/sqs"
	"github.com/jeffotoni/quick"
)

type SQSHealthClient interface {
	GetQueueAttributesWithContext(context.Context, *sqs.GetQueueAttributesInput, ...request.Option) (*sqs.GetQueueAttributesOutput, error)
}

func (r *Routes) ready(c *quick.Ctx) error {
	if r.pool == nil || r.sqs == nil || r.queueURL == "" {
		return c.Status(http.StatusServiceUnavailable).JSON(map[string]string{"status": "not_ready"})
	}
	ctx, cancel := context.WithTimeout(c.Ctx(), r.healthTimeout)
	defer cancel()
	if err := r.pool.Ping(ctx); err != nil {
		return c.Status(http.StatusServiceUnavailable).JSON(map[string]string{"status": "not_ready"})
	}
	if err := checkSQS(ctx, r.sqs, r.queueURL); err != nil {
		return c.Status(http.StatusServiceUnavailable).JSON(map[string]string{"status": "not_ready"})
	}
	return c.Status(http.StatusOK).JSON(map[string]string{"status": "ready"})
}

func checkSQS(ctx context.Context, client SQSHealthClient, queueURL string) error {
	_, err := client.GetQueueAttributesWithContext(ctx, &sqs.GetQueueAttributesInput{
		QueueUrl:       aws.String(queueURL),
		AttributeNames: []*string{aws.String("QueueArn")},
	})
	return err
}
