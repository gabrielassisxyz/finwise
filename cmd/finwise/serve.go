package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"path"
	"strings"
	"time"

	"github.com/gabrielassisxyz/finwise/internal/api"
	"github.com/gabrielassisxyz/finwise/internal/authored"
	"github.com/gabrielassisxyz/finwise/internal/ingest"
	"github.com/gabrielassisxyz/finwise/internal/ledger"
	"github.com/gabrielassisxyz/finwise/internal/webui"
)

const (
	defaultListen         = "127.0.0.1:8080"
	shutdownTimeout       = 10 * time.Second
	defaultIngestInterval = 10 * time.Minute
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
	ingestInterval time.Duration
	model          modelSettings
	// agentToken is nil when no FINWISE_AGENT_TOKEN_HASH is set, and then no
	// request is answered as the agent's.
	agentToken *api.AgentTokenHash
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
	interval := flags.String("ingest-interval", "", "time between two runs of the ingest with --db (FINWISE_INGEST_INTERVAL, default 10m)")
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
	var valid bool
	if settings.ingestInterval, valid = ingestInterval(firstNonEmpty(*interval, env.getenv("FINWISE_INGEST_INTERVAL"))); !valid {
		fmt.Fprintln(env.stderr, "finwise: the ingest interval (--ingest-interval or FINWISE_INGEST_INTERVAL) is not a positive duration such as 10m")
		return serveSettings{}, exitFailure
	}
	return completeServeSettings(settings, env)
}

// ingestInterval reads the time between two runs of the ingest's timer, ten
// minutes when nothing says otherwise.
func ingestInterval(raw string) (time.Duration, bool) {
	if raw == "" {
		return defaultIngestInterval, true
	}
	interval, err := time.ParseDuration(raw)
	return interval, err == nil && interval > 0
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
	model, err := readModelSettings(env.getenv)
	if err != nil {
		fmt.Fprintf(env.stderr, "finwise: %v\n", err)
		return serveSettings{}, exitFailure
	}
	settings.model = model
	return completeAgentToken(settings, env)
}

// completeAgentToken reads the optional hash of the agent's token. Its error
// names the variable and never the value, which is one step from the token.
func completeAgentToken(settings serveSettings, env environment) (serveSettings, int) {
	stored := env.getenv(agentTokenHashVariable)
	if stored == "" {
		return settings, 0
	}
	hash, err := api.ParseAgentTokenHash(stored)
	if err != nil {
		fmt.Fprintf(env.stderr, "finwise: %s: %v\n", agentTokenHashVariable, err)
		return serveSettings{}, exitFailure
	}
	settings.agentToken = &hash
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
	var database *authored.Database
	if settings.databasePath != "" {
		opened, openErr := authored.Open(ctx, settings.databasePath, authored.Options{Now: env.now})
		if openErr != nil {
			return openErr
		}
		database = opened
		defer func() {
			if closeErr := database.Close(); closeErr != nil && err == nil {
				err = fmt.Errorf("close %s: %w", settings.databasePath, closeErr)
			}
		}()
		logger.Info("app database open", "path", settings.databasePath)
		if settings.pinHash, err = storedPINHash(ctx, database, settings, logger); err != nil {
			return err
		}
	}
	var suggester *serverSuggester
	var ingests serverIngests
	config := apiConfig(settings, database, env, logger)
	if database != nil {
		if settings.model.configured {
			// Deferred after the database's close so in-flight model calls are
			// cancelled and joined before the database closes.
			suggester = newServerSuggester(ctx, settings, database, env, logger)
			defer suggester.stop()
		}
		ingests = serverIngestsOf(settings, database, env, logger)
		ingests.suggester = suggester
		config.AfterBudgetSaved, config.TakeSnapshotIn = ingests.afterBudgetSaved, ingests.beforeWrite
	}
	handler, err := api.NewHandler(config)
	if err != nil {
		return err
	}
	listener, err := net.Listen("tcp", settings.listen)
	if err != nil {
		return fmt.Errorf("listen: %w", err)
	}
	server := &http.Server{
		Handler:           withInterface(handler, webui.Handler(webui.Built())),
		ReadHeaderTimeout: 10 * time.Second,
		ReadTimeout:       30 * time.Second,
		WriteTimeout:      60 * time.Second,
		IdleTimeout:       2 * time.Minute,
	}
	logger.Info("listening", "address", listener.Addr().String())
	if settings.agentToken != nil {
		logger.Info("an agent token opens the reads")
	}
	if settings.insecureCookie {
		logger.Warn("session cookie is not Secure: development only")
	}
	if !settings.model.configured {
		logger.Info(noModelConfiguredMessage)
	}
	if database != nil {
		// Deferred after the suggester's and database's stops, so it runs first:
		// no ingest can trigger a suggestion or touch the database after this.
		stop := ingests.every(ctx, settings.ingestInterval)
		defer stop()
	}
	if env.listening != nil {
		env.listening(listener.Addr().String())
	}
	return serveUntilDone(ctx, server, listener)
}

// withInterface hands the API's own paths to its handler as they come, and every
// other path to the interface. The interface never sees a path the API owns, so
// a mistyped method is answered by the API's error and not by a page, and the
// API's session, token and headers are the same as without the interface.
func withInterface(apiHandler, interfaceHandler http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if ownedByAPI(r.URL.Path) {
			apiHandler.ServeHTTP(w, r)
			return
		}
		interfaceHandler.ServeHTTP(w, r)
	})
}

// ownedByAPI judges a path by where it lands once cleaned, as the API's own mux
// does, so that `/./api/x` and `//api/x` reach the API and its redirect instead
// of the interface's fallback.
func ownedByAPI(urlPath string) bool {
	cleaned := path.Clean("/" + urlPath)
	return cleaned == "/healthz" || cleaned == "/api" || strings.HasPrefix(cleaned, "/api/")
}

// storedPINHash is the hash that opens the app: the one the person set in the
// app when there is one, and the environment's until then. A stored hash that
// does not parse stops the start instead of falling back to the environment's,
// which would let a PIN the person retired open the app again.
func storedPINHash(ctx context.Context, database *authored.Database, settings serveSettings, logger *slog.Logger) (api.PINHash, error) {
	stored, found, err := database.StoredPINHash(ctx)
	if err != nil {
		return api.PINHash{}, fmt.Errorf("read the PIN stored in %s: %w", settings.databasePath, err)
	}
	if !found {
		return settings.pinHash, nil
	}
	hash, err := api.ParsePINHash(stored)
	if err != nil {
		return api.PINHash{}, fmt.Errorf("the PIN stored in %s is not a valid hash: %w", settings.databasePath, err)
	}
	logger.Info("the PIN stored in the database opens the app; FINWISE_PIN_HASH is ignored")
	return hash, nil
}

// apiConfig hands the API the one clock of the process and the app's database,
// nil when serve was given none. With a database, a budget save takes the
// ledger in before it answers, and a write that finds the snapshot not taken in
// takes it in before it goes on.
func apiConfig(settings serveSettings, database *authored.Database, env environment, logger *slog.Logger) api.Config {
	config := api.Config{
		Ledger:         ledger.New(settings.ledgerPath),
		Database:       database,
		VerifyPIN:      settings.pinHash.Verify,
		Now:            env.now,
		SessionIdle:    settings.sessionIdle,
		InsecureCookie: settings.insecureCookie,
		Logger:         logger,
		ModelName:      settings.model.name,
	}
	if database != nil {
		ingests := serverIngestsOf(settings, database, env, logger)
		config.AfterBudgetSaved, config.TakeSnapshotIn = ingests.afterBudgetSaved, ingests.beforeWrite
	}
	if settings.agentToken != nil {
		config.VerifyAgentToken = settings.agentToken.Verify
	}
	return config
}

// serverIngestsOf is the ingest of the ledger serve reads into its database,
// logged on its log.
func serverIngestsOf(settings serveSettings, database *authored.Database, env environment, logger *slog.Logger) serverIngests {
	sources := ingest.Sources{Ledger: ledger.New(settings.ledgerPath), Database: database}
	return serverIngests{in: ingester{sources: sources, run: env.runIngest}, logger: logger, now: env.now}
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
