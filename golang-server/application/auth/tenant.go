package auth

import (
	"context"
	"errors"

	"github.com/google/uuid"
)

type tenantKey struct{}

func SetTenant(ctx context.Context, tenantID uuid.UUID) context.Context {
	return context.WithValue(ctx, tenantKey{}, tenantID)
}

func GetTenant(ctx context.Context) (uuid.UUID, error) {
	val, ok := ctx.Value(tenantKey{}).(uuid.UUID)
	if !ok {
		return uuid.Nil, errors.New("failed to get tenant from ctx")
	}
	return val, nil
}

func MustGetTenant(ctx context.Context) uuid.UUID {
	tenant, err := GetTenant(ctx)
	if err != nil {
		panic(err)
	}
	return tenant
}
