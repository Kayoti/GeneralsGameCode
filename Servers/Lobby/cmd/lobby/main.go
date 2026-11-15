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
	"servers-lobby/internal/pass"
	"servers-lobby/internal/realtime"
	"servers-lobby/internal/store"
)

// The documents do not set these server timeouts. They stop a client that
// stalls while sending a request, or sits idle between requests, from holding
// a connection open. A WebSocket is taken over from the server when it is
// upgraded, so these do not apply to it; the WebSocket has its own idle time.
const (
	readHeaderTimeout = 10 * time.Second
	readTimeout       = 30 * time.Second
	writeTimeout      = 30 * time.Second
	idleTimeout       = 60 * time.Second
)

func main() {
	if err := run(os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, "lobby:", err)
		os.Exit(1)
	}
}

// run parses the command line, reads the configuration, opens the database
// and serves HTTP and WebSocket until the server stops.
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
	passes, err := pass.Load(cfg.Regions)
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
	hub := realtime.NewHub(realtime.DefaultSettings(cfg.Rooms), accountService, logger)
	options := api.Options{TrustForwardedFor: cfg.TrustForwardedFor, WSURI: cfg.PublicWSURL, MOTD: cfg.MOTD, TurnSecret: cfg.TurnSecret, Passes: passes}
	server := &http.Server{
		Addr:              cfg.Listen,
		Handler:           api.NewHandler(options, accountService, hub, logger),
		ReadHeaderTimeout: readHeaderTimeout,
		ReadTimeout:       readTimeout,
		WriteTimeout:      writeTimeout,
		IdleTimeout:       idleTimeout,
	}
	logger.Info("listening", "address", cfg.Listen)
	return server.ListenAndServe()
}
