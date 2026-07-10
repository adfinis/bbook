package auth

import (
	"context"
	"errors"
	"fmt"
	"log"
	"time"

	"git.adfinis.com/int-infrastructure/bbook/bbook/database"
	"golang.org/x/oauth2"
)

const cleanupInterval = 5 * time.Minute

// Runs a periodic sweep that revokes access tokens for users Keycloak no longer allows.
func StartTokenCleanup(ctx context.Context) {
	ticker := time.NewTicker(cleanupInterval)
	defer ticker.Stop()
	for {
		if err := cleanupOnce(ctx); err != nil {
			log.Printf("token cleanup: %v", err)
		}
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}

// Expires the Carddav access tokens (and removes the offline tokens) of users that are not allowed anymore through OIDC. It does so by trying to refresh the offline token of each user. If the refresh fails, the user is considered disallowed and his tokens are expired immediately. Logging back in renews them.
func cleanupOnce(ctx context.Context) error {
	rows, err := database.Client.Queries.ListOfflineTokens(ctx)
	if err != nil {
		return err
	}

	var disallowed []string
	for _, row := range rows {
		ok, err := refreshOfflineToken(ctx, row.UserSub, row.OfflineToken)
		if err != nil {
			log.Printf("token cleanup: check %s: %v", row.UserSub, err)
			continue
		}
		if !ok {
			disallowed = append(disallowed, row.UserSub)
		}
	}

	if len(disallowed) == 0 {
		return nil
	}
	if _, err := database.Client.Queries.DeleteOfflineTokensForUserSubs(ctx, disallowed); err != nil {
		return err
	}
	n, err := database.Client.Queries.ExpireTokensForUserSubs(ctx, disallowed)
	if err != nil {
		return err
	}
	log.Printf("token cleanup: expired %d tokens across %d users", n, len(disallowed))
	return nil
}

const expiredTokenPurgeInterval = time.Hour

// Runs a periodic sweep that deletes CardDAV tokens expired for over a year.
func StartExpiredTokenPurge(ctx context.Context) {
	ticker := time.NewTicker(expiredTokenPurgeInterval)
	defer ticker.Stop()
	for {
		if n, err := database.Client.Queries.DeleteLongExpiredTokens(ctx); err != nil {
			log.Printf("token purge: %v", err)
		} else if n > 0 {
			log.Printf("token purge: deleted %d long-expired tokens", n)
		}
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}

// Try to refresh the offline token of a user. Return true if the refreshed token is retrieved, otherwise false.
func refreshOfflineToken(ctx context.Context, sub string, ct []byte) (bool, error) {
	rt, err := decrypt(ct)
	if err != nil {
		return false, err
	}
	ts := oauthCfg.TokenSource(ctx, &oauth2.Token{RefreshToken: string(rt)})
	tok, err := ts.Token()
	if err != nil {
		var rErr *oauth2.RetrieveError
		if errors.As(err, &rErr) {
			return false, nil
		}
		return false, err
	}

	// A successful refresh is not enough, the user must also still be in the required group.
	if requiredGroup != "" {
		allowed, err := refreshedTokenInGroup(ctx, tok)
		if err != nil {
			return false, err
		}
		if !allowed {
			return false, nil
		}
	}

	if tok.RefreshToken != "" && tok.RefreshToken != string(rt) {
		if err := saveOfflineToken(ctx, sub, tok.RefreshToken); err != nil {
			return true, err
		}
	}
	return true, nil
}

// Verifies the refreshed ID token and reports whether it still carries the required group.
func refreshedTokenInGroup(ctx context.Context, tok *oauth2.Token) (bool, error) {
	raw, ok := tok.Extra("id_token").(string)
	if !ok {
		return false, errors.New("checking refreshed token group: no id_token in refresh response")
	}
	idToken, err := verifier.Verify(ctx, raw)
	if err != nil {
		return false, fmt.Errorf("checking refreshed token group: verify id_token: %w", err)
	}
	var claims struct {
		Groups []string `json:"groups"`
	}
	if err := idToken.Claims(&claims); err != nil {
		return false, fmt.Errorf("checking refreshed token group: parse claims: %w", err)
	}
	return inGroup(claims.Groups, requiredGroup), nil
}
