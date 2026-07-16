package config

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func setValidEnv(t *testing.T) {
	t.Helper()
	for k, v := range map[string]string{
		"LOG_FORMAT":                           "json",
		"LOG_LEVEL":                            "debug",
		"BBOOK_SCHEME":                         "https",
		"BBOOK_HOSTNAME":                       "bbook.example.com",
		"BBOOK_PORT":                           "",
		"POSTGRES_HOST":                        "db.example.com",
		"POSTGRES_USER":                        "bbook-user",
		"POSTGRES_PASSWORD":                    "hunter2",
		"POSTGRES_DB":                          "bbook",
		"CONNECTION_STRING_PARAMS":             "sslmode=disable",
		"ZOHO_CLIENT_ID":                       "zoho-client",
		"ZOHO_CLIENT_SECRET":                   "zoho-secret",
		"ZOHO_REFRESH_TOKEN":                   "zoho-refresh",
		"ZOHO_BASE_URL":                        "https://zoho.example.com",
		"ZOHO_ACCOUNTS_URL":                    "https://accounts.zoho.example.com",
		"ZOHO_SYNC_ENABLED":                    "true",
		"OIDC_ISSUER_URL":                      "https://idp.example.com",
		"OIDC_DISCOVERY_URL":                   "https://idp.example.com/.well-known/openid-configuration",
		"OIDC_INSECURE_SKIP_ISSUER_VALIDATION": "false",
		"OIDC_CLIENT_ID":                       "bbook-client",
		"OIDC_CLIENT_SECRET":                   "oidc-secret",
		"OIDC_REDIRECT_URL":                    "https://bbook.example.com/callback",
		"OIDC_REQUIRED_GROUP":                  "bbook-users",
		"SESSION_SECRET":                       "0123456789abcdef0123456789abcdef", // 32 bytes.
	} {
		t.Setenv(k, v)
	}
}

func TestLoadValidEnv(t *testing.T) {
	setValidEnv(t)
	c, err := Load()
	require.NoError(t, err)
	assert.Equal(t, &Config{
		LogFormat:                        "json",
		LogLevel:                         "debug",
		BaseURL:                          "https://bbook.example.com",
		PostgresHost:                     "db.example.com",
		PostgresUser:                     "bbook-user",
		PostgresPassword:                 "hunter2",
		PostgresDB:                       "bbook",
		ConnectionStringParams:           "sslmode=disable",
		ZohoClientID:                     "zoho-client",
		ZohoClientSecret:                 "zoho-secret",
		ZohoRefreshToken:                 "zoho-refresh",
		ZohoBaseURL:                      "https://zoho.example.com",
		ZohoAccountsURL:                  "https://accounts.zoho.example.com",
		ZohoSyncEnabled:                  true,
		OIDCIssuerURL:                    "https://idp.example.com",
		OIDCDiscoveryURL:                 "https://idp.example.com/.well-known/openid-configuration",
		OIDCInsecureSkipIssuerValidation: false,
		OIDCClientID:                     "bbook-client",
		OIDCClientSecret:                 "oidc-secret",
		OIDCRedirectURL:                  "https://bbook.example.com/callback",
		OIDCRequiredGroup:                "bbook-users",
		SessionSecret:                    "0123456789abcdef0123456789abcdef",
	}, c)
}

func TestLoadReportsEachMissingRequiredVar(t *testing.T) {
	for _, name := range []string{
		"BBOOK_HOSTNAME",
		"POSTGRES_HOST", "POSTGRES_USER", "POSTGRES_PASSWORD", "POSTGRES_DB",
		"OIDC_ISSUER_URL", "OIDC_CLIENT_ID", "OIDC_CLIENT_SECRET",
		"OIDC_REDIRECT_URL", "OIDC_DISCOVERY_URL",
		"ZOHO_SYNC_ENABLED", "OIDC_INSECURE_SKIP_ISSUER_VALIDATION",
	} {
		t.Run(name, func(t *testing.T) {
			setValidEnv(t)
			t.Setenv(name, "")
			_, err := Load()
			assert.ErrorIs(t, err, missingVarError(name))
		})
	}
}

func TestLoadJoinsMultipleMissingVars(t *testing.T) {
	setValidEnv(t)
	t.Setenv("POSTGRES_HOST", "")
	t.Setenv("OIDC_CLIENT_ID", "")
	_, err := Load()
	assert.ErrorIs(t, err, missingVarError("POSTGRES_HOST"))
	assert.ErrorIs(t, err, missingVarError("OIDC_CLIENT_ID"))
}

func TestLoadRejectsShortSessionSecret(t *testing.T) {
	setValidEnv(t)
	t.Setenv("SESSION_SECRET", "too-short")
	_, err := Load()
	assert.ErrorIs(t, err, errSessionSecretTooShort)
}

func TestLoadAcceptsExactly32ByteSessionSecret(t *testing.T) {
	setValidEnv(t)
	secret := "abcdefghijklmnopqrstuvwxyz012345"
	require.Len(t, secret, 32)
	t.Setenv("SESSION_SECRET", secret)
	_, err := Load()
	assert.NoError(t, err)
}

func TestLoadRequiresZohoVarsWhenSyncEnabled(t *testing.T) {
	for _, name := range []string{
		"ZOHO_CLIENT_ID", "ZOHO_CLIENT_SECRET", "ZOHO_REFRESH_TOKEN",
		"ZOHO_BASE_URL", "ZOHO_ACCOUNTS_URL",
	} {
		t.Run(name, func(t *testing.T) {
			setValidEnv(t)
			t.Setenv("ZOHO_SYNC_ENABLED", "true")
			t.Setenv(name, "")
			_, err := Load()
			assert.ErrorIs(t, err, missingVarError(name))
		})
	}
}

func TestLoadAllowsEmptyZohoVarsWhenSyncDisabled(t *testing.T) {
	setValidEnv(t)
	t.Setenv("ZOHO_SYNC_ENABLED", "false")
	for _, name := range []string{
		"ZOHO_CLIENT_ID", "ZOHO_CLIENT_SECRET", "ZOHO_REFRESH_TOKEN",
		"ZOHO_BASE_URL", "ZOHO_ACCOUNTS_URL",
	} {
		t.Setenv(name, "")
	}
	c, err := Load()
	require.NoError(t, err)
	assert.False(t, c.ZohoSyncEnabled)
}

func TestBaseURLPortElision(t *testing.T) {
	tests := []struct {
		name   string
		scheme string
		port   string
		want   string
	}{
		{"https default port elided", "https", "443", "https://bbook.example.com"},
		{"http default port elided", "http", "80", "http://bbook.example.com"},
		{"https custom port kept", "https", "8443", "https://bbook.example.com:8443"},
		{"http with 443 kept", "http", "443", "http://bbook.example.com:443"},
		{"empty port omitted", "https", "", "https://bbook.example.com"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			setValidEnv(t)
			t.Setenv("BBOOK_SCHEME", tt.scheme)
			t.Setenv("BBOOK_PORT", tt.port)
			c, err := Load()
			require.NoError(t, err)
			assert.Equal(t, tt.want, c.BaseURL)
		})
	}
}

func TestLoadRejectsInvalidBools(t *testing.T) {
	setValidEnv(t)
	t.Setenv("ZOHO_SYNC_ENABLED", "yes")
	t.Setenv("OIDC_INSECURE_SKIP_ISSUER_VALIDATION", "banana")
	_, err := Load()
	assert.ErrorIs(t, err, invalidBoolError("ZOHO_SYNC_ENABLED"))
	assert.ErrorIs(t, err, invalidBoolError("OIDC_INSECURE_SKIP_ISSUER_VALIDATION"))
}
