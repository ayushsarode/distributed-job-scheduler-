package aws

import (
	"context"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/sqs"
)

const (
	sqsMaxNumberOfMessages = 10
	sqsWaitTimeSeconds     = 20
)

func (q *Queue) ConsumeStringMessage(ctx context.Context, callback func(ctx context.Context, message string) error) error {
	for {
		result, err := q.client.ReceiveMessage(ctx, &sqs.ReceiveMessageInput{
			QueueUrl:            &q.queueURL,
			MaxNumberOfMessages: sqsMaxNumberOfMessages,
			WaitTimeSeconds:     sqsWaitTimeSeconds,
		})
		if err != nil {
			return err
		}

		for _, msg := range result.Messages {
			messageBody := aws.ToString(msg.Body)
			err := callback(ctx, messageBody)
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
