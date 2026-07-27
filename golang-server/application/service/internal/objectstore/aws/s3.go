package aws

import (
	"context"
	"fmt"
	"io"
	"time"

	"exiro.ai/application/service/internal/types"
	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/s3"
)

type AWSS3 struct {
	client *s3.Client
}

// PutObject implements types.ObjectStore.
func (s *AWSS3) PutObject(
	ctx context.Context,
	objectBody io.Reader,
	bucket string,
	objectKey string,
	contentType string,
) (string, error) {
	_, err := s.client.PutObject(ctx, &s3.PutObjectInput{
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

// GetObject implements types.ObjectStore.
func (s *AWSS3) GetObject(
	ctx context.Context,
	bucket string,
	objectKey string,
) (io.ReadCloser, error) {
	resp, err := s.client.GetObject(ctx, &s3.GetObjectInput{
		Bucket: aws.String(bucket),
		Key:    aws.String(objectKey),
	})
	if err != nil {
		return nil, err
	}

	return resp.Body, nil
}

// GetSignedURL implements types.ObjectStore.
func (s *AWSS3) GetSignedURL(
	ctx context.Context,
	bucket string,
	objectKey string,
	expires time.Duration,
) (string, error) {
	presignClient := s3.NewPresignClient(s.client)

	request, err := presignClient.PresignGetObject(ctx, &s3.GetObjectInput{
		Bucket: aws.String(bucket),
		Key:    aws.String(objectKey),
	}, s3.WithPresignExpires(expires))

	if err != nil {
		return "", fmt.Errorf("failed to generate presigned URL: %w", err)
	}

	return request.URL, nil
}

var _ types.ObjectStore = &AWSS3{}

func NewAWSS3(ctx context.Context, region string) (*AWSS3, error) {
	cfg, err := config.LoadDefaultConfig(ctx,
		config.WithRegion(region),
	)
	if err != nil {
		return nil, fmt.Errorf("unable to load SDK config: %w", err)
	}

	client := s3.NewFromConfig(cfg)

	return &AWSS3{
		client: client,
	}, nil
}
