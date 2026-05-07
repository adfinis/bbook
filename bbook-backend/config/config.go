package config

import "os"

type Config struct {
	Env string

	PostgresHost     string
	PostgresUser     string
	PostgresPassword string
	PostgresDB       string
}

func Load() *Config {
	return &Config{
		Env:              os.Getenv("ENV"),
		PostgresHost:     os.Getenv("POSTGRES_HOST"),
		PostgresUser:     os.Getenv("POSTGRES_USER"),
		PostgresPassword: os.Getenv("POSTGRES_PASSWORD"),
		PostgresDB:       os.Getenv("POSTGRES_DB"),
	}
}
