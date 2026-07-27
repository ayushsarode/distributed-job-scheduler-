package objectstore

import (
	"context"
	"fmt"
	"errors"

	"exiro.ai/application/service/internal/objectstore/aws"
	"exiro.ai/application/service/internal/objectstore/localstack"
	"exiro.ai/application/service/internal/types"
	"exiro.ai/config"
)

func NewObjectStore(ctx context.Context, bucketName string) (types.ObjectStore, error) {
	switch config.Ctx(ctx).ObjectStoreType {
	case config.LocalStack:
		return localstack.NewLocalstackS3(ctx, bucketName), nil
	case config.S3:
		s3Client, err := aws.NewAWSS3(ctx, config.Ctx(ctx).AWSRegion)
		if err != nil {
			return nil, fmt.Errorf("failed to create AWS S3 client: %w", err)
		}
		return s3Client, nil
	case config.GCS:
		return nil, errors.New("GCS not implemented")
	default:
		return nil, fmt.Errorf("invalid object store type: %s", config.Ctx(ctx).ObjectStoreType)
	}
}
