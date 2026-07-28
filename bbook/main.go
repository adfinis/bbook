package main

import (
	"context"
	"log/slog"
	"os"

	"git.adfinis.com/int-infrastructure/bbook/bbook/auth"
	"git.adfinis.com/int-infrastructure/bbook/bbook/config"
	"git.adfinis.com/int-infrastructure/bbook/bbook/database"
	"git.adfinis.com/int-infrastructure/bbook/bbook/logging"
	"git.adfinis.com/int-infrastructure/bbook/bbook/server"
	"git.adfinis.com/int-infrastructure/bbook/bbook/zoho"
	"github.com/urfave/cli/v3"
)

func main() {
	cfg, err := config.Load()
	if err != nil {
		slog.Error("loading config", "err", err)
		os.Exit(1)
	}
	logging.Setup(cfg.LogFormat, cfg.LogLevel)

	if err := database.Init(context.Background(), cfg); err != nil {
		slog.Error("initializing database", "err", err)
		os.Exit(1)
	}
	defer database.Client.DB.Close() //nolint:errcheck

	if err := zoho.Init(cfg); err != nil {
		slog.Error("initializing zoho", "err", err)
		os.Exit(1)
	}

	if err := auth.Init(context.Background(), cfg); err != nil {
		slog.Error("initializing auth", "err", err)
		os.Exit(1)
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
					return server.Start(ctx, cfg)
				},
			},
		},
	}

	if err := cmd.Run(context.Background(), os.Args); err != nil {
		slog.Error("running command", "err", err)
		os.Exit(1)
	}
}
