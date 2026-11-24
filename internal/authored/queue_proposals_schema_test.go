package authored_test

import (
	"io/fs"
	"slices"
	"testing"
	"testing/fstest"

	"github.com/gabrielassisxyz/finwise/internal/authored/authoredtest"
)

// What the database held before the queue's proposals existed: every proposal
// of every outcome, in the table whose check the migration rebuilds.
var proposalsBeforeTheQueue = []string{
	`INSERT INTO proposals (submitted_at, proposer, method, arguments, reason, title, based_on) VALUES ('2026-11-28T12:00:00.000Z', 'assistente', 'transactions.addTag', '["a","#x"]', 'primeira', 'Marcar a', 0)`,
	`INSERT INTO proposals (submitted_at, proposer, method, arguments, reason, title, based_on, outcome, answered_at, refusal) VALUES ('2026-11-28T12:01:00.000Z', 'assistente', 'transactions.addTag', '["b","#x"]', 'segunda', 'Marcar b', 0, 'refused', '2026-11-28T13:00:00.000Z', 'não')`,
	`INSERT INTO proposals (submitted_at, proposer, method, arguments, reason, title, based_on, outcome, answered_at, stale_reason) VALUES ('2026-11-28T12:02:00.000Z', 'assistente', 'transactions.addTag', '["c","#x"]', 'terceira', 'Marcar c', 0, 'stale', '2026-11-28T13:00:00.000Z', 'proposal_stale')`,
}

const proposalsDump = `SELECT id || '|' || submitted_at || '|' || proposer || '|' || method || '|' || arguments || '|' || reason || '|' || title || '|' || based_on
  || '|' || outcome || '|' || coalesce(answered_at, '-') || '|' || coalesce(refusal, '-') || '|' || coalesce(stale_reason, '-') || '|' || coalesce(change_id, '-')
FROM proposals ORDER BY id`

// The database the earlier releases left holds the proposals table with its
// own check, so the migration that rebuilds it has to keep every row.
func TestADatabaseThatHeldProposalsKeepsEveryOneWhenItGainsTheQueueTable(t *testing.T) {
	path := databasePath(t)
	earlier := fstest.MapFS{}
	embedded := withMigrations(t, nil)
	for name, file := range embedded {
		if name != "0011_queue_proposals.sql" {
			earlier[name] = file
		}
	}
	if _, found := embedded["0011_queue_proposals.sql"]; !found || len(earlier) != len(embedded)-1 {
		t.Fatal("the embedded set does not carry 0011_queue_proposals, so the test would prove nothing")
	}
	database := openDatabase(t, path, creationInstant, earlier)
	for _, statement := range proposalsBeforeTheQueue {
		if _, err := database.Pool().ExecContext(t.Context(), statement); err != nil {
			t.Fatal(err)
		}
	}
	before := columnValues(t, database.Pool(), proposalsDump)
	if len(before) != len(proposalsBeforeTheQueue) || slices.Contains(tableNames(t, database.Pool()), "queue_proposals") {
		t.Fatalf("the earlier database holds %v with the queue's table %v", before, tableNames(t, database.Pool()))
	}
	if err := database.Close(); err != nil {
		t.Fatal(err)
	}

	reopened := openDatabase(t, path, laterInstant, nil)
	if after := columnValues(t, reopened.Pool(), proposalsDump); !equalStrings(after, before) {
		t.Errorf("the proposals after the migration are\n%v\nwant\n%v", after, before)
	}
	if !slices.Contains(tableNames(t, reopened.Pool()), "queue_proposals") {
		t.Errorf("the tables after the open are %v, want queue_proposals among them", tableNames(t, reopened.Pool()))
	}
	// An id is never reused: the next proposal is numbered past the ones copied.
	inserted, err := reopened.Pool().ExecContext(t.Context(), `INSERT INTO proposals (submitted_at, proposer, method, arguments, reason, title, based_on) VALUES ('2026-11-29T12:00:00.000Z', 'a', 'm', '[]', 'r', 't', 0)`)
	if err != nil {
		t.Fatal(err)
	}
	if id, _ := inserted.LastInsertId(); id != int64(len(proposalsBeforeTheQueue))+1 {
		t.Errorf("the next proposal got id %d, want %d", id, len(proposalsBeforeTheQueue)+1)
	}
	if copies := copiesBeside(t, path); len(copies) != 1 {
		t.Errorf("copies = %v, want the one taken before migrating a database that holds data", copies)
	}
}

func TestTheQueueMigrationIsInTheEmbeddedSet(t *testing.T) {
	names, err := fs.Glob(withMigrations(t, nil), "0011_*.sql")
	if err != nil || !equalStrings(names, []string{"0011_queue_proposals.sql"}) {
		t.Errorf("0011_* = %v (%v), want 0011_queue_proposals.sql", names, err)
	}
}

// Each refusal comes from the database itself, with no Go in the way, after the
// valid row they differ from.
func TestTheQueueProposalsTableRefusesWhatIsNotAValidRow(t *testing.T) {
	database := openDatabase(t, databasePath(t), creationInstant, nil)
	line, other := twoLines(t, database)
	key := indexed(t, database, seenRow("p-1", "2026-11-28", -100, "name:a"))
	first := authoredtest.InsertProposal(t, database, queueDraft("primeira", queueProposalInstant))
	second := authoredtest.InsertProposal(t, database, queueDraft("segunda", queueProposalInstant))
	insert := `INSERT INTO queue_proposals (proposal_id, tx_key, line_id) VALUES (?, ?, ?)`
	pool := database.Pool()

	if _, err := pool.ExecContext(t.Context(), insert, int64(first), int64(key), int64(line)); err != nil {
		t.Fatalf("a valid row was refused: %v", err)
	}
	refused := map[string][]any{
		"a proposal twice":                  {int64(first), int64(key), int64(other)},
		"a proposal that does not exist":    {int64(99), int64(key), int64(line)},
		"a transaction the index lacks":     {int64(second), int64(99), int64(line)},
		"a line that does not exist":        {int64(second), int64(key), int64(999)},
		"no transaction":                    {int64(second), nil, int64(line)},
		"no line":                           {int64(second), int64(key), nil},
		"a line that is not a whole number": {int64(second), int64(key), "Mercado"},
	}
	for label, args := range refused {
		if _, err := pool.ExecContext(t.Context(), insert, args...); err == nil {
			t.Errorf("the database accepted %s", label)
		}
	}
	if _, err := pool.ExecContext(t.Context(), insert, int64(second), int64(key), int64(other)); err != nil {
		t.Errorf("a second proposal for the same transaction was refused by the table, which only the store's rule keeps to one: %v", err)
	}
}
