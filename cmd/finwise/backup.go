package main

import (
	"context"
	"flag"
	"fmt"

	"github.com/gabrielassisxyz/finwise/internal/authored"
)

const backupUsage = "usage: finwise backup --db <file> --to <file>"

// runBackup takes the source the way serve does, so the nightly job can run in
// the server's environment; the destination is a flag only, so a stray
// variable can never choose where a copy lands.
func runBackup(ctx context.Context, args []string, env environment) int {
	flags := flag.NewFlagSet("backup", flag.ContinueOnError)
	flags.SetOutput(env.stderr)
	databasePath := flags.String("db", "", "path of the app's own database, which must exist (FINWISE_DB)")
	destination := flags.String("to", "", "path of the copy, which must not exist")
	if err := flags.Parse(args); err != nil {
		return exitUsage
	}
	source := firstNonEmpty(*databasePath, env.getenv("FINWISE_DB"))
	if source == "" || *destination == "" || flags.NArg() != 0 {
		fmt.Fprintln(env.stderr, backupUsage)
		return exitUsage
	}
	if err := authored.Backup(ctx, source, *destination, authored.BackupOptions{Now: env.now}); err != nil {
		fmt.Fprintf(env.stderr, "finwise: %v\n", err)
		return exitFailure
	}
	fmt.Fprintf(env.stdout, "finwise: backup of %s written to %s\n", source, *destination)
	return 0
}
