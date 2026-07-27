package auth

import (
	"context"
	"errors"
	"os"

	"exiro.ai/config"
	"github.com/rs/zerolog"

	fireauth "github.com/LewisWatson/firebase-jwt-auth"
)

type userKey struct{}

// Initialalize jwt client here once.

func SetUser(ctx context.Context, user string) context.Context {
	ctx = context.WithValue(ctx, userKey{}, user)
	return ctx
}

func GetUser(ctx context.Context) (string, error) {
	val, ok := ctx.Value(userKey{}).(string)
	if !ok {
		return "", errors.New("failed to get user from ctx")
	}
	return val, nil
}

func MustGetUser(ctx context.Context) string {
	user, err := GetUser(ctx)
	if err != nil {
		panic(err)
	}
	return user
}

type TokenVerifier interface {
	Verify(token string) (string, error)
}

type tokenVerifier struct {
	verifier *fireauth.FireAuth
}

func (t *tokenVerifier) Verify(token string) (string, error) {
	// if !config.C.ProductionMode {
	// 	return token, nil
	// }

	uid, _, err := t.verifier.Verify(token)
	if err != nil {
		return "", err
	}
	return uid, err
}

func NewTokenVerifier(ctx context.Context) (TokenVerifier, error) {
	logger := zerolog.Ctx(ctx)

	cfg := config.Ctx(ctx)
	if cfg.TestMode {
		logger.Info().Ctx(ctx).Msg("Test mode enabled: using MockTokenVerifier")
		return NewMockTokenVerifier(cfg.TestUserID), nil
	}

	projectID := os.Getenv("FIREBASE_PROJECT_ID")
	if projectID == "" {
		logger.Error().Ctx(ctx).Msg("FIREBASE_PROJECT_ID environment variable not set")
		return nil, errors.New("FIREBASE_PROJECT_ID environment variable not set")
	}

	verifier, err := fireauth.New(projectID)
	if err != nil {
		logger.Error().Ctx(ctx).Err(err).Msg("Failed to initialize Firebase verifier")
		return nil, errors.New("failed to initialize Firebase verifier: %w")
	}

	return &tokenVerifier{verifier: verifier}, nil
}
