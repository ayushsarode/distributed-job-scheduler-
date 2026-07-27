package auth

import (
	"context"
	"errors"

	"exiro.ai/application/assert"
)

type sessionKey struct{}

func SetSessionId(ctx context.Context, sessionId string) context.Context {
	assert.True(sessionId != "")
	ctx = context.WithValue(ctx, sessionKey{}, sessionId)
	return ctx
}

func GetSessionId(ctx context.Context) (string, error) {
	val, ok := ctx.Value(sessionKey{}).(string)
	if !ok {
		return "", errors.New("failed to get session id from ctx")
	}

	return val, nil
}
