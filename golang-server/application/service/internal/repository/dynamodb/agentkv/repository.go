package agentkv

import (
	"context"

	xerrors "exiro.ai/application/errors"
	"exiro.ai/application/service/types/entity"
	"exiro.ai/config"
	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/feature/dynamodb/attributevalue"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	dynamodbTypes "github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
	"github.com/rs/zerolog"
)

type AgentKVRepository struct {
	kvTableName string
	client      *dynamodb.Client
	logger      *zerolog.Logger
}

func NewAgentKVRepository(ctx context.Context) *AgentKVRepository {
	return &AgentKVRepository{
		kvTableName: config.Ctx(ctx).TranscriptionStorage.AgentKVStoreTableName,
		client:      NewDynamoDBClient(ctx),
		logger:      zerolog.Ctx(ctx),
	}
}

func (r *AgentKVRepository) QuerySessionKV(ctx context.Context, sessionID string, maxItems int32) ([]entity.AgentKVItem, error) {
	var all []entity.AgentKVItem
	var startKey map[string]dynamodbTypes.AttributeValue
	remaining := maxItems
	for remaining > 0 {
		limit := remaining
		out, err := r.client.Query(ctx, &dynamodb.QueryInput{
			TableName:              &r.kvTableName,
			KeyConditionExpression: aws.String("session_id = :sid"),
			ExpressionAttributeValues: map[string]dynamodbTypes.AttributeValue{
				":sid": &dynamodbTypes.AttributeValueMemberS{Value: sessionID},
			},
			ExclusiveStartKey: startKey,
			Limit:             &limit,
		})
		if err != nil {
			r.logger.Error().
				Err(err).
				Str("session_id", sessionID).
				Msg("AgentKVRepository: DynamoDB Query failed")
			return nil, xerrors.InternalError(ctx, err)
		}
		for _, item := range out.Items {
			var row entity.AgentKVItem
			if err := attributevalue.UnmarshalMap(item, &row); err != nil {
				r.logger.Error().
					Err(err).
					Str("session_id", sessionID).
					Msg("AgentKVRepository: failed to unmarshal KV item")
				return nil, xerrors.InternalError(ctx, err)
			}
			all = append(all, row)
			remaining--
			if remaining <= 0 {
				break
			}
		}
		if remaining <= 0 || out.LastEvaluatedKey == nil {
			break
		}
		startKey = out.LastEvaluatedKey
	}
	return all, nil
}
