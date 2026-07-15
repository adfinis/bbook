package server

import (
	"context"
	"log/slog"
	"net/http"
	"os/signal"
	"syscall"
	"time"

	"git.adfinis.com/int-infrastructure/bbook/bbook/auth"
	"git.adfinis.com/int-infrastructure/bbook/bbook/config"
	"git.adfinis.com/int-infrastructure/bbook/bbook/database"
	"git.adfinis.com/int-infrastructure/bbook/bbook/router"
	"git.adfinis.com/int-infrastructure/bbook/bbook/server/search"
	"git.adfinis.com/int-infrastructure/bbook/bbook/zoho"
)

const addr = ":8081"

func Start(ctx context.Context, cfg *config.Config) error {
	// Build the search index from the DB on startup
	if contacts, err := database.Client.Queries.AllContacts(ctx); err != nil {
		slog.Error("initial search index: loading contacts", "err", err)
	} else if err := search.Rebuild(contacts); err != nil {
		slog.Error("initial search index: rebuilding", "err", err)
	}

	srv := &http.Server{
		Handler:           router.Router(cfg),
		Addr:              addr,
		ReadHeaderTimeout: 10 * time.Second,
		ReadTimeout:       30 * time.Second,
		WriteTimeout:      30 * time.Second,
		IdleTimeout:       120 * time.Second,
	}

	ctx, stop := signal.NotifyContext(ctx, syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	serverErr := make(chan error, 1)
	go func() {
		slog.Info("listening", "addr", addr)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			serverErr <- err
		}
		close(serverErr)
	}()

	// Periodic user access/mailclient token cleanup
	go auth.StartTokenCleanup(ctx)
	go auth.StartExpiredTokenPurge(ctx)

	// Zoho sync job
	go func() {
		if !cfg.ZohoSyncEnabled {
			return
		}
		ticker := time.NewTicker(1 * time.Hour)
		defer ticker.Stop()
		for {
			slog.Info("zoho sync: start")
			if err := zoho.RunZohoSync(ctx); err != nil {
				slog.Error("zoho sync", "err", err)
			}
			slog.Info("zoho sync: done")
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
			}
		}
	}()

	select {
	case err := <-serverErr:
		return err
	case <-ctx.Done():
		slog.Info("shutting down")
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		return srv.Shutdown(shutdownCtx)
	}
}
