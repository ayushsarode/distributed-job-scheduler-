package migrations

import (
	"embed"
	"fmt"
	"os"
	"sync"
	"context"

	"exiro.ai/config"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/stdlib"
	"github.com/pressly/goose/v3"
)

//go:embed *.sql
var embedMigrations embed.FS
var once sync.Once

func Migrate(ctx context.Context) {
	once.Do(func() {
		config, err := pgx.ParseConfig(config.Ctx(ctx).DatabaseURL)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Unable to parse database config: %v\n", err)
			os.Exit(1)
		}

		db := stdlib.OpenDB(*config)
		defer func() {
			_ = db.Close()
		}()

		goose.SetBaseFS(embedMigrations)

		if err := goose.SetDialect("postgres"); err != nil {
			panic(err)
		}

		if err := goose.Up(db, ".", goose.WithAllowMissing()); err != nil {
			panic(err)
		}
	})
}
