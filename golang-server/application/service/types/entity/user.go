package entity

import "github.com/google/uuid"

type User struct {
	UserID    string
	FirstName string
	LastName  string
	TenantID  uuid.UUID
}
