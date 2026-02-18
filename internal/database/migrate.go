package database

import (
	"database/sql"
	"fmt"
	"log"
	"net/url"
	"os"
	"path/filepath"
	"strings"

	"github.com/golang-migrate/migrate/v4"
	_ "github.com/golang-migrate/migrate/v4/database/postgres"
	_ "github.com/golang-migrate/migrate/v4/source/file"
	"github.com/lib/pq"
)

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
	cwd, _ := os.Getwd()
	dirs := []string{
		filepath.Join(cwd, "database", "migrations"),
		filepath.Join(cwd, "..", "database", "migrations"),
	}
	var absDir string
	for _, d := range dirs {
		if _, err := os.Stat(d); err == nil {
			absDir, _ = filepath.Abs(d)
			break
		}
	}
	if absDir == "" {
		return fmt.Errorf("migrations dir not found")
	}
	sourceURL := "file://" + filepath.ToSlash(absDir)
	m, err := migrate.New(sourceURL, databaseURL)
	if err != nil {
		return fmt.Errorf("migrate new: %w", err)
	}
	defer m.Close()
	if err := m.Up(); err != nil && err != migrate.ErrNoChange {
		return err
	}
	if err == migrate.ErrNoChange {
		log.Println("migrate: no pending migrations")
	} else {
		log.Println("migrate: up ok")
	}
	return nil
}
