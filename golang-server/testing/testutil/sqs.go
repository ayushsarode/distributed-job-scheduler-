package testutil

import (
	"context"
	"fmt"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/sqs"
)

const defaultAWSRegion = "us-east-1"

// SendSQSMessage sends a message to the named SQS queue on the given
// LocalStack endpoint. It is a generic helper that can be reused by any
// test suite that needs to push messages onto an SQS queue.
func SendSQSMessage(ctx context.Context, localstackEndpoint string, queueName string, body string) error {
	cfg, err := config.LoadDefaultConfig(ctx,
		config.WithRegion(defaultAWSRegion),
	)
	if err != nil {
		return fmt.Errorf("failed to load AWS config: %w", err)
	}

	sqsClient := sqs.NewFromConfig(cfg, func(o *sqs.Options) {
		o.BaseEndpoint = aws.String(localstackEndpoint)
	})

	queueURLResult, err := sqsClient.GetQueueUrl(ctx, &sqs.GetQueueUrlInput{
		QueueName: aws.String(queueName),
	})
	if err != nil {
		return fmt.Errorf("failed to get queue URL for %s: %w", queueName, err)
	}

	_, err = sqsClient.SendMessage(ctx, &sqs.SendMessageInput{
		QueueUrl:    queueURLResult.QueueUrl,
		MessageBody: aws.String(body),
	})
	if err != nil {
		return fmt.Errorf("failed to send SQS message: %w", err)
	}

	return nil
}
