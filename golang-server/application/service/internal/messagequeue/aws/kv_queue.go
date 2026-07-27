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

// KVQueue is a standard (non-FIFO) SQS publisher used for KVStoreEvent messages sent to the agent-kv-store-queue.
type KVQueue struct {
	client   *sqs.Client
	queueURL string
	logger   *zerolog.Logger
}

var _ types.KVMessageQueue = (*KVQueue)(nil)

func (q *KVQueue) PublishStringMessage(ctx context.Context, msg string) error {
	_, err := q.client.SendMessage(ctx, &sqs.SendMessageInput{
		MessageBody: aws.String(msg),
		QueueUrl:    aws.String(q.queueURL),
	})
	if err != nil {
		q.logger.Error().Err(err).Msg("KVQueue: failed to publish message to agent-kv-store-queue")
	}
	return err
}

func NewKVQueue(ctx context.Context, queueName string) (*KVQueue, error) {
	cfg, err := config.LoadDefaultConfig(ctx)
	assert.NoError(err)

	client := sqs.NewFromConfig(cfg)
	queueURL, err := client.GetQueueUrl(ctx, &sqs.GetQueueUrlInput{
		QueueName: &queueName,
	})
	assert.NoError(err)

	return &KVQueue{
		client:   client,
		queueURL: *queueURL.QueueUrl,
		logger:   zerolog.Ctx(ctx),
	}, nil
}