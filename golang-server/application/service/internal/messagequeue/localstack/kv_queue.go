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

type LocalStackKVQueue struct {
	client    *sqs.Client
	queueURL  *string
	logger    *zerolog.Logger
}

var _ types.KVMessageQueue = (*LocalStackKVQueue)(nil)

func (q *LocalStackKVQueue) PublishStringMessage(ctx context.Context, msg string) error {
	_, err := q.client.SendMessage(ctx, &sqs.SendMessageInput{
		QueueUrl:    q.queueURL,
		MessageBody: aws.String(msg),
	})
	if err != nil {
		q.logger.Error().
			Ctx(ctx).
			Err(err).
			Str("queue_url", aws.ToString(q.queueURL)).
			Msg("LocalStackKVQueue: failed to send message")
	}
	return err
}

func NewLocalStackKVQueue(ctx context.Context, queueName string) (*LocalStackKVQueue, error) {
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

	result, err := sqsClient.GetQueueUrl(ctx, &sqs.GetQueueUrlInput{
		QueueName: aws.String(queueName),
	})
	assert.NoError(err)

	return &LocalStackKVQueue{
		client:   sqsClient,
		queueURL: result.QueueUrl,
		logger:   zerolog.Ctx(ctx),
	}, nil
}