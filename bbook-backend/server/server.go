package server

import (
	"context"
	"log"
	"net/http"
	"os/signal"
	"syscall"
	"time"

	"git.adfinis.com/albertc/bbook/bbook-backend/database"
	"git.adfinis.com/albertc/bbook/bbook-backend/router"
	"git.adfinis.com/albertc/bbook/bbook-backend/zoho"
)

const addr = ":8081"

func Start(ctx context.Context) error {

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

	// Zoho sync job
	go func() {
		ticker := time.NewTicker(5 * time.Minute)
		defer ticker.Stop()
	    for {
			log.Printf("zoho sync: start")
	        if err := runZohoSync(ctx); err != nil {
	            log.Printf("zoho sync job: %v", err)
	        }
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




func runZohoSync(ctx context.Context) error {
	runStart := time.Now()
	contacts, err := zoho.FetchContacts(ctx)
	if err != nil {
		return err
	}

	tx, err := database.Client.DB.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()  // no-op after a successful Commit

	q := database.Client.Queries.WithTx(tx)
	for _, c := range contacts {
		c.SyncedAt = time.Now()
		if err := q.UpsertContact(ctx, database.UpsertContactParams(c)); err != nil {
			return err
		}
	}

	// Delete contacts that weren't synced by this run (i.e were deleted since the last sync)
	deleted, err := q.DeleteContactsSyncedBefore(ctx, runStart)
	if err != nil {
		return err
	}
	log.Printf("zoho sync: upserted %d, deleted %d stale", len(contacts), deleted)
	return tx.Commit()
}
