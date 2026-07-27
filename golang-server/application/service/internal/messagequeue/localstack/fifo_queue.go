package localstack

import (
	"context"

	"exiro.ai/application/assert"
	"exiro.ai/application/service/internal/types"
	appConf "exiro.ai/config"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/sqs"
	sqsTypes "github.com/aws/aws-sdk-go-v2/service/sqs/types"

	"github.com/rs/zerolog"
)

const (
	localstackFifoMaxNumberOfMessages = 10
	localstackFifoWaitTimeSeconds     = 20
)

type localstackFIFOSQS struct {
	client    *sqs.Client
	queueURL  *string
	queueName string
	logger    *zerolog.Logger
}

var (
	_ types.FIFOMessageQueue    = (*localstackFIFOSQS)(nil)
	_ types.FIFOMessageConsumer = (*localstackFIFOSQS)(nil)
)

// PublishStringMessage implements types.FIFOMessageQueue.
func (l *localstackFIFOSQS) PublishStringMessage(ctx context.Context, message string, messageGroupID string, deduplicationID string) error {
	_, err := l.client.SendMessage(ctx, &sqs.SendMessageInput{
		QueueUrl:               l.queueURL,
		MessageBody:            aws.String(message),
		MessageGroupId:         aws.String(messageGroupID),
		MessageDeduplicationId: aws.String(deduplicationID),
	})
	if err != nil {
		l.logger.Error().
			Ctx(ctx).
			Err(err).
			Str("queue_name", l.queueName).
			Str("queue_url", aws.ToString(l.queueURL)).
			Msg("failed to send message to FIFO queue")
	}
	return err
}

// ConsumeStringMessage implements types.FIFOMessageConsumer.
func (l *localstackFIFOSQS) ConsumeStringMessage(ctx context.Context, callback func(ctx context.Context, message string, messageGroupID string) error) error {
	l.logger.Info().Ctx(ctx).Str("queue_name", l.queueName).Msg("starting FIFO message processor")

	for {
		result, err := l.client.ReceiveMessage(ctx, &sqs.ReceiveMessageInput{
			QueueUrl:            l.queueURL,
			MaxNumberOfMessages: localstackFifoMaxNumberOfMessages,
			WaitTimeSeconds:     localstackFifoWaitTimeSeconds,
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
				l.logger.Error().Err(err).Msg("Failed to process message. Continuing...")
				continue
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

func NewLocalStackFIFOSQS(ctx context.Context, queueName string) (*localstackFIFOSQS, error) {
	cfg, err := config.LoadDefaultConfig(ctx,
		config.WithRegion(defaultAWSRegion),
	)
	assert.NoError(err)

	sqsClient := sqs.NewFromConfig(cfg, func(o *sqs.Options) {
		o.BaseEndpoint = aws.String(appConf.Ctx(ctx).LocalStackEndpoint)
	})

	_, err = sqsClient.CreateQueue(ctx, &sqs.CreateQueueInput{
		QueueName: aws.String(queueName),
		Attributes: map[string]string{
			"FifoQueue": "true",
		},
	})
	assert.NoError(err)

	// Get the Queue URL
	gQInput := &sqs.GetQueueUrlInput{
		QueueName: aws.String(queueName),
	}
	result, err := sqsClient.GetQueueUrl(ctx, gQInput)
	assert.NoError(err)

	queueURL := result.QueueUrl

	return &localstackFIFOSQS{
		client:    sqsClient,
		queueURL:  queueURL,
		queueName: queueName,
		logger:    zerolog.Ctx(ctx),
	}, nil
}
