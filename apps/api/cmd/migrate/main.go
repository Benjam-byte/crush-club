package main

import (
	"context"
	"database/sql"
	"fmt"
	"os"

	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/pressly/goose/v3"
)

func main() {
	if len(os.Args) < 2 {
		panic("usage: migrate <up|down|status>")
	}
	databaseURL := os.Getenv("DATABASE_URL")
	if databaseURL == "" {
		panic("DATABASE_URL is required")
	}

	db, err := sql.Open("pgx", databaseURL)
	if err != nil {
		panic(err)
	}
	defer db.Close()

	if err := db.PingContext(context.Background()); err != nil {
		panic(err)
	}

	goose.SetDialect("postgres")
	command := os.Args[1]
	if err := goose.Run(command, db, "migrations"); err != nil {
		panic(fmt.Errorf("migration %s failed: %w", command, err))
	}
}
