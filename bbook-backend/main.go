package main

import (
	"context"
	"log"
	"os"

	"git.sos.ethz.ch/vsos/bbook.vsos.ethz.ch/bbook-backend/config"
	"git.sos.ethz.ch/vsos/bbook.vsos.ethz.ch/bbook-backend/server"
	"github.com/urfave/cli/v3"
)

func main() {
	cfg := config.Load()

	cmd := &cli.Command{
		Name:                  "bbook-backend",
		Usage:                 "BBook backend",
		EnableShellCompletion: true,
		Commands: []*cli.Command{
			{
				Name:        "server",
				Description: "Start the HTTP server",
				Action: func(ctx context.Context, _ *cli.Command) error {
					return server.Start(ctx, cfg)
				},
			},
		},
	}

	if err := cmd.Run(context.Background(), os.Args); err != nil {
		log.Fatal(err)
	}
}
