package transaction

import (
	"context"
	"errors"

	"github.com/rs/zerolog"
	"exiro.ai/infra/database/postgres"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type txKey struct{}

type TransactionHandler struct {
	db *pgxpool.Pool
}

// NewTransactionHandler creates a new transaction handler with the given database connection.
func NewTransactionHandler(ctx context.Context) *TransactionHandler {
	db := postgres.Ctx(ctx)
	return &TransactionHandler{db: db}
}

// WithTransaction executes the given function within a database transaction.
func (th *TransactionHandler) WithTransaction(ctx context.Context, fn func(ctx context.Context) error) error {
	tx, err := th.db.Begin(ctx)
	if err != nil {
		return err
	}

	logger := zerolog.Ctx(ctx)

	// Add the transaction to the context so repositories can access it
	ctxWithTx := context.WithValue(ctx, txKey{}, tx)

	err = fn(ctxWithTx)
	if err != nil {
		logger.Err(err).Msg("unable to execute transaction callback")
		errT := tx.Rollback(ctx)
		if errT != nil {
			// TODO: use xerrors
			logger.Err(err).Msg("unable to rollback transaction")
			return errors.Join(err, errT)
		}
		return err
	}
	err = tx.Commit(ctx)
	if err != nil {
		logger.Err(err).Msg("unable to commit transaction")
		return err
	}
	return nil
}

// TxFromContext extracts the transaction from context if it exists.
func TxFromContext(ctx context.Context) (pgx.Tx, bool) {
	tx, ok := ctx.Value(txKey{}).(pgx.Tx)
	return tx, ok
}
