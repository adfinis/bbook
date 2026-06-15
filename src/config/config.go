package config

import (
	"os"
)

var AppConfig config = parse()

type config struct {
	Env string

	PostgresHost     string
	PostgresUser     string
	PostgresPassword string
	PostgresDB       string
	ConnectionStringParams string

	ZohoClientID     string
	ZohoClientSecret string
	ZohoRefreshToken string
	ZohoBaseURL      string
	ZohoAccountsURL  string
}

func parse() config {
	c := config{}
	c.Env = os.Getenv("ENV")
	c.PostgresHost = os.Getenv("POSTGRES_HOST")
	c.PostgresUser = os.Getenv("POSTGRES_USER")
	c.PostgresPassword = os.Getenv("POSTGRES_PASSWORD")
	c.PostgresDB = os.Getenv("POSTGRES_DB")
	c.ConnectionStringParams = os.Getenv("CONNECTION_STRING_PARAMS")

	c.ZohoClientID = os.Getenv("ZOHO_CLIENT_ID")
	c.ZohoClientSecret = os.Getenv("ZOHO_CLIENT_SECRET")
	c.ZohoRefreshToken = os.Getenv("ZOHO_REFRESH_TOKEN")
	c.ZohoBaseURL = os.Getenv("ZOHO_BASE_URL")
	c.ZohoAccountsURL = os.Getenv("ZOHO_ACCOUNTS_URL")

	return c
}
