package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"log/slog"
	"net/url"
	"strings"
	"time"

	"github.com/gabrielassisxyz/finwise/internal/authored"
	"github.com/gabrielassisxyz/finwise/internal/ledger"
	"github.com/gabrielassisxyz/finwise/internal/suggest"
)

const (
	suggestUsage             = "usage: finwise suggest --db <file> --ledger <snapshot>"
	noModelConfiguredMessage = "no model configured: pending purchases get no suggestion"
)

// suggestFunc is suggest.Run, which main hands the commands; a test may inject
// a fake or instrumented runner.
type suggestFunc func(ctx context.Context, sources suggest.Sources, config suggest.Config, now func() time.Time) (suggest.Counts, error)

var errSuggestPanicked = errors.New("finwise: suggest panicked")

type modelSettings struct {
	configured bool
	url        string
	name       string
	key        string
}

// readModelSettings reads FINWISE_MODEL_URL, FINWISE_MODEL_NAME and the
// optional FINWISE_MODEL_KEY. Neither of the first two set disables the step;
// one without the other or a non-HTTPS URL to a non-loopback host is refused
// with a message naming only the variables, never their values.
func readModelSettings(getenv func(string) string) (modelSettings, error) {
	rawURL := getenv("FINWISE_MODEL_URL")
	name := getenv("FINWISE_MODEL_NAME")
	key := getenv("FINWISE_MODEL_KEY")
	if rawURL == "" && name == "" {
		return modelSettings{}, nil
	}
	if rawURL == "" || name == "" {
		return modelSettings{}, errors.New("FINWISE_MODEL_URL and FINWISE_MODEL_NAME must both be set, or neither")
	}
	if !allowedModelURL(rawURL) {
		return modelSettings{}, errors.New("FINWISE_MODEL_URL must be an https:// URL or an http:// URL to a loopback host")
	}
	return modelSettings{configured: true, url: rawURL, name: name, key: key}, nil
}

func allowedModelURL(raw string) bool {
	parsed, err := url.Parse(raw)
	if err != nil || parsed.Host == "" {
		return false
	}
	host := parsed.Hostname()
	if host == "" {
		return false
	}
	switch strings.ToLower(parsed.Scheme) {
	case "https":
		return true
	case "http":
		return isLoopbackHost(host)
	default:
		return false
	}
}

func effectiveSuggest(env environment) suggestFunc {
	if env.runSuggest != nil {
		return env.runSuggest
	}
	return suggest.Run
}

func suggestConfigOf(model modelSettings, env environment) suggest.Config {
	return suggest.Config{
		URL:     model.url,
		Model:   model.name,
		Key:     model.key,
		Timeout: env.suggestTimeout,
	}
}

// serverSuggester runs the model suggestion step in one background goroutine so
// no HTTP request or ingest is ever blocked by a model call. Triggers arriving
// while a step is running coalesce into a single follow-up run; stop cancels
// any call in flight and waits for the goroutine to finish before the database
// closes.
type serverSuggester struct {
	cancel context.CancelFunc
	notify chan struct{}
	done   chan struct{}

	sources suggest.Sources
	config  suggest.Config
	run     suggestFunc
	logger  *slog.Logger
	now     func() time.Time
}

func newServerSuggester(
	parent context.Context,
	settings serveSettings,
	database *authored.Database,
	env environment,
	logger *slog.Logger,
) *serverSuggester {
	ctx, cancel := context.WithCancel(parent)
	suggester := &serverSuggester{
		cancel:  cancel,
		notify:  make(chan struct{}, 1),
		done:    make(chan struct{}),
		sources: suggest.Sources{Ledger: ledger.New(settings.ledgerPath), Database: database},
		config:  suggestConfigOf(settings.model, env),
		run:     effectiveSuggest(env),
		logger:  logger,
		now:     env.now,
	}
	go func() {
		defer close(suggester.done)
		for {
			select {
			case <-ctx.Done():
				return
			case <-suggester.notify:
				if ctx.Err() != nil {
					return
				}
				suggester.once(ctx)
			}
		}
	}()
	return suggester
}

func (suggester *serverSuggester) trigger() {
	if suggester == nil {
		return
	}
	select {
	case suggester.notify <- struct{}{}:
	default:
	}
}

func (suggester *serverSuggester) stop() {
	if suggester == nil {
		return
	}
	suggester.cancel()
	<-suggester.done
}

func (suggester *serverSuggester) once(ctx context.Context) {
	counts, err := runSuggestSafe(ctx, suggester.run, suggester.sources, suggester.config, suggester.now)
	switch {
	case err != nil:
		if errors.Is(ctx.Err(), context.Canceled) {
			return
		}
		suggester.logger.Warn("suggest failed", "trigger", "ingest", "kind", suggestFailureKind(err))
	case counts.Requests > 0:
		logSuggestCounts(suggester.logger, "ingest", counts)
	}
}

func runSuggestSafe(
	ctx context.Context,
	run suggestFunc,
	sources suggest.Sources,
	config suggest.Config,
	now func() time.Time,
) (counts suggest.Counts, err error) {
	defer func() {
		if recover() != nil {
			counts, err = suggest.Counts{}, errSuggestPanicked
		}
	}()
	return run(ctx, sources, config, now)
}

func logSuggestCounts(logger *slog.Logger, trigger string, counts suggest.Counts) {
	logger.Info("suggest",
		"trigger", trigger,
		"requests", counts.Requests,
		"asked", counts.Asked,
		"suggested", counts.Suggested,
		"unusable", counts.Unusable,
	)
}

func suggestFailureKind(err error) string {
	if errors.Is(err, errSuggestPanicked) {
		return "panic"
	}
	if kind, classified := suggest.FailureOf(err); classified {
		return kind.String()
	}
	return "unknown"
}

// runSuggestCommand runs the model suggestion step once from the CLI. Unlike
// ingest, it never creates a missing database.
func runSuggestCommand(ctx context.Context, args []string, env environment) int {
	flags := flag.NewFlagSet("suggest", flag.ContinueOnError)
	flags.SetOutput(env.stderr)
	databaseFlag := flags.String("db", "", "path of the app's own database, which must exist (FINWISE_DB)")
	ledgerFlag := flags.String("ledger", "", "path of the ledger snapshot (FINWISE_LEDGER)")
	if err := flags.Parse(args); err != nil {
		return exitUsage
	}
	databasePath := firstNonEmpty(*databaseFlag, env.getenv("FINWISE_DB"))
	ledgerPath := firstNonEmpty(*ledgerFlag, env.getenv("FINWISE_LEDGER"))
	if databasePath == "" || ledgerPath == "" || flags.NArg() != 0 {
		fmt.Fprintln(env.stderr, suggestUsage)
		return exitUsage
	}
	model, err := readModelSettings(env.getenv)
	if err != nil {
		fmt.Fprintf(env.stderr, "finwise: %v\n", err)
		return exitFailure
	}
	if !model.configured {
		fmt.Fprintln(env.stderr, "finwise: no model configured: set FINWISE_MODEL_URL and FINWISE_MODEL_NAME")
		return exitFailure
	}
	database, err := authored.Open(ctx, databasePath, authored.Options{Now: env.now, MustExist: true})
	if err != nil {
		fmt.Fprintf(env.stderr, "finwise: %v\n", err)
		return exitFailure
	}
	sources := suggest.Sources{Ledger: ledger.New(ledgerPath), Database: database}
	counts, err := runSuggestSafe(ctx, effectiveSuggest(env), sources, suggestConfigOf(model, env), env.now)
	if closeErr := database.Close(); closeErr != nil && err == nil {
		fmt.Fprintf(env.stderr, "finwise: close %s: %v\n", databasePath, closeErr)
		return exitFailure
	}
	if err != nil {
		fmt.Fprintf(env.stderr, "finwise: suggest failed: %s\n", suggestFailureKind(err))
		return exitFailure
	}
	logSuggestCounts(slog.New(slog.NewTextHandler(env.stdout, nil)), "command", counts)
	return 0
}
