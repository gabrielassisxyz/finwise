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
)

const (
	exitFailure = 1
	exitUsage   = 2
)

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
}

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	env := environment{stdin: os.Stdin, stdout: os.Stdout, stderr: os.Stderr, getenv: os.Getenv, now: time.Now}
	os.Exit(run(ctx, os.Args[1:], env))
}

func run(ctx context.Context, args []string, env environment) int {
	if len(args) == 0 {
		fmt.Fprintln(env.stderr, "usage: finwise serve | hash-pin")
		return exitUsage
	}
	switch args[0] {
	case "serve":
		return runServe(ctx, args[1:], env)
	case "hash-pin":
		return runHashPIN(env)
	default:
		fmt.Fprintln(env.stderr, "usage: finwise serve | hash-pin")
		return exitUsage
	}
}
