package agentkv

import (
	"context"
	"errors"

	"exiro.ai/application/service/internal/repository/dynamodb/transcriptionstorage"
	"exiro.ai/config"
	sdkaws "github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
)

func createDynamoDBTable(ctx context.Context, client *dynamodb.Client) error {
	tableName := config.Ctx(ctx).TranscriptionStorage.AgentKVStoreTableName
	_, err := client.CreateTable(ctx, &dynamodb.CreateTableInput{
		TableName: sdkaws.String(tableName),
		AttributeDefinitions: []types.AttributeDefinition{
			{AttributeName: sdkaws.String("session_id"), AttributeType: types.ScalarAttributeTypeS},
			{AttributeName: sdkaws.String("key"), AttributeType: types.ScalarAttributeTypeS},
		},
		KeySchema: []types.KeySchemaElement{
			{AttributeName: sdkaws.String("session_id"), KeyType: types.KeyTypeHash},
			{AttributeName: sdkaws.String("key"), KeyType: types.KeyTypeRange},
		},
		BillingMode: types.BillingModePayPerRequest,
	})
	if err != nil {
		var rne *types.ResourceInUseException
		if !errors.As(err, &rne) {
			return err
		}
		return nil
	}
	return nil
}

func NewDynamoDBClient(ctx context.Context) *dynamodb.Client {
	client := transcriptionstorage.NewDynamoDBClient(ctx)
	if config.Ctx(ctx).TranscriptionStorage.UseLocalStack {
		_ = createDynamoDBTable(ctx, client)
	}
	return client
}
