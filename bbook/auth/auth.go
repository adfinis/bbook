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
	"log"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"git.adfinis.com/int-infrastructure/bbook/bbook/config"
	"git.adfinis.com/int-infrastructure/bbook/bbook/database"
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
	Sub string `json:"sub"`
	Exp int64  `json:"exp"`
}

type flowClaims struct {
	State    string `json:"state"`
	Nonce    string `json:"nonce"`
	ReturnTo string `json:"return_to"`
	Exp      int64  `json:"exp"`
}

// Init discovers the OIDC provider and prepares the oauth2 config + verifier.
func Init(ctx context.Context, cfg *config.Config) error {
	if cfg.SessionSecret == "" {
		return errors.New("initializing auth: SESSION_SECRET is empty")
	}
	secret = []byte(cfg.SessionSecret)

	if err := initCrypto(); err != nil {
		return fmt.Errorf("initializing auth: %w", err)
	}

	var provider *oidc.Provider
	var err error
	if cfg.OIDCInsecureSkipIssuerValidation {
		// allow iss to be different from the issuer URL
		issuerCtx := oidc.InsecureIssuerURLContext(ctx, cfg.OIDCIssuerURL)
		provider, err = oidc.NewProvider(issuerCtx, cfg.OIDCDiscoveryURL)
	} else {
		provider, err = oidc.NewProvider(ctx, cfg.OIDCIssuerURL)
	}
	if err != nil {
		return fmt.Errorf("initializing auth: discover provider: %w", err)
	}
	verifier = provider.Verifier(&oidc.Config{ClientID: cfg.OIDCClientID})
	oauthCfg = &oauth2.Config{
		ClientID:     cfg.OIDCClientID,
		ClientSecret: cfg.OIDCClientSecret,
		Endpoint:     provider.Endpoint(),
		RedirectURL:  cfg.OIDCRedirectURL,
		Scopes:       []string{oidc.ScopeOpenID, "email", "profile", "offline_access"},
	}
	return nil
}

// Redirects unauthenticated requests to the login flow.
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

// Starts the auth-code flow.
func LoginHandler(w http.ResponseWriter, r *http.Request) {
	returnTo := r.URL.Query().Get("return_to")
	if returnTo == "" || !strings.HasPrefix(returnTo, "/") {
		returnTo = "/"
	}
	state, err := randString(24)
	if err != nil {
		log.Printf("login: generate state: %v", err)
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	nonce, err := randString(24)
	if err != nil {
		log.Printf("login: generate nonce: %v", err)
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
		log.Printf("login: set flow cookie: %v", err)
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	http.Redirect(w, r, oauthCfg.AuthCodeURL(state, oidc.Nonce(nonce)), http.StatusFound)
}

// Completes the auth-code flow.
func CallbackHandler(w http.ResponseWriter, r *http.Request) {
	var flow flowClaims
	if err := readSignedCookie(r, flowCookieName, &flow); err != nil {
		log.Printf("login callback: read flow cookie: %v", err)
		http.Error(w, "login failed", http.StatusBadRequest)
		return
	}
	if time.Now().Unix() > flow.Exp {
		http.Error(w, "login flow expired", http.StatusBadRequest)
		return
	}
	if r.URL.Query().Get("state") != flow.State {
		log.Printf("login callback: state mismatch")
		http.Error(w, "login failed", http.StatusBadRequest)
		return
	}

	token, err := oauthCfg.Exchange(r.Context(), r.URL.Query().Get("code"))
	if err != nil {
		log.Printf("login callback: code exchange: %v", err)
		http.Error(w, "login failed", http.StatusBadGateway)
		return
	}
	rawIDToken, ok := token.Extra("id_token").(string)
	if !ok {
		log.Printf("login callback: no id_token in token response")
		http.Error(w, "login failed", http.StatusBadGateway)
		return
	}
	idToken, err := verifier.Verify(r.Context(), rawIDToken)
	if err != nil {
		log.Printf("login callback: verify id_token: %v", err)
		http.Error(w, "login failed", http.StatusUnauthorized)
		return
	}
	if idToken.Nonce != flow.Nonce {
		log.Printf("login callback: nonce mismatch")
		http.Error(w, "login failed", http.StatusBadRequest)
		return
	}

	sub := idToken.Subject

	// Persist the offline refresh so the cleanup goroutine can later check if the user is still allowed.
	if token.RefreshToken != "" && sub != "" {
		if err := saveOfflineToken(r.Context(), sub, token.RefreshToken); err != nil {
			log.Printf("login callback: store offline token for %s: %v", strconv.Quote(sub), err)
			http.Error(w, "login failed", http.StatusInternalServerError)
			return
		}
	}

	// Refresh a user's carddav access tokens once he logs in
	if sub != "" {
		if _, err := database.Client.Queries.RenewTokensForUser(r.Context(), sub); err != nil {
			log.Printf("renew tokens for %s: %v", strconv.Quote(sub), err)
		}
	}

	session := sessionClaims{
		Sub: sub,
		Exp: time.Now().Add(sessionTTL).Unix(),
	}
	if err := setSignedCookie(w, sessionCookieName, session, sessionTTL); err != nil {
		log.Printf("login callback: set session: %v", err)
		http.Error(w, "login failed", http.StatusInternalServerError)
		return
	}
	clearCookie(w, flowCookieName)
	http.Redirect(w, r, flow.ReturnTo, http.StatusFound)
}

func saveOfflineToken(ctx context.Context, sub, refreshToken string) error {
	ct, err := encrypt([]byte(refreshToken))
	if err != nil {
		return err
	}
	return database.Client.Queries.UpsertOfflineToken(ctx, database.UpsertOfflineTokenParams{
		UserSub:      sub,
		OfflineToken: ct,
	})
}

// Clears the session cookie.
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

// CurrentUserSub returns the logged-in user identifier for the current request.
func CurrentUserSub(r *http.Request) string {
	s, ok := currentSession(r)
	if !ok {
		return ""
	}
	return s.Sub
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
