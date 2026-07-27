package localstack

import (
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"io"
	"errors"

	"exiro.ai/application/assert"
	xerrors "exiro.ai/application/errors"
	"exiro.ai/application/service/internal/types"
	appConf "exiro.ai/config"
	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/kms"
	kmsTypes "github.com/aws/aws-sdk-go-v2/service/kms/types"
	"github.com/rs/zerolog"
)

type LocalStackKMSClient struct {
	client *kms.Client
	keyID  string
}

var _ types.EncryptionClient = &LocalStackKMSClient{}

func NewLocalStackKMSClient(ctx context.Context) *LocalStackKMSClient {
	region := appConf.Ctx(ctx).AWSRegion
	if region == "" {
		region = "us-east-1"
	}

	cfg, err := config.LoadDefaultConfig(ctx,
		config.WithRegion(region),
	)
	assert.NoError(err)

	client := kms.NewFromConfig(cfg, func(o *kms.Options) {
		o.BaseEndpoint = aws.String(appConf.Ctx(ctx).LocalStackEndpoint)
	})

	// Create or get KMS key
	keyID, err := createOrGetKMSKey(ctx, client, "exiro-credentials")
	assert.NoError(err)

	return &LocalStackKMSClient{
		client: client,
		keyID:  keyID,
	}
}

func (c *LocalStackKMSClient) Encrypt(ctx context.Context, data []byte) (*types.EncryptedData, error) {
	// Generate data key using KMS
	generateKeyOutput, err := c.client.GenerateDataKey(ctx, &kms.GenerateDataKeyInput{
		KeyId:   aws.String(c.keyID),
		KeySpec: kmsTypes.DataKeySpecAes256,
	})
	if err != nil {
		return nil, xerrors.InternalError(ctx, errors.New("failed to generate key"))
	}

	dekPlaintext := generateKeyOutput.Plaintext
	encryptedDEK := generateKeyOutput.CiphertextBlob

	// Create AES cipher
	block, err := aes.NewCipher(dekPlaintext)
	if err != nil {
		return nil, xerrors.InternalError(ctx, errors.New("failed to generate cipher"))
	}

	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, xerrors.InternalError(ctx, errors.New("failed to encrypt"))
	}

	// Generate random IV (nonce)
	iv := make([]byte, gcm.NonceSize())
	if _, err := io.ReadFull(rand.Reader, iv); err != nil {
		return nil, xerrors.InternalError(ctx, errors.New("failed to add randomness"))
	}

	encryptedPayload := gcm.Seal(nil, iv, data, nil)

	return &types.EncryptedData{
		EncryptedPayload: encryptedPayload,
		EncryptedDataKey: encryptedDEK,
		IV:               iv,
	}, nil
}

func (c *LocalStackKMSClient) Decrypt(ctx context.Context, encryptedData *types.EncryptedData) ([]byte, error) {
	// Decrypt the data key using KMS
	decryptOutput, err := c.client.Decrypt(ctx, &kms.DecryptInput{
		CiphertextBlob: encryptedData.EncryptedDataKey,
		KeyId:          aws.String(c.keyID),
	})
	if err != nil {
		return nil, xerrors.InternalError(ctx, errors.New("failed to decrypt data key"))
	}

	dekPlaintext := decryptOutput.Plaintext

	// Create AES cipher
	block, err := aes.NewCipher(dekPlaintext)
	if err != nil {
		return nil, xerrors.InternalError(ctx, errors.New("failed to create cipher"))
	}

	gcm, err := cipher.NewGCM(block)
	if err != nil {
		// failed to create GCM
		return nil, xerrors.InternalError(ctx, errors.New("failed to create cipher"))
	}

	// Decrypt the payload
	plaintext, err := gcm.Open(nil, encryptedData.IV, encryptedData.EncryptedPayload, nil)
	if err != nil {
		return nil, xerrors.InternalError(ctx, errors.New("failed to decrypt"))
	}

	return plaintext, nil
}

func createOrGetKMSKey(ctx context.Context, client *kms.Client, keyAlias string) (string, error) {
	aliasName := "alias/" + keyAlias

	describeKeyOutput, err := client.DescribeKey(ctx, &kms.DescribeKeyInput{
		KeyId: aws.String(aliasName),
	})

	// If key exists, return it
	if err == nil && describeKeyOutput.KeyMetadata != nil {
		return *describeKeyOutput.KeyMetadata.KeyId, nil
	}

	// Create new key
	createKeyOutput, err := client.CreateKey(ctx, &kms.CreateKeyInput{
		Description: aws.String("LocalStack KMS key for credential encryption (auto-created)"),
		KeyUsage:    "ENCRYPT_DECRYPT",
	})
	if err != nil {
		return "", xerrors.InternalError(ctx, errors.New("failed to create KMS key"))
	}
	keyId := *createKeyOutput.KeyMetadata.KeyId

	_, err = client.CreateAlias(ctx, &kms.CreateAliasInput{
		AliasName:   aws.String(aliasName),
		TargetKeyId: aws.String(keyId),
	})
	if err != nil {
		// should work irrespective of ability to create alias
		zerolog.Ctx(ctx).Warn().Err(err).Str("alias", aliasName).Msg("failed to create alias for KMS key")
	}

	return keyId, nil
}
