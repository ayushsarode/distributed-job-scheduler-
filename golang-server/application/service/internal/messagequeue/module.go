package messagequeue

import (
	"context"

	"exiro.ai/application/assert"
	"exiro.ai/application/service/internal/messagequeue/aws"
	"exiro.ai/application/service/internal/messagequeue/localstack"
	"exiro.ai/application/service/internal/types"
	"exiro.ai/config"
)

func NewAgentDeploymentMessageQueue(ctx context.Context) types.MessageQueue {
	switch config.Ctx(ctx).MessageQueueType {
	case "LocalStack":
		queue, err := localstack.NewLocalStackSQS(ctx, config.Ctx(ctx).AgentDeploymentSQSQueueName)
		assert.NoError(err)
		return queue
	case "SQS":
		queue, err := aws.NewQueue(ctx, config.Ctx(ctx).AgentDeploymentSQSQueueName)
		assert.NoError(err)
		return queue
	default:
		panic("Invalid message queue type: " + config.Ctx(ctx).MessageQueueType)
	}
}

func NewAgentDeploymentStatusConsumer(ctx context.Context) types.MessageConsumer {
	switch config.Ctx(ctx).MessageQueueType {
	case "LocalStack":
		queue, err := localstack.NewLocalStackSQS(ctx, config.Ctx(ctx).AgentDeploymentCheckSQSQueueName)
		assert.NoError(err)
		return queue
	case "SQS":
		queue, err := aws.NewQueue(ctx, config.Ctx(ctx).AgentDeploymentCheckSQSQueueName)
		assert.NoError(err)
		return queue
	default:
		panic("Invalid message queue type: " + config.Ctx(ctx).MessageQueueType)
	}
}


func NewTranscriptionFIFOMessageQueue(ctx context.Context) types.FIFOMessageQueue {
	switch config.Ctx(ctx).MessageQueueType {
	case "LocalStack":
		queue, err := localstack.NewLocalStackFIFOSQS(ctx, config.Ctx(ctx).TranscriptionStorage.TranscriptsQueueName)
		assert.NoError(err)
		return queue
	case "SQS":
		queue, err := aws.NewFIFOQueue(ctx, config.Ctx(ctx).TranscriptionStorage.TranscriptsQueueName)
		assert.NoError(err)
		return queue
	default:
		panic("Invalid message queue type: " + config.Ctx(ctx).MessageQueueType)
	}
}

func NewAgentKVMessageQueue(ctx context.Context) types.KVMessageQueue {
	switch config.Ctx(ctx).MessageQueueType {
	case "LocalStack":
		queue, err := localstack.NewLocalStackKVQueue(ctx, config.Ctx(ctx).TranscriptionStorage.AgentKVStoreQueueName)
		assert.NoError(err)
		return queue
	case "SQS":
		queue, err := aws.NewKVQueue(ctx, config.Ctx(ctx).TranscriptionStorage.AgentKVStoreQueueName)
		assert.NoError(err)
		return queue
	default:
		panic("Invalid message queue type: " + config.Ctx(ctx).MessageQueueType)
	}
}

func NewTranscriptionFIFOMessageConsumer(ctx context.Context) types.FIFOMessageConsumer {
	switch config.Ctx(ctx).ConsumerQueueType {
	case "LocalStack":
		queue, err := localstack.NewLocalStackFIFOSQS(ctx, config.Ctx(ctx).TranscriptionStorage.TranscriptsQueueName)
		assert.NoError(err)
		return queue
	case "SQS":
		queue, err := aws.NewFIFOQueue(ctx, config.Ctx(ctx).TranscriptionStorage.TranscriptsQueueName)
		assert.NoError(err)
		return queue
	default:
		panic("Invalid message queue type: " + config.Ctx(ctx).ConsumerQueueType)
	}
}
