package ledgertest

import "database/sql"

// document is one current-state row about to be written.
type document struct {
	kind, id, parent string
	body             map[string]any
	state            string
	ambiguous        bool
	day              string
	// table, columns and values are the extracted columns of the projection row,
	// written between id and raw.
	table   string
	columns []string
	values  []any
}

func (f *Fixture) ensureScope(kind, parent string) int64 {
	f.t.Helper()
	var id int64
	err := f.db.QueryRow("SELECT id FROM scopes WHERE run_id = 1 AND kind = ? AND parent = ?", kind, parent).Scan(&id)
	if err == nil {
		return id
	}
	if err != sql.ErrNoRows {
		f.t.Fatalf("ledgertest: scope lookup: %v", err)
	}
	minDate := noLowerBound
	if start, ok := f.coverageStart[parent]; ok {
		minDate = start
	}
	return f.lastID(f.exec(`INSERT INTO scopes (run_id, kind, parent, expected, outcome, infer_absence,
	        status, rows, min_date, closed_at, closed_day)
	        VALUES (1, ?, ?, 1, 'complete', 1, 200, 0, ?, '2026-03-15T03:00:00.000Z', '2026-03-15')`,
		kind, parent, minDate))
}

// observe writes a version and one observation of it, and returns the version's hash.
func (f *Fixture) observe(kind, id, parent, day string, body map[string]any) string {
	f.t.Helper()
	encoded := mustJSON(f.t, body)
	sha := shaOf(encoded)
	f.exec("INSERT OR IGNORE INTO versions (sha256, body) VALUES (?, ?)", sha, encoded)
	f.exec("INSERT OR IGNORE INTO documents (kind, provider_id) VALUES (?, ?)", kind, id)

	var documentID int64
	if err := f.db.QueryRow("SELECT id FROM documents WHERE kind = ? AND provider_id = ?", kind, id).Scan(&documentID); err != nil {
		f.t.Fatalf("ledgertest: document lookup: %v", err)
	}
	f.exec(`INSERT INTO observations (run_id, scope_id, document_id, parent, sha256, observed_at, observed_day)
	        VALUES (?, ?, ?, ?, ?, ?, ?)`,
		f.currentRun(), f.ensureScope(kind, parent), documentID, parent, sha, day+"T03:00:00.000Z", day)
	return sha
}

func (f *Fixture) put(doc document) {
	f.t.Helper()
	if doc.day == "" {
		doc.day = DefaultDay
	}
	if doc.state == "" {
		doc.state = "present"
	}
	f.bodies[doc.kind+"/"+doc.id] = doc.body
	sha := f.observe(doc.kind, doc.id, doc.parent, doc.day, doc.body)

	var documentID int64
	if err := f.db.QueryRow("SELECT id FROM documents WHERE kind = ? AND provider_id = ?", doc.kind, doc.id).Scan(&documentID); err != nil {
		f.t.Fatalf("ledgertest: document lookup: %v", err)
	}
	var ambiguousRun any
	if doc.ambiguous {
		ambiguousRun = 1
		f.exec("INSERT INTO ambiguities (run_id, document_id, candidate_parents, reason) VALUES (1, ?, '[]', 'fixture')", documentID)
	}
	f.exec(`INSERT INTO document_state (document_id, state, first_seen_run, last_seen_run, state_run,
	        ambiguous_run, parent, sha256, observed_at, observed_day)
	        VALUES (?, ?, 1, 1, 1, ?, ?, ?, ?, ?)`,
		documentID, doc.state, ambiguousRun, doc.parent, sha, doc.day+"T03:00:00.000Z", doc.day)

	columns := append([]string{"id"}, doc.columns...)
	columns = append(columns, "raw", "fetched_at", "first_seen_run", "last_seen_run", "state", "ambiguous_run")
	values := append([]any{doc.id}, doc.values...)
	values = append(values, mustJSON(f.t, doc.body), doc.day+"T03:00:00.000Z", 1, 1, doc.state, ambiguousRun)

	query := "INSERT INTO " + doc.table + " (" + join(columns) + ") VALUES (" + placeholders(len(columns)) + ")"
	f.exec(query, values...)
}

func join(columns []string) string {
	joined := ""
	for i, column := range columns {
		if i > 0 {
			joined += ", "
		}
		joined += column
	}
	return joined
}

func placeholders(count int) string {
	marks := ""
	for i := 0; i < count; i++ {
		if i > 0 {
			marks += ", "
		}
		marks += "?"
	}
	return marks
}
