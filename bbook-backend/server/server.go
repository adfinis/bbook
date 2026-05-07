package server

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"os/signal"
	"syscall"
	"time"

	"git.sos.ethz.ch/vsos/bbook.vsos.ethz.ch/bbook-backend/config"
	"git.sos.ethz.ch/vsos/bbook.vsos.ethz.ch/bbook-backend/router"
	"git.sos.ethz.ch/vsos/bbook.vsos.ethz.ch/bbook-backend/storage"
)

const addr = ":8081"

func Start(ctx context.Context, cfg *config.Config) error {
	store, err := storage.Init(cfg)
	if err != nil {
		return fmt.Errorf("storage: %w", err)
	}
	defer store.Close()

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
