package ledger

import "database/sql"

// OpenReadOnly exposes the open path every query uses, so a test can try to
// write through it.
func OpenReadOnly(path string) (*sql.DB, error) { return openReadOnly(path) }

// AllQueries exposes the registry of statements the package runs.
func AllQueries() map[string]string { return allQueries }

// CloneTransaction exposes the copy each caller of CleanTransactions gets.
func CloneTransaction(transaction Transaction) Transaction { return transaction.clone() }
