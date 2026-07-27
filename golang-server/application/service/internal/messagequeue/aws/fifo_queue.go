package aws

import (
	"context"

	"exiro.ai/application/assert"
	"exiro.ai/application/service/internal/types"
	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/sqs"
	sqsTypes "github.com/aws/aws-sdk-go-v2/service/sqs/types"

	"github.com/rs/zerolog"
)

const (
	fifoSqsMaxNumberOfMessages = 10
	fifoSqsWaitTimeSeconds     = 20
)


type FIFOQueue struct {
	client   *sqs.Client
	queueURL string
	logger   *zerolog.Logger
}

var (
	_ types.FIFOMessageQueue    = (*FIFOQueue)(nil)
	_ types.FIFOMessageConsumer = (*FIFOQueue)(nil)
)

// PublishStringMessage implements types.FIFOMessageQueue.
func (q *FIFOQueue) PublishStringMessage(ctx context.Context, message string, messageGroupID string, deduplicationID string) error {
	_, err := q.client.SendMessage(ctx, &sqs.SendMessageInput{
		MessageBody:            aws.String(message),
		QueueUrl:               aws.String(q.queueURL),
		MessageGroupId:         aws.String(messageGroupID),
		MessageDeduplicationId: aws.String(deduplicationID),
	})
	return err
}

// ConsumeStringMessage implements types.FIFOMessageConsumer.
func (q *FIFOQueue) ConsumeStringMessage(ctx context.Context, callback func(ctx context.Context, message string, messageGroupID string) error) error {
	for {
		result, err := q.client.ReceiveMessage(ctx, &sqs.ReceiveMessageInput{
			QueueUrl:            &q.queueURL,
			MaxNumberOfMessages: fifoSqsMaxNumberOfMessages,
			WaitTimeSeconds:     fifoSqsWaitTimeSeconds,
			AttributeNames:      []sqsTypes.QueueAttributeName{"MessageGroupId"}, // Request MessageGroupId attribute
		})
		if err != nil {
			return err
		}

		for _, msg := range result.Messages {
			messageBody := aws.ToString(msg.Body)
			messageGroupID := ""
			if msg.Attributes != nil {
				messageGroupID = msg.Attributes["MessageGroupId"]
			}

			err := callback(ctx, messageBody, messageGroupID)
			if err != nil {
				q.logger.Error().Err(err).Msg("Failed to process message. Continuing...")
				continue
			}

			// Message processed successfully, so Ack (delete) it from the queue.
			_, delErr := q.client.DeleteMessage(ctx, &sqs.DeleteMessageInput{
				QueueUrl:      &q.queueURL,
				ReceiptHandle: msg.ReceiptHandle,
			})
			if delErr != nil {
				q.logger.Error().Err(delErr).Msg("Failed to delete message. Continuing...")
				continue
			}
		}
	}
}

func NewFIFOQueue(ctx context.Context, queueName string) (*FIFOQueue, error) {
	cfg, err := config.LoadDefaultConfig(ctx)
	assert.NoError(err)

	client := sqs.NewFromConfig(cfg)
	queueURL, err := client.GetQueueUrl(ctx, &sqs.GetQueueUrlInput{
		QueueName: &queueName,
	})
	assert.NoError(err)

	return &FIFOQueue{
		client:   client,
		queueURL: *queueURL.QueueUrl,
		logger:   zerolog.Ctx(ctx),
	}, nil
}
