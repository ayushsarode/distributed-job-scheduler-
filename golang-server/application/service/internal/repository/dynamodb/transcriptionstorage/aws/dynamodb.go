package aws

import (
	"context"

	"exiro.ai/application/assert"

	appConf "exiro.ai/config"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
)

func NewAWSDynamoDB(ctx context.Context) (*dynamodb.Client, error) {
	region := appConf.Ctx(ctx).AWSRegion
	if region == "" {
		region = "us-east-1"
	}

	cfg, err := config.LoadDefaultConfig(ctx,
		config.WithRegion(region),
	)
	assert.NoError(err)

	return dynamodb.NewFromConfig(cfg), nil
}
