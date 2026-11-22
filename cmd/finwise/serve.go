package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"time"

	"github.com/gabrielassisxyz/finwise/internal/api"
	"github.com/gabrielassisxyz/finwise/internal/authored"
	"github.com/gabrielassisxyz/finwise/internal/ledger"
)

const (
	defaultListen   = "127.0.0.1:8080"
	shutdownTimeout = 10 * time.Second
)

// serveSettings is what `finwise serve` was told, from flags first, then the
// environment, then the default.
type serveSettings struct {
	listen         string
	ledgerPath     string
	databasePath   string
	insecureCookie bool
	pinHash        api.PINHash
	sessionIdle    time.Duration
}

func runServe(ctx context.Context, args []string, env environment) int {
	settings, code := readServeSettings(args, env)
	if code != 0 {
		return code
	}
	if err := serve(ctx, settings, env); err != nil {
		fmt.Fprintf(env.stderr, "finwise: %v\n", err)
		return exitFailure
	}
	return 0
}

func readServeSettings(args []string, env environment) (serveSettings, int) {
	flags := flag.NewFlagSet("serve", flag.ContinueOnError)
	flags.SetOutput(env.stderr)
	listen := flags.String("listen", "", "address to listen on (FINWISE_LISTEN, default "+defaultListen+")")
	ledgerPath := flags.String("ledger", "", "path of the ledger snapshot (FINWISE_LEDGER)")
	databasePath := flags.String("db", "", "path of the app's own database, created if missing (FINWISE_DB; without it the server is read-only)")
	insecure := flags.Bool("dev-insecure-cookie", false, "development only: session cookie without Secure and the __Host- prefix")
	if err := flags.Parse(args); err != nil {
		return serveSettings{}, exitUsage
	}
	settings := serveSettings{
		listen:         firstNonEmpty(*listen, env.getenv("FINWISE_LISTEN"), defaultListen),
		ledgerPath:     firstNonEmpty(*ledgerPath, env.getenv("FINWISE_LEDGER")),
		databasePath:   firstNonEmpty(*databasePath, env.getenv("FINWISE_DB")),
		insecureCookie: *insecure,
	}
	if settings.ledgerPath == "" {
		fmt.Fprintln(env.stderr, "finwise: the ledger path is missing: pass --ledger or set FINWISE_LEDGER")
		return serveSettings{}, exitUsage
	}
	return completeServeSettings(settings, env)
}

// completeServeSettings reads the two values that come only from the
// environment. Neither error names the value: both would be a secret or near one.
func completeServeSettings(settings serveSettings, env environment) (serveSettings, int) {
	stored := env.getenv("FINWISE_PIN_HASH")
	if stored == "" {
		fmt.Fprintln(env.stderr, "finwise: FINWISE_PIN_HASH is not set; create one with `finwise hash-pin`")
		return serveSettings{}, exitFailure
	}
	hash, err := api.ParsePINHash(stored)
	if err != nil {
		fmt.Fprintf(env.stderr, "finwise: FINWISE_PIN_HASH: %v\n", err)
		return serveSettings{}, exitFailure
	}
	settings.pinHash = hash
	if raw := env.getenv("FINWISE_SESSION_TTL"); raw != "" {
		idle, err := time.ParseDuration(raw)
		if err != nil || idle <= 0 {
			fmt.Fprintln(env.stderr, "finwise: FINWISE_SESSION_TTL is not a positive duration such as 12h")
			return serveSettings{}, exitFailure
		}
		settings.sessionIdle = idle
	}
	return settings, 0
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if value != "" {
			return value
		}
	}
	return ""
}

// serve starts even when the snapshot is missing: the PIN works and reads
// answer `ledger_unavailable` until the collector has produced one. The app's
// database, when one is given, is opened before the listener, so a database
// that is refused never leaves a server answering.
func serve(ctx context.Context, settings serveSettings, env environment) (err error) {
	logger := slog.New(slog.NewTextHandler(env.stdout, nil))
	if settings.databasePath != "" {
		database, openErr := authored.Open(ctx, settings.databasePath, authored.Options{Now: env.now})
		if openErr != nil {
			return openErr
		}
		defer func() {
			if closeErr := database.Close(); closeErr != nil && err == nil {
				err = fmt.Errorf("close %s: %w", settings.databasePath, closeErr)
			}
		}()
		logger.Info("app database open", "path", settings.databasePath)
	}
	handler, err := api.NewHandler(api.Config{
		Ledger:         ledger.New(settings.ledgerPath),
		VerifyPIN:      settings.pinHash.Verify,
		Now:            env.now,
		SessionIdle:    settings.sessionIdle,
		InsecureCookie: settings.insecureCookie,
		Logger:         logger,
	})
	if err != nil {
		return err
	}
	listener, err := net.Listen("tcp", settings.listen)
	if err != nil {
		return fmt.Errorf("listen: %w", err)
	}
	server := &http.Server{
		Handler:           handler,
		ReadHeaderTimeout: 10 * time.Second,
		ReadTimeout:       30 * time.Second,
		WriteTimeout:      60 * time.Second,
		IdleTimeout:       2 * time.Minute,
	}
	logger.Info("listening", "address", listener.Addr().String())
	if settings.insecureCookie {
		logger.Warn("session cookie is not Secure: development only")
	}
	if env.listening != nil {
		env.listening(listener.Addr().String())
	}
	return serveUntilDone(ctx, server, listener)
}

func serveUntilDone(ctx context.Context, server *http.Server, listener net.Listener) error {
	failed := make(chan error, 1)
	go func() { failed <- server.Serve(listener) }()
	select {
	case err := <-failed:
		return err
	case <-ctx.Done():
	}
	shutdownCtx, cancel := context.WithTimeout(context.Background(), shutdownTimeout)
	defer cancel()
	if err := server.Shutdown(shutdownCtx); err != nil {
		return fmt.Errorf("shutdown: %w", err)
	}
	if err := <-failed; !errors.Is(err, http.ErrServerClosed) {
		return err
	}
	return nil
}
