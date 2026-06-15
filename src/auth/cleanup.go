package auth

import (
	"context"
	"errors"
	"log"
	"time"

	"git.adfinis.com/albertc/bbook/bbook-backend/database"
	"golang.org/x/oauth2"
)

const cleanupInterval = 5 * time.Minute

// Runs a periodic sweep that revokes access tokens for users Keycloak no longer allows
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

// Removes offline tokens and Carddav access tokens for users that are not allowed anymore through OIDC. It does so by trying to refresh the offline token of each user. If the refresh fails, the user is considered disallowed and all his tokens are immediately wiped.
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
	n, err := database.Client.Queries.DeleteTokensForUserSubs(ctx, disallowed)
	if err != nil {
		return err
	}
	log.Printf("token cleanup: revoked %d tokens across %d users", n, len(disallowed))
	return nil
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
	if tok.RefreshToken != "" && tok.RefreshToken != string(rt) {
		if err := saveOfflineToken(ctx, sub, tok.RefreshToken); err != nil {
			return true, err
		}
	}
	return true, nil
}
