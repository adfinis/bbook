package main

import (
	"context"
	"log"
	"os"

	"git.sos.ethz.ch/vsos/bbook.vsos.ethz.ch/bbook-backend/server"
	"git.sos.ethz.ch/vsos/bbook.vsos.ethz.ch/bbook-backend/storage"
	"github.com/urfave/cli/v3"
)

func main() {

	store, err := storage.Init()
	if err != nil {
		log.Fatalf("storage: %w", err)
	}
	defer store.Close()



	cmd := &cli.Command{
		Name:                  "bbook-backend",
		Usage:                 "BBook backend",
		EnableShellCompletion: true,
		Commands: []*cli.Command{
			{
				Name:        "server",
				Description: "Start the HTTP server",
				Action: func(ctx context.Context, _ *cli.Command) error {
					return server.Start(ctx)
				},
			},
		},
	}

	if err := cmd.Run(context.Background(), os.Args); err != nil {
		log.Fatal(err)
	}
}
