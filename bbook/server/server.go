package server

import (
	"context"
	"log"
	"net/http"
	"os/signal"
	"syscall"
	"time"

	"git.adfinis.com/int-infrastructure/bbook/bbook/auth"
	"git.adfinis.com/int-infrastructure/bbook/bbook/database"
	"git.adfinis.com/int-infrastructure/bbook/bbook/router"
	"git.adfinis.com/int-infrastructure/bbook/bbook/server/search"
)

const addr = ":8081"

func Start(ctx context.Context) error {

	// Build the search index from the DB on startup
	if contacts, err := database.Client.Queries.AllContacts(ctx); err != nil {
		log.Printf("initial search index: load contacts: %v", err)
	} else if err := search.Rebuild(contacts); err != nil {
		log.Printf("initial search index: rebuild: %v", err)
	}

	srv := &http.Server{
		Handler:      router.Router(),
		Addr:         addr,
		ReadTimeout:  15 * time.Second,
		WriteTimeout: 15 * time.Second,
	}

	ctx, stop := signal.NotifyContext(ctx, syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	serverErr := make(chan error, 1)
	go func() {
		log.Printf("Listening on %s ...", addr)
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
		ticker := time.NewTicker(5 * time.Minute)
		defer ticker.Stop()
		for {
			log.Printf("zoho sync: start")
			// if err := zoho.RunZohoSync(ctx); err != nil {
			// 	log.Printf("zoho sync job: %v", err)
			// }
			log.Printf("zoho sync: done")
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
		log.Println("Shutting down ...")
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		return srv.Shutdown(shutdownCtx)
	}
}
