package localstack

import (
	"context"
	"fmt"
	"io"
	"time"

	"exiro.ai/application/service/internal/types"
	appConf "exiro.ai/config"
	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	s3Types "github.com/aws/aws-sdk-go-v2/service/s3/types"
)

const localstackS3CorsMaxAgeSeconds int32 = 3000
type localstackS3 struct {
	client     *s3.Client
	bucketName string
}

var _ (types.ObjectStore) = (*localstackS3)(nil)

// PutObject implements types.ObjectStore.
func (l *localstackS3) PutObject(
	ctx context.Context,
	objectBody io.Reader,
	bucket string,
	objectKey string,
	contentType string,
) (string, error) {
	_, err := l.client.PutObject(ctx, &s3.PutObjectInput{
		Bucket:      aws.String(bucket),
		Key:         aws.String(objectKey),
		Body:        objectBody,
		ContentType: aws.String(contentType),
	})
	if err != nil {
		return "", err
	}

	return fmt.Sprintf("s3://%s/%s", bucket, objectKey), nil
}
func (l *localstackS3) GetObject(
	ctx context.Context,
	bucket string,
	objectKey string,
) (io.ReadCloser, error) {
	resp, err := l.client.GetObject(ctx, &s3.GetObjectInput{
		Bucket: aws.String(bucket),
		Key:    aws.String(objectKey),
	})
	if err != nil {
		return nil, err
	}

	return resp.Body, nil
}

// GetSignedURL implements types.ObjectStore.
func (l *localstackS3) GetSignedURL(
	ctx context.Context,
	bucket string,
	objectKey string,
	expires time.Duration,
) (string, error) {
	presignClient := s3.NewPresignClient(l.client)

	request, err := presignClient.PresignGetObject(ctx, &s3.GetObjectInput{
		Bucket: aws.String(bucket),
		Key:    aws.String(objectKey),
	}, s3.WithPresignExpires(expires))

	if err != nil {
		return "", fmt.Errorf("failed to generate presigned URL: %w", err)
	}

	return request.URL, nil
}

func NewLocalstackS3(ctx context.Context, bucketName string) types.ObjectStore {
	awsEndpoint := appConf.Ctx(ctx).LocalStackEndpoint
	awsRegion := "us-east-1"

	awsCfg, err := config.LoadDefaultConfig(ctx,
		config.WithRegion(awsRegion),
	)
	if err != nil {
		// TODO: Use xerrors to create error
		panic(err)
	}

	s3Client := s3.NewFromConfig(awsCfg, func(o *s3.Options) {
		o.UsePathStyle = true
		o.BaseEndpoint = aws.String(awsEndpoint)
	})

	_, err = s3Client.CreateBucket(ctx, &s3.CreateBucketInput{
		Bucket: aws.String(bucketName),
	})
	if err != nil {
		panic(err)
	}

	corsRules := []s3Types.CORSRule{
		{
			AllowedMethods: []string{
				"PUT",
				"POST",
				"DELETE",
			},
			AllowedOrigins: []string{"http://localhost:3000", "https://exiro.ai", "https://www.exiro.ai"},
			AllowedHeaders: []string{"*"},
		},
		{
			AllowedMethods: []string{
				"GET",
			},
			AllowedOrigins: []string{"http://localhost:3000", "https://exiro.ai", "https://www.exiro.ai"},
			ExposeHeaders:  []string{"x-amz-server-side-encryption"},
			MaxAgeSeconds:  aws.Int32(localstackS3CorsMaxAgeSeconds),
		},
	}

	_, err = s3Client.PutBucketCors(ctx, &s3.PutBucketCorsInput{
		Bucket: aws.String(bucketName),
		CORSConfiguration: &s3Types.CORSConfiguration{
			CORSRules: corsRules,
		},
	})
	if err != nil {
		panic(err)
	}

	return &localstackS3{
		client:     s3Client,
		bucketName: bucketName,
	}
}
