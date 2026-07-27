package awskms

import (
	"context"

	"exiro.ai/application/service/internal/awskms/aws"
	"exiro.ai/application/service/internal/awskms/localstack"
	"exiro.ai/application/service/internal/types"
	"exiro.ai/config"
)

func NewEncryptionClient(ctx context.Context) types.EncryptionClient {
	if config.Ctx(ctx).ProductionMode {
		return aws.NewAWSKMSClient(ctx)
	}
	return localstack.NewLocalStackKMSClient(ctx)
}
