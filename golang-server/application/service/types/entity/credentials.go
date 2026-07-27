package entity

import (
	"fmt"
	"time"

	"exiro.ai/application/models/pb"
	"github.com/google/uuid"
)

type CredentialType int

const (
	CredentialTypeUnspecified CredentialType = iota
	CredentialTypeTwilio
	CredentialTypeExotel
)

// String returns the string representation of CredentialType.
func (c CredentialType) String() string {
	switch c {
	case CredentialTypeUnspecified:
		return "CREDENTIAL_TYPE_UNSPECIFIED"
	case CredentialTypeTwilio:
		return "TWILIO"
	case CredentialTypeExotel:
		return "EXOTEL"
	default:
		return "CREDENTIAL_TYPE_UNSPECIFIED"
	}
}

// ParseCredentialType parses a string into CredentialType.
func ParseCredentialType(s string) (CredentialType, error) {
	switch s {
	case "TWILIO":
		return CredentialTypeTwilio, nil
	case "EXOTEL":
		return CredentialTypeExotel, nil
	default:
		return CredentialTypeUnspecified, fmt.Errorf("invalid credential type: %s", s)
	}
}

type CredentialEncrypted struct {
	ID                 uuid.UUID
	TenantID           uuid.UUID
	CredentialName     string
	Type               CredentialType
	CredentialMetadata *pb.CredentialMetadata
	EncryptedPayload   []byte // Encrypted protojson of pb.Credential
	EncryptedDataKey   []byte // KMS-encrypted data encryption key
	Nonce              []byte
	CreatedBy          string
	CreatedAt          time.Time
	UpdatedAt          time.Time
}

type Credential struct {
	ID                 uuid.UUID
	TenantID           uuid.UUID
	CredentialName     string
	Type               CredentialType
	CredentialMetadata *pb.CredentialMetadata
	Credential         *pb.Credential // Decrypted proto message
	CreatedBy          string
	CreatedAt          time.Time
	UpdatedAt          time.Time
}
