package storage

import (
	"database/sql"
	"fmt"

	"git.sos.ethz.ch/vsos/bbook.vsos.ethz.ch/bbook-backend/config"
	_ "github.com/lib/pq"
)

type storage struct {
	DB *sql.DB
}

var client storage = storage{}



func Init() (*storage, error) {
	dsn := fmt.Sprintf(
		"postgres://%s:%s@%s/%s?sslmode=disable",
		config.AppConfig.PostgresUser, config.AppConfig.PostgresPassword, config.AppConfig.PostgresHost, config.AppConfig.PostgresDB,
	)
	db, err := sql.Open("postgres", dsn)
	if err != nil {
		return nil, fmt.Errorf("postgres error: open: %w", err)
	}
	if err := db.Ping(); err != nil {
		db.Close()
		return nil, fmt.Errorf("postgres error: ping: %w", err)
	}
	client.DB = db
	return &client, nil
}

func (s *storage) Close() error {
	return s.DB.Close()
}
