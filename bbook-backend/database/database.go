package database

import (
	"database/sql"
	"fmt"

	"git.sos.ethz.ch/vsos/bbook.vsos.ethz.ch/bbook-backend/config"
	"git.sos.ethz.ch/vsos/bbook.vsos.ethz.ch/bbook-backend/database/migrations"
	"github.com/golang-migrate/migrate/v4"
	"github.com/golang-migrate/migrate/v4/database/postgres"
	"github.com/golang-migrate/migrate/v4/source/iofs"
	_ "github.com/lib/pq"
)

type database struct {
	DB      *sql.DB
	Queries *Queries
}

var Client database = database{}

func Init() (error) {
	dsn := fmt.Sprintf(
		"postgres://%s:%s@%s/%s?sslmode=disable",
		config.AppConfig.PostgresUser, config.AppConfig.PostgresPassword,
		config.AppConfig.PostgresHost, config.AppConfig.PostgresDB,
	)
	db, err := sql.Open("postgres", dsn)
	if err != nil {
		return fmt.Errorf("connecting to postgres: open: %w", err)
	}
	if err := db.Ping(); err != nil {
		db.Close()
		return fmt.Errorf("connecting to postgres: ping: %w", err)
	}

	if err := runMigrations(db); err != nil {
		db.Close()
		return err
	}

	Client.DB = db
	Client.Queries = New(db)
	return nil
}

func runMigrations(db *sql.DB) error {
	src, err := iofs.New(migrations.FS, ".")
	if err != nil {
		return fmt.Errorf("running migrations: source: %w", err)
	}
	driver, err := postgres.WithInstance(db, &postgres.Config{})
	if err != nil {
		return fmt.Errorf("running migrations: driver: %w", err)
	}
	m, err := migrate.NewWithInstance("iofs", src, "postgres", driver)
	if err != nil {
		return fmt.Errorf("running migrations: migrator: %w", err)
	}
	if err := m.Up(); err != nil && err != migrate.ErrNoChange {
		return fmt.Errorf("running migrations: up: %w", err)
	}
	return nil
}
