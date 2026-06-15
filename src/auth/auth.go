package auth

import (
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"

	"git.adfinis.com/albertc/bbook/bbook-backend/config"
	"github.com/coreos/go-oidc/v3/oidc"
	"golang.org/x/oauth2"
)

const (
	sessionCookieName = "bbook_session"
	flowCookieName    = "bbook_flow"
	sessionTTL        = 12 * time.Hour
	flowTTL           = 10 * time.Minute
)

var (
	verifier *oidc.IDTokenVerifier
	oauthCfg *oauth2.Config
	secret   []byte
)

type sessionClaims struct {
	Sub   string `json:"sub"`
	Email string `json:"email"`
	Name  string `json:"name"`
	Exp   int64  `json:"exp"`
}

type flowClaims struct {
	State    string `json:"state"`
	Nonce    string `json:"nonce"`
	ReturnTo string `json:"return_to"`
	Exp      int64  `json:"exp"`
}

// Init discovers the OIDC provider and prepares the oauth2 config + verifier.
func Init(ctx context.Context) error {
	if config.AppConfig.SessionSecret == "" {
		return errors.New("initializing auth: SESSION_SECRET is empty")
	}
	secret = []byte(config.AppConfig.SessionSecret)

	issuerCtx := oidc.InsecureIssuerURLContext(ctx, config.AppConfig.OIDCIssuerURL)
	provider, err := oidc.NewProvider(issuerCtx, config.AppConfig.OIDCDiscoveryURL)
	if err != nil {
		return fmt.Errorf("initializing auth: discover provider: %w", err)
	}
	verifier = provider.Verifier(&oidc.Config{ClientID: config.AppConfig.OIDCClientID})
	oauthCfg = &oauth2.Config{
		ClientID:     config.AppConfig.OIDCClientID,
		ClientSecret: config.AppConfig.OIDCClientSecret,
		Endpoint:     provider.Endpoint(),
		RedirectURL:  config.AppConfig.OIDCRedirectURL,
		Scopes:       []string{oidc.ScopeOpenID, "email", "profile"},
	}
	return nil
}

// Redirects unauthenticated requests to the login flow
func Middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if _, ok := currentSession(r); ok {
			next.ServeHTTP(w, r)
			return
		}
		loginURL := "/auth/login?return_to=" + url.QueryEscape(r.URL.RequestURI())
		// HTMX redirect
		if r.Header.Get("HX-Request") == "true" {
			w.Header().Set("HX-Redirect", loginURL)
			w.WriteHeader(http.StatusOK)
			return
		}
		http.Redirect(w, r, loginURL, http.StatusFound)
	})
}

// Starts the auth-code flow
func LoginHandler(w http.ResponseWriter, r *http.Request) {
	returnTo := r.URL.Query().Get("return_to")
	if returnTo == "" || !strings.HasPrefix(returnTo, "/") {
		returnTo = "/"
	}
	state, err := randString(24)
	if err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	nonce, err := randString(24)
	if err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	flow := flowClaims{
		State:    state,
		Nonce:    nonce,
		ReturnTo: returnTo,
		Exp:      time.Now().Add(flowTTL).Unix(),
	}
	if err := setSignedCookie(w, flowCookieName, flow, flowTTL); err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	http.Redirect(w, r, oauthCfg.AuthCodeURL(state, oidc.Nonce(nonce)), http.StatusFound)
}

// Completes the auth-code flow
func CallbackHandler(w http.ResponseWriter, r *http.Request) {
	var flow flowClaims
	if err := readSignedCookie(r, flowCookieName, &flow); err != nil {
		http.Error(w, "invalid login flow: "+err.Error(), http.StatusBadRequest)
		return
	}
	if time.Now().Unix() > flow.Exp {
		http.Error(w, "login flow expired", http.StatusBadRequest)
		return
	}
	if r.URL.Query().Get("state") != flow.State {
		http.Error(w, "state mismatch", http.StatusBadRequest)
		return
	}

	token, err := oauthCfg.Exchange(r.Context(), r.URL.Query().Get("code"))
	if err != nil {
		http.Error(w, "code exchange: "+err.Error(), http.StatusBadGateway)
		return
	}
	rawIDToken, ok := token.Extra("id_token").(string)
	if !ok {
		http.Error(w, "no id_token in response", http.StatusBadGateway)
		return
	}
	idToken, err := verifier.Verify(r.Context(), rawIDToken)
	if err != nil {
		http.Error(w, "verify id_token: "+err.Error(), http.StatusUnauthorized)
		return
	}
	if idToken.Nonce != flow.Nonce {
		http.Error(w, "nonce mismatch", http.StatusBadRequest)
		return
	}

	var claims struct {
		Sub   string `json:"sub"`
		Email string `json:"email"`
		Name  string `json:"name"`
	}
	if err := idToken.Claims(&claims); err != nil {
		http.Error(w, "decode claims: "+err.Error(), http.StatusInternalServerError)
		return
	}
	session := sessionClaims{
		Sub:   claims.Sub,
		Email: claims.Email,
		Name:  claims.Name,
		Exp:   time.Now().Add(sessionTTL).Unix(),
	}
	if err := setSignedCookie(w, sessionCookieName, session, sessionTTL); err != nil {
		http.Error(w, "set session: "+err.Error(), http.StatusInternalServerError)
		return
	}
	clearCookie(w, flowCookieName)
	http.Redirect(w, r, flow.ReturnTo, http.StatusFound)
}

// LogoutHandler clears the session cookie.
func LogoutHandler(w http.ResponseWriter, r *http.Request) {
	clearCookie(w, sessionCookieName)
	http.Redirect(w, r, "/", http.StatusFound)
}

func currentSession(r *http.Request) (sessionClaims, bool) {
	var s sessionClaims
	if err := readSignedCookie(r, sessionCookieName, &s); err != nil {
		return s, false
	}
	if time.Now().Unix() > s.Exp {
		return s, false
	}
	return s, true
}

func setSignedCookie(w http.ResponseWriter, name string, v any, ttl time.Duration) error {
	payload, err := json.Marshal(v)
	if err != nil {
		return fmt.Errorf("signing cookie %s: marshal: %w", name, err)
	}
	value := sign(payload)
	http.SetCookie(w, &http.Cookie{
		Name:     name,
		Value:    value,
		Path:     "/",
		HttpOnly: true,
		Secure:   true,
		SameSite: http.SameSiteLaxMode,
		MaxAge:   int(ttl.Seconds()),
	})
	return nil
}

func readSignedCookie(r *http.Request, name string, v any) error {
	c, err := r.Cookie(name)
	if err != nil {
		return fmt.Errorf("reading cookie %s: %w", name, err)
	}
	payload, err := verify(c.Value)
	if err != nil {
		return fmt.Errorf("reading cookie %s: %w", name, err)
	}
	if err := json.Unmarshal(payload, v); err != nil {
		return fmt.Errorf("reading cookie %s: unmarshal: %w", name, err)
	}
	return nil
}

func clearCookie(w http.ResponseWriter, name string) {
	http.SetCookie(w, &http.Cookie{
		Name:     name,
		Value:    "",
		Path:     "/",
		HttpOnly: true,
		Secure:   true,
		SameSite: http.SameSiteLaxMode,
		MaxAge:   -1,
	})
}

func sign(payload []byte) string {
	mac := hmac.New(sha256.New, secret)
	mac.Write(payload)
	return base64.RawURLEncoding.EncodeToString(payload) + "." + base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
}

func verify(s string) ([]byte, error) {
	parts := strings.SplitN(s, ".", 2)
	if len(parts) != 2 {
		return nil, errors.New("bad cookie format")
	}
	payload, err := base64.RawURLEncoding.DecodeString(parts[0])
	if err != nil {
		return nil, fmt.Errorf("decode payload: %w", err)
	}
	sig, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return nil, fmt.Errorf("decode signature: %w", err)
	}
	mac := hmac.New(sha256.New, secret)
	mac.Write(payload)
	if !hmac.Equal(mac.Sum(nil), sig) {
		return nil, errors.New("bad cookie signature")
	}
	return payload, nil
}

func randString(n int) (string, error) {
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("random string: %w", err)
	}
	return base64.RawURLEncoding.EncodeToString(b), nil
}
