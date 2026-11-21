package ledger

import _ "embed"

// SchemaV1 is the collector's schema at user_version 1, with no data. It is
// embedded so that fixtures and the query-preparation test build databases from
// the real definitions; production code never reads it.
//
//go:embed testdata/ledger-schema-v1.sql
var SchemaV1 string
