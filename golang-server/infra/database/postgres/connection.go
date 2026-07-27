package postgres

import (
	"context"
	"fmt"

	"exiro.ai/config"
	"github.com/jackc/pgx/v5/pgxpool"
)

type pgxPoolContextKey struct{}
var ctxKey = pgxPoolContextKey{}

func WithContext(ctx context.Context) context.Context {
	conn, err := pgxpool.New(ctx, config.Ctx(ctx).DatabaseURL)
	if err != nil {
		panic(fmt.Errorf("failed to connect to postgres: %w", err))
	}

	return context.WithValue(ctx, ctxKey, conn)
}

func Ctx(ctx context.Context) *pgxpool.Pool {
	conn := ctx.Value(ctxKey)
	if conn == nil {
		panic("no connection found in context")
	}

	return conn.(*pgxpool.Pool)
}
