// Command lobby runs the project's lobby service (docs/servers/SERVICE.md).
//
// Usage: lobby --config <file>
package main

import (
	"errors"
	"flag"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"time"

	"servers-lobby/internal/accounts"
	"servers-lobby/internal/api"
	"servers-lobby/internal/config"
	"servers-lobby/internal/store"
)

// readHeaderTimeout limits how long a client may take to send the request
// headers. The documents do not set it; it stops idle connections from
// holding a slot open.
const readHeaderTimeout = 10 * time.Second

func main() {
	if err := run(os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, "lobby:", err)
		os.Exit(1)
	}
}

// run parses the command line, reads the configuration, opens the database
// and serves HTTP until the server stops.
func run(args []string) error {
	flags := flag.NewFlagSet("lobby", flag.ContinueOnError)
	configPath := flags.String("config", "", "path to the configuration file")
	if err := flags.Parse(args); err != nil {
		return err
	}
	if *configPath == "" {
		return errors.New("--config <file> is required")
	}
	cfg, err := config.Load(*configPath)
	if err != nil {
		return err
	}
	db, err := store.Open(cfg.Database)
	if err != nil {
		return err
	}
	defer db.Close()

	logger := slog.New(slog.NewTextHandler(os.Stderr, nil))
	accountService := accounts.New(db, cfg.AccountsPerAddressPerHour)
	server := &http.Server{
		Addr:              cfg.Listen,
		Handler:           api.NewHandler(cfg.TrustForwardedFor, cfg.PublicWSURL, accountService, logger),
		ReadHeaderTimeout: readHeaderTimeout,
	}
	logger.Info("listening", "address", cfg.Listen)
	return server.ListenAndServe()
}
