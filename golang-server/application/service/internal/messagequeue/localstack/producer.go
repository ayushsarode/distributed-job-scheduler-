package localstack

import (
	"context"

	"exiro.ai/application/assert"
	"exiro.ai/application/service/internal/types"
	appConf "exiro.ai/config"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/sqs"
	"github.com/rs/zerolog"
)

const (
	defaultAWSRegion = "us-east-1"
)

type localstackSQS struct {
	client    *sqs.Client
	queueURL  *string
	queueName string
	logger    *zerolog.Logger
}

var (
	_ types.MessageQueue    = (*localstackSQS)(nil)
	_ types.MessageConsumer = (*localstackSQS)(nil)
)

// PublishBytesMessage implements types.MessageQueue.
func (l *localstackSQS) PublishBytesMessage(ctx context.Context, message []byte) error {
	panic("SQS does not support bytes messages")
}

// PublishStringMessage implements types.MessageQueue.
func (l *localstackSQS) PublishStringMessage(ctx context.Context, message string) error {
	_, err := l.client.SendMessage(ctx, &sqs.SendMessageInput{
		QueueUrl:    l.queueURL,
		MessageBody: aws.String(message),
	})
	if err != nil {
		l.logger.Error().
			Ctx(ctx).
			Err(err).
			Str("queue_name", l.queueName).
			Str("queue_url", aws.ToString(l.queueURL)).
			Msg("failed to send message to queue")
	}
	return err
}

func NewLocalStackSQS(ctx context.Context, queueName string) (*localstackSQS, error) {
	cfg, err := config.LoadDefaultConfig(ctx,
		config.WithRegion(defaultAWSRegion),
	)
	assert.NoError(err)

	sqsClient := sqs.NewFromConfig(cfg, func(o *sqs.Options) {
		o.BaseEndpoint = aws.String(appConf.Ctx(ctx).LocalStackEndpoint)
	})

	_, err = sqsClient.CreateQueue(ctx, &sqs.CreateQueueInput{
		QueueName: aws.String(queueName),
	})
	assert.NoError(err)

	// Get the Queue URL
	gQInput := &sqs.GetQueueUrlInput{
		QueueName: aws.String(queueName),
	}
	result, err := sqsClient.GetQueueUrl(ctx, gQInput)
	assert.NoError(err)

	queueURL := result.QueueUrl

	return &localstackSQS{
		client:    sqsClient,
		queueURL:  queueURL,
		queueName: queueName,
		logger:    zerolog.Ctx(ctx),
	}, nil
}
