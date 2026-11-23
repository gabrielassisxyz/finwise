// Command finwise serves the finwise API over a read-only ledger snapshot.
package main

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/gabrielassisxyz/finwise/internal/ingest"
	"github.com/gabrielassisxyz/finwise/internal/suggest"
)

const (
	exitFailure = 1
	exitUsage   = 2
)

const usage = "usage: finwise serve | hash-pin | backup | ingest | suggest"

// environment is everything the commands take from the process, so a test can
// run them without touching the real standard streams or variables.
type environment struct {
	stdin  io.Reader
	stdout io.Writer
	stderr io.Writer
	getenv func(string) string
	// now is the only clock the commands read; main gives it the host's.
	now func() time.Time
	// listening is told the bound address once the server accepts connections.
	listening func(address string)
	// runIngest takes the ledger in; main gives it ingest.Run.
	runIngest ingestFunc
	// runSuggest asks the configured model about pending purchases; defaults to suggest.Run.
	runSuggest suggestFunc
	// suggestTimeout overrides the per-request model timeout when positive.
	suggestTimeout time.Duration
}

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	env := environment{stdin: os.Stdin, stdout: os.Stdout, stderr: os.Stderr, getenv: os.Getenv, now: time.Now, runIngest: ingest.Run, runSuggest: suggest.Run}
	os.Exit(run(ctx, os.Args[1:], env))
}

func run(ctx context.Context, args []string, env environment) int {
	if len(args) == 0 {
		fmt.Fprintln(env.stderr, usage)
		return exitUsage
	}
	switch args[0] {
	case "serve":
		return runServe(ctx, args[1:], env)
	case "hash-pin":
		return runHashPIN(env)
	case "backup":
		return runBackup(ctx, args[1:], env)
	case "ingest":
		return runIngest(ctx, args[1:], env)
	case "suggest":
		return runSuggestCommand(ctx, args[1:], env)
	default:
		fmt.Fprintln(env.stderr, usage)
		return exitUsage
	}
}
