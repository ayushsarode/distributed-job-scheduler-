package transcriptionstorage

import (
	"context"
	"errors"

	"exiro.ai/application/assert"
	"exiro.ai/application/service/internal/repository/dynamodb/transcriptionstorage/aws"
	"exiro.ai/application/service/internal/repository/dynamodb/transcriptionstorage/localstack"
	"exiro.ai/config"
	sdkaws "github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
)

func createDynamoDBTable(ctx context.Context, client *dynamodb.Client) error {
	// Create table if it doesn't exist
	_, err := client.CreateTable(ctx, &dynamodb.CreateTableInput{
		TableName: &config.Ctx(ctx).TranscriptionStorage.TranscriptionTableName,
		AttributeDefinitions: []types.AttributeDefinition{
			{AttributeName: sdkaws.String("session_id"), AttributeType: types.ScalarAttributeTypeS},
			{AttributeName: sdkaws.String("sort_key"), AttributeType: types.ScalarAttributeTypeS},
			{AttributeName: sdkaws.String("owner_id"), AttributeType: types.ScalarAttributeTypeS},
			{AttributeName: sdkaws.String("status"), AttributeType: types.ScalarAttributeTypeS},
			{AttributeName: sdkaws.String("last_updated"), AttributeType: types.ScalarAttributeTypeN},
		},
		KeySchema: []types.KeySchemaElement{
			{AttributeName: sdkaws.String("session_id"), KeyType: types.KeyTypeHash},
			{AttributeName: sdkaws.String("sort_key"), KeyType: types.KeyTypeRange},
		},
		BillingMode: types.BillingModePayPerRequest,
		GlobalSecondaryIndexes: []types.GlobalSecondaryIndex{
			{
				IndexName: sdkaws.String("OwnerIdLastUpdatedIndex"),
				KeySchema: []types.KeySchemaElement{
					{AttributeName: sdkaws.String("owner_id"), KeyType: types.KeyTypeHash},
					{AttributeName: sdkaws.String("last_updated"), KeyType: types.KeyTypeRange},
				},
				Projection: &types.Projection{
					ProjectionType: types.ProjectionTypeKeysOnly,
				},
			},
			{
				IndexName: sdkaws.String("StatusLastUpdatedIndex"),
				KeySchema: []types.KeySchemaElement{
					{AttributeName: sdkaws.String("status"), KeyType: types.KeyTypeHash},
					{AttributeName: sdkaws.String("last_updated"), KeyType: types.KeyTypeRange},
				},
				Projection: &types.Projection{
					ProjectionType: types.ProjectionTypeKeysOnly,
				},
			},
		},
	})
	if err != nil {
		// Ignore "table already exists" errors
		var rne *types.ResourceInUseException
		if !errors.As(err, &rne) {
			return err
		}
		return nil
	}
	return nil
}

// NewDynamoDBClient creates a new DynamoDB client based on configuration.
func NewDynamoDBClient(ctx context.Context) *dynamodb.Client {
	useLocalStack := config.Ctx(ctx).TranscriptionStorage.UseLocalStack

	if useLocalStack {
		client, err := localstack.NewLocalStackDynamoDB(ctx)
		assert.NoError(err)
		err = createDynamoDBTable(ctx, client)
		assert.NoError(err)
		return client
	} else {
		client, err := aws.NewAWSDynamoDB(ctx)
		assert.NoError(err)
		return client
	}
}
