package database

import (
	"database/sql"
	"embed"
	"fmt"
	"log"
	"net/url"
	"strings"

	"github.com/lib/pq"
	"github.com/pressly/goose/v3"
)

//go:embed migrations/*.sql
var embedMigrations embed.FS

func ensureDatabase(databaseURL string) error {
	u, err := url.Parse(databaseURL)
	if err != nil {
		return fmt.Errorf("parse database url: %w", err)
	}
	dbName := strings.TrimPrefix(u.Path, "/")
	if dbName == "" {
		return fmt.Errorf("database name is empty in url")
	}
	u.Path = "/postgres"
	adminURL := u.String()
	db, err := sql.Open("postgres", adminURL)
	if err != nil {
		return fmt.Errorf("open admin connection: %w", err)
	}
	defer db.Close()
	if err := db.Ping(); err != nil {
		return fmt.Errorf("ping admin connection: %w", err)
	}
	var exists bool
	if err := db.QueryRow("SELECT true FROM pg_database WHERE datname = $1", dbName).Scan(&exists); err != nil && err != sql.ErrNoRows {
		return fmt.Errorf("check database existence: %w", err)
	}
	if exists {
		return nil
	}
	if _, err = db.Exec("CREATE DATABASE " + pq.QuoteIdentifier(dbName)); err != nil {
		return fmt.Errorf("create database %q: %w", dbName, err)
	}
	log.Printf("database: created %q\n", dbName)
	return nil
}

func MigrateUp(databaseURL string) error {
	if err := ensureDatabase(databaseURL); err != nil {
		return fmt.Errorf("ensure database: %w", err)
	}
	db, err := sql.Open("postgres", databaseURL)
	if err != nil {
		return fmt.Errorf("open db: %w", err)
	}
	defer db.Close()

	goose.SetBaseFS(embedMigrations)
	if err := goose.SetDialect("postgres"); err != nil {
		return fmt.Errorf("set dialect: %w", err)
	}
	if err := goose.Up(db, "migrations"); err != nil {
		return fmt.Errorf("goose up: %w", err)
	}
	return nil
}
