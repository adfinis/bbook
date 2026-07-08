package main

import (
	"context"
	"log"
	"os"

	"git.adfinis.com/int-infrastructure/bbook/bbook/auth"
	"git.adfinis.com/int-infrastructure/bbook/bbook/database"
	"git.adfinis.com/int-infrastructure/bbook/bbook/server"
	"git.adfinis.com/int-infrastructure/bbook/bbook/zoho"
	"github.com/urfave/cli/v3"
)

func main() {

	err := database.Init()
	if err != nil {
		log.Fatalf("database: %v", err)
	}
	defer database.Client.DB.Close()

	if err := zoho.Init(); err != nil {
		log.Fatalf("zoho: %v", err)
	}

	if err := auth.Init(context.Background()); err != nil {
		log.Fatalf("auth: %v", err)
	}

	cmd := &cli.Command{
		Name:                  "bbook",
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
