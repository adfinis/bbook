package storage

import (
	"database/sql"
	"fmt"

	"git.sos.ethz.ch/vsos/bbook.vsos.ethz.ch/bbook-backend/config"
	_ "github.com/lib/pq"
)

type Storage struct {
	DB *sql.DB
}

func Init(cfg *config.Config) (*Storage, error) {
	dsn := fmt.Sprintf(
		"postgres://%s:%s@%s/%s?sslmode=disable",
		cfg.PostgresUser, cfg.PostgresPassword, cfg.PostgresHost, cfg.PostgresDB,
	)
	db, err := sql.Open("postgres", dsn)
	if err != nil {
		return nil, fmt.Errorf("open postgres: %w", err)
	}
	if err := db.Ping(); err != nil {
		db.Close()
		return nil, fmt.Errorf("ping postgres: %w", err)
	}
	return &Storage{DB: db}, nil
}

func (s *Storage) Close() error {
	return s.DB.Close()
}
