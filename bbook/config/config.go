package config

import (
	"fmt"
	"log/slog"
	"net/url"
	"os"
	"strconv"
	"strings"
)

type Config struct {
	LogFormat string // "json" or "text"
	LogLevel  string // debug/info/warn/error

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
	ZohoSyncEnabled  bool

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
func Load() (*Config, error) {
	c := &Config{}
	c.LogFormat = os.Getenv("LOG_FORMAT")
	c.LogLevel = os.Getenv("LOG_LEVEL")
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
	c.ZohoSyncEnabled, _ = strconv.ParseBool(os.Getenv("ZOHO_SYNC_ENABLED"))

	c.OIDCIssuerURL = os.Getenv("OIDC_ISSUER_URL")
	c.OIDCDiscoveryURL = os.Getenv("OIDC_DISCOVERY_URL")
	c.OIDCInsecureSkipIssuerValidation, _ = strconv.ParseBool(os.Getenv("OIDC_INSECURE_SKIP_ISSUER_VALIDATION"))
	c.OIDCClientID = os.Getenv("OIDC_CLIENT_ID")
	c.OIDCClientSecret = os.Getenv("OIDC_CLIENT_SECRET")
	c.OIDCRedirectURL = os.Getenv("OIDC_REDIRECT_URL")
	c.OIDCRequiredGroup = os.Getenv("OIDC_REQUIRED_GROUP")
	c.SessionSecret = os.Getenv("SESSION_SECRET")

	return c, c.validate()
}

func (c *Config) validate() error {
	var errs []string
	require := func(name, val string) {
		if val == "" {
			errs = append(errs, name+" is required")
		}
	}

	if u, err := url.Parse(c.BaseURL); err != nil || u.Host == "" {
		errs = append(errs, "BBOOK_HOSTNAME is required")
	}

	require("POSTGRES_HOST", c.PostgresHost)
	require("POSTGRES_USER", c.PostgresUser)
	require("POSTGRES_PASSWORD", c.PostgresPassword)
	require("POSTGRES_DB", c.PostgresDB)

	require("OIDC_ISSUER_URL", c.OIDCIssuerURL)
	require("OIDC_CLIENT_ID", c.OIDCClientID)
	require("OIDC_CLIENT_SECRET", c.OIDCClientSecret)
	require("OIDC_REDIRECT_URL", c.OIDCRedirectURL)
	require("OIDC_DISCOVERY_URL", c.OIDCDiscoveryURL)
	if c.OIDCInsecureSkipIssuerValidation {
		slog.Warn("OIDC issuer validation disabled")
	}

	if len(c.SessionSecret) < 32 {
		errs = append(errs, fmt.Sprintf("SESSION_SECRET must be at least 32 bytes, got %d", len(c.SessionSecret)))
	}

	// Zoho settings are only needed when the sync runs.
	if c.ZohoSyncEnabled {
		require("ZOHO_CLIENT_ID", c.ZohoClientID)
		require("ZOHO_CLIENT_SECRET", c.ZohoClientSecret)
		require("ZOHO_REFRESH_TOKEN", c.ZohoRefreshToken)
		require("ZOHO_BASE_URL", c.ZohoBaseURL)
		require("ZOHO_ACCOUNTS_URL", c.ZohoAccountsURL)
	} else {
		slog.Warn("zoho sync is disabled")
	}

	if len(errs) > 0 {
		return fmt.Errorf("loading config: %s", strings.Join(errs, "; "))
	}
	return nil
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
