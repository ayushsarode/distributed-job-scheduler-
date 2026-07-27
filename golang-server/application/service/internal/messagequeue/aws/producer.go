package aws

import (
	"context"

	"exiro.ai/application/assert"
	"exiro.ai/application/service/internal/types"
	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/sqs"
	"github.com/rs/zerolog"
)

type Queue struct {
	client   *sqs.Client
	queueURL string
	logger   *zerolog.Logger
}

var (
	_ types.MessageQueue    = (*Queue)(nil)
	_ types.MessageConsumer = (*Queue)(nil)
)

// PublishBytesMessage implements types.MessageQueue.
func (q *Queue) PublishBytesMessage(ctx context.Context, message []byte) error {
	panic("not supported")
}

// PublishStringMessage implements types.MessageQueue.
func (q *Queue) PublishStringMessage(ctx context.Context, message string) error {
	_, err := q.client.SendMessage(ctx, &sqs.SendMessageInput{
		MessageBody: aws.String(message),
		QueueUrl:    aws.String(q.queueURL),
	})
	return err
}

func NewQueue(ctx context.Context, queueName string) (*Queue, error) {
	cfg, err := config.LoadDefaultConfig(ctx)
	assert.NoError(err)

	client := sqs.NewFromConfig(cfg)
	queueURL, err := client.GetQueueUrl(ctx, &sqs.GetQueueUrlInput{
		QueueName: &queueName,
	})
	assert.NoError(err)

	return &Queue{
		client:   client,
		queueURL: *queueURL.QueueUrl,
		logger:   zerolog.Ctx(ctx),
	}, nil
}
