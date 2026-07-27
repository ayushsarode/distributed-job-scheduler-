package localstack

import (
	"context"

	"exiro.ai/application/assert"

	appConf "exiro.ai/config"
	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
)

const (
	defaultAWSRegion = "us-east-1"
)

func NewLocalStackDynamoDB(ctx context.Context) (*dynamodb.Client, error) {
	cfg, err := config.LoadDefaultConfig(ctx,
		config.WithRegion(defaultAWSRegion),
	)
	assert.NoError(err)

	dynamoClient := dynamodb.NewFromConfig(cfg, func(o *dynamodb.Options) {
		o.BaseEndpoint = aws.String(appConf.Ctx(ctx).LocalStackEndpoint)
	})

	return dynamoClient, nil
}
