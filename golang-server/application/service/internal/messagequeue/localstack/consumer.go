package localstack

import (
	"context"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/sqs"
)

const (
	localstackSqsMaxNumberOfMessages = 10
	localstackSqsWaitTimeSeconds     = 20
)

func (l *localstackSQS) ConsumeStringMessage(ctx context.Context, callback func(ctx context.Context, message string) error) error {
	l.logger.Info().Ctx(ctx).Str("queue_name", l.queueName).Msg("starting message processor")

	for {
		result, err := l.client.ReceiveMessage(ctx, &sqs.ReceiveMessageInput{
			QueueUrl:            l.queueURL,
			MaxNumberOfMessages: localstackSqsMaxNumberOfMessages,
			WaitTimeSeconds:     localstackSqsWaitTimeSeconds,
		})
		if err != nil {
			return err
		}

		for _, msg := range result.Messages {
			messageBody := aws.ToString(msg.Body)
			err := callback(ctx, messageBody) // @TODO: Instead of continuing, we should retry 5 times and then delete the message.
			if err != nil {
				l.logger.Error().Err(err).Msg("Failed to process message. Continuing...")
				// continue
			}
			_, delErr := l.client.DeleteMessage(ctx, &sqs.DeleteMessageInput{
				QueueUrl:      l.queueURL,
				ReceiptHandle: msg.ReceiptHandle,
			})
			if delErr != nil {
				l.logger.Error().Err(delErr).Msg("Failed to delete message. Continuing...")
				continue
			}
		}
	}
}
