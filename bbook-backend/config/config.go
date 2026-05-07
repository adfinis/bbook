package config

import "os"

var AppConfig config = parse()

type config struct {
	Env string

	PostgresHost     string
	PostgresUser     string
	PostgresPassword string
	PostgresDB       string
}

func parse() config {
	c := config{}
	c.Env = os.Getenv("ENV")
	c.PostgresHost = os.Getenv("POSTGRES_HOST")
	c.PostgresUser = os.Getenv("POSTGRES_USER")
	c.PostgresPassword = os.Getenv("POSTGRES_PASSWORD")
	c.PostgresDB = os.Getenv("POSTGRES_DB")

	return c
}
