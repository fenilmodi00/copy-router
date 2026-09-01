// Command initdb bootstraps the router's Postgres database and schema.
// Idempotent: safe to run repeatedly on an already-initialized database.
package main

import (
	"context"
	_ "embed"
	"fmt"
	"net/url"
	"os"
	"strings"
	"time"

	"workweave/router/internal/config"

	"github.com/jackc/pgx/v5"
)

// canonicalSchema is the single source of truth for the fresh-install
// schema, embedded so the binary carries it (mirrors db/init/00-create-schema.sql).
//
//go:embed schema.sql
var canonicalSchema string

func main() {
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	dsn := config.PostgresDSN()

	u, err := url.Parse(dsn)
	if err != nil {
		fatal("Cannot parse DATABASE_URL: %v", err)
	}

	dbName := strings.TrimPrefix(u.Path, "/")
	if dbName == "" {
		fatal("DATABASE_URL has no database name")
	}

	ensureDatabase(ctx, u, dbName)
	ensureSchema(ctx, dsn)
}

// ensureDatabase connects to the "postgres" maintenance database and creates
// the target database if it doesn't already exist.
func ensureDatabase(ctx context.Context, u *url.URL, dbName string) {
	adminURL := *u
	adminURL.Path = "/postgres"

	conn, err := pgx.Connect(ctx, adminURL.String())
	if err != nil {
		fatal("Cannot connect to postgres maintenance database: %v", err)
	}
	defer conn.Close(ctx)

	var exists bool
	err = conn.QueryRow(ctx,
		"SELECT EXISTS(SELECT 1 FROM pg_database WHERE datname = @db_name)",
		pgx.NamedArgs{"db_name": dbName},
	).Scan(&exists)
	if err != nil {
		fatal("Cannot check database existence: %v", err)
	}

	if exists {
		fmt.Printf("Database %q already exists\n", dbName)
		return
	}

	quotedName := pgx.Identifier{dbName}.Sanitize()
	if _, err := conn.Exec(ctx, "CREATE DATABASE "+quotedName); err != nil {
		fatal("Cannot create database %s: %v", dbName, err)
	}
	fmt.Printf("Created database %q\n", dbName)
}

// ensureSchema connects to the target database, creates the router schema if
// missing, and applies the canonical schema to a fresh (empty) one. An
// already-populated schema is left untouched — the product ships fresh, so
// there is no upgrade path by design.
func ensureSchema(ctx context.Context, dsn string) {
	conn, err := pgx.Connect(ctx, dsn)
	if err != nil {
		fatal("Cannot connect to target database: %v", err)
	}
	defer conn.Close(ctx)

	var tables int
	if err := conn.QueryRow(ctx,
		"SELECT count(*) FROM pg_tables WHERE schemaname = 'router'").Scan(&tables); err != nil {
		fatal("Cannot inspect router schema: %v", err)
	}
	if tables > 0 {
		fmt.Printf("Schema 'router' ready (%d tables, already initialized)\n", tables)
		return
	}

	// The canonical schema is router.*-qualified and creates the schema
	// itself (IF NOT EXISTS), so it applies cleanly whether or not the
	// schema exists yet.
	if _, err := conn.Exec(ctx, canonicalSchema); err != nil {
		fatal("Cannot apply canonical schema: %v", err)
	}
	if err := conn.QueryRow(ctx,
		"SELECT count(*) FROM pg_tables WHERE schemaname = 'router'").Scan(&tables); err != nil {
		fatal("Cannot inspect router schema: %v", err)
	}
	fmt.Printf("Schema 'router' ready (%d tables, initialized)\n", tables)
}

func fatal(format string, args ...any) {
	fmt.Fprintf(os.Stderr, "Error: "+format+"\n", args...)
	os.Exit(1)
}
