package config

import (
	"os"
	"strconv"
)

type Config struct {
	Env string

	BaseURL string

	PostgresHost           string
	PostgresUser           string
	PostgresPassword       string
	PostgresDB             string
	ConnectionStringParams string

	ZohoClientID     string
	ZohoClientSecret string
	ZohoRefreshToken string
	ZohoBaseURL      string
	ZohoAccountsURL  string

	OIDCIssuerURL                    string
	OIDCDiscoveryURL                 string
	OIDCInsecureSkipIssuerValidation bool
	OIDCClientID                     string
	OIDCClientSecret                 string
	OIDCRedirectURL                  string
	OIDCRequiredGroup                string
	SessionSecret                    string
}

// reads the configuration from the environment.
func Load() *Config {
	c := &Config{}
	c.Env = os.Getenv("ENV")
	c.BaseURL = baseURL()
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

	c.OIDCIssuerURL = os.Getenv("OIDC_ISSUER_URL")
	c.OIDCDiscoveryURL = os.Getenv("OIDC_DISCOVERY_URL")
	c.OIDCInsecureSkipIssuerValidation, _ = strconv.ParseBool(os.Getenv("OIDC_INSECURE_SKIP_ISSUER_VALIDATION"))
	c.OIDCClientID = os.Getenv("OIDC_CLIENT_ID")
	c.OIDCClientSecret = os.Getenv("OIDC_CLIENT_SECRET")
	c.OIDCRedirectURL = os.Getenv("OIDC_REDIRECT_URL")
	c.OIDCRequiredGroup = os.Getenv("OIDC_REQUIRED_GROUP")
	c.SessionSecret = os.Getenv("SESSION_SECRET")

	return c
}

func baseURL() string {
	scheme := os.Getenv("BBOOK_SCHEME")
	host := os.Getenv("BBOOK_HOSTNAME")
	port := os.Getenv("BBOOK_PORT")

	url := scheme + "://" + host
	if port != "" && (scheme != "https" || port != "443") && (scheme != "http" || port != "80") {
		url += ":" + port
	}
	return url
}
