package main

import (
	"context"
	"log"
	"os"

	"git.adfinis.com/albertc/bbook/bbook-backend/database"
	"git.adfinis.com/albertc/bbook/bbook-backend/server"
	"git.adfinis.com/albertc/bbook/bbook-backend/zoho"
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

	// Test
	// ctx := context.Background()
	// contacts, err := zoho.FetchContacts(ctx)
	// if err != nil {
	// 	log.Fatalf("zoho: %v", err)
	// }
	// syncedAt := time.Now()
	// for _, c := range contacts {
	// 	c.SyncedAt = syncedAt
	// 	if err := database.Client.Queries.UpsertContact(ctx, database.UpsertContactParams(c)); err != nil {
	// 		log.Fatalf("upsert: %v", err)
	// 	}
	// }
	// log.Printf("fetched %d contacts", len(contacts))
	// stored, err := database.Client.Queries.AllContacts(ctx)
	// if err != nil {
	// 	log.Fatalf("read back: %v", err)
	// }
	// log.Printf("stored %d contacts", len(stored))

	// ratelimit, err := zoho.FetchRateLimit(ctx)
	// if err != nil {
	// 	log.Fatalf("zoho: %v", err)
	// }
	// fmt.Println(ratelimit.Limit, ratelimit.Remaining, ratelimit.ResetAt)
	//
	//
	// contacts, err := zoho.FetchContacts(context.Background())
	// if err != nil {
	// 	log.Fatalf("zoho: %v", err)
	// }
	// log.Println("Success", len(contacts), "contacts fetched")
	// b, _ := json.MarshalIndent(contacts[0], "", "  ")
	// fmt.Println(string(b))

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
