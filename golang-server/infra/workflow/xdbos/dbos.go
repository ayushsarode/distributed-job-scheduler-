package xdbos

import (
	"context"

	"exiro.ai/application/assert"
	"github.com/dbos-inc/dbos-transact-golang/dbos"
)

type dbosCtxKey struct{}

var ctxKey = dbosCtxKey{}

func WithContext(ctx context.Context, databaseURL string) context.Context {
	dbosContext, err := dbos.NewDBOSContext(ctx, dbos.Config{
		AppName:     "exiro-go-backend",
		DatabaseURL: databaseURL,
	})
	assert.NoError(err)
	return context.WithValue(ctx, ctxKey, dbosContext)
}

func Ctx(ctx context.Context) dbos.DBOSContext {
	dbosContext := ctx.Value(ctxKey)
	assert.NotNil(dbosContext)
	return dbosContext.(dbos.DBOSContext)
}
