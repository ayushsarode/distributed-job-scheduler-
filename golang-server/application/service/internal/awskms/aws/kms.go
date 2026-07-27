package aws

import (
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"errors"
	"io"

	"exiro.ai/application/assert"
	xerrors "exiro.ai/application/errors"
	"exiro.ai/application/service/internal/types"
	appConf "exiro.ai/config"
	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/kms"
	kmsTypes "github.com/aws/aws-sdk-go-v2/service/kms/types"
)

type AWSKMSClient struct {
	client *kms.Client
	keyID  string
}

var _ types.EncryptionClient = &AWSKMSClient{}

func NewAWSKMSClient(ctx context.Context) *AWSKMSClient {
	region := appConf.Ctx(ctx).AWSRegion
	if region == "" {
		region = "us-east-1"
	}

	cfg, err := config.LoadDefaultConfig(ctx,
		config.WithRegion(region),
	)
	assert.NoError(err)

	keyID := appConf.Ctx(ctx).CredentialService.AWSKMSKeyId
	assert.NotEmpty(keyID, "AWS KMS Key ID must be configured in production mode")

	return &AWSKMSClient{
		client: kms.NewFromConfig(cfg),
		keyID:  keyID,
	}
}

func (c *AWSKMSClient) Encrypt(ctx context.Context, data []byte) (*types.EncryptedData, error) {
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

func (c *AWSKMSClient) Decrypt(ctx context.Context, encryptedData *types.EncryptedData) ([]byte, error) {
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
