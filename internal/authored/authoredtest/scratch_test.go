package authoredtest_test

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/gabrielassisxyz/finwise/internal/authored"
	"github.com/gabrielassisxyz/finwise/internal/authored/authoredtest"
)

func clockAt(year int, month time.Month) func() time.Time {
	return func() time.Time { return time.Date(year, month, 20, 15, 0, 0, 0, time.UTC) }
}

func record(t *testing.T, scratch *authoredtest.Scratch, write func(ctx context.Context, recorder *authored.Recorder) error) {
	t.Helper()
	facts := authored.ChangeFacts{Author: authored.AuthorPerson, Method: "scratch.write", At: clockAt(2026, time.March)()}
	err := scratch.Database.Change(t.Context(), facts, func(recorder *authored.Recorder) error {
		return write(t.Context(), recorder)
	})
	if err != nil {
		t.Fatal(err)
	}
}

func text(value string) authored.Value         { return authored.Text(value) }
func number(value int64) authored.Value        { return authored.Integer(value) }
func key(v ...authored.Value) []authored.Value { return v }

func TestTwoDatabasesBuiltTheSameWayAreTheSameInEveryTable(t *testing.T) {
	first := authoredtest.Open(t, clockAt(2026, time.March))
	second := authoredtest.Open(t, clockAt(2026, time.March))

	if first.Canonical(t) != second.Canonical(t) {
		t.Errorf("canonical dumps differ:\n%s\n%s", first.Canonical(t), second.Canonical(t))
	}
	if first.Everything(t) != second.Everything(t) {
		t.Errorf("the files differ:\n%s\n%s", first.Everything(t), second.Everything(t))
	}
	if !strings.Contains(first.Everything(t), `tracking_start_month`) || !strings.Contains(first.Everything(t), "== schema_migrations") {
		t.Errorf("the built database is not migrated or holds no tracking start:\n%s", first.Everything(t))
	}
}

func TestTheInstantOfCreationDecidesTheTrackingStartMonth(t *testing.T) {
	march := authoredtest.Open(t, clockAt(2026, time.March))
	august := authoredtest.Open(t, clockAt(2026, time.August))
	fresh := authoredtest.Fresh(t, clockAt(2026, time.August))

	want := map[string]*authored.Database{"2026-03": march.Database, "2026-08": august.Database, "2026-08 (fresh)": fresh}
	for expected, database := range want {
		got, err := database.TrackingStartMonth(t.Context())
		if err != nil || string(got)+"" != strings.TrimSuffix(expected, " (fresh)") {
			t.Errorf("tracking start %q (%v), want %s", got, err, expected)
		}
	}
	if march.Everything(t) == august.Everything(t) {
		t.Error("two databases created in different months hold the same")
	}
}

func TestTheCanonicalDumpIsOneJSONDocumentWithEveryTableInNameOrder(t *testing.T) {
	scratch := authoredtest.Open(t, clockAt(2026, time.March))
	record(t, scratch, func(ctx context.Context, recorder *authored.Recorder) error {
		if err := recorder.Put(ctx, authoredtest.Tags, authored.Row{"tx": text("b"), "tag": text("x")}); err != nil {
			return err
		}
		return recorder.Put(ctx, authoredtest.Tags, authored.Row{"tx": text("a"), "tag": text("y")})
	})

	dump := scratch.Canonical(t)

	var parsed map[string][]map[string]any
	if err := json.Unmarshal([]byte(dump), &parsed); err != nil {
		t.Fatalf("the dump is not JSON: %v\n%s", err, dump)
	}
	names := []string{
		"budget_version_lines", "budget_versions", "closed_months", "pot_lines", "pot_settings", "scenarios",
		authoredtest.Children, authoredtest.Facts, authoredtest.Parents, authoredtest.Tags, authoredtest.Things,
		"settings",
	}
	last := -1
	for _, name := range names {
		if _, listed := parsed[name]; !listed {
			t.Errorf("%s is not in the dump, empty tables included:\n%s", name, dump)
		}
		at := strings.Index(dump, `"`+name+`"`)
		if at < last {
			t.Errorf("%s is out of name order:\n%s", name, dump)
		}
		last = at
	}
	if len(parsed) != len(names) {
		t.Errorf("the dump lists %d tables, want %d", len(parsed), len(names))
	}
	if first, second := strings.Index(dump, `"tx":"a"`), strings.Index(dump, `"tx":"b"`); first < 0 || first > second {
		t.Errorf("rows are not in key order:\n%s", dump)
	}
}

func TestTheCanonicalDumpFollowsOneColumnAndOneRow(t *testing.T) {
	build := func() *authoredtest.Scratch {
		scratch := authoredtest.Open(t, clockAt(2026, time.March))
		record(t, scratch, func(ctx context.Context, recorder *authored.Recorder) error {
			return recorder.Set(ctx, authoredtest.Facts, key(text("a")), authored.Row{"amount": number(5)})
		})
		return scratch
	}
	base, same, column, row := build(), build(), build(), build()
	record(t, column, func(ctx context.Context, recorder *authored.Recorder) error {
		return recorder.Set(ctx, authoredtest.Facts, key(text("a")), authored.Row{"amount": number(6)})
	})
	record(t, row, func(ctx context.Context, recorder *authored.Recorder) error {
		return recorder.Set(ctx, authoredtest.Facts, key(text("b")), authored.Row{"amount": number(5)})
	})

	if base.Canonical(t) != same.Canonical(t) {
		t.Error("two equal states gave different bytes")
	}
	if base.Canonical(t) == column.Canonical(t) {
		t.Error("one column of difference gave the same bytes")
	}
	if base.Canonical(t) == row.Canonical(t) {
		t.Error("one row of difference gave the same bytes")
	}
}

// A table the dump forgets would let an undo that leaves it behind pass, so each
// table of the registry is changed in turn and the dump must notice.
func TestEveryTableOfTheRegistryShowsInTheDump(t *testing.T) {
	writes := map[string]func(ctx context.Context, recorder *authored.Recorder) error{
		authoredtest.Facts: func(ctx context.Context, recorder *authored.Recorder) error {
			return recorder.Set(ctx, authoredtest.Facts, key(text("a")), authored.Row{"amount": number(5)})
		},
		authoredtest.Things: func(ctx context.Context, recorder *authored.Recorder) error {
			return recorder.Create(ctx, authoredtest.Things, authored.Row{"id": number(1), "name": text("n"), "amount": number(2)})
		},
		authoredtest.Tags: func(ctx context.Context, recorder *authored.Recorder) error {
			return recorder.Put(ctx, authoredtest.Tags, authored.Row{"tx": text("a"), "tag": text("x")})
		},
		authoredtest.Parents: func(ctx context.Context, recorder *authored.Recorder) error {
			return recorder.Put(ctx, authoredtest.Parents, authored.Row{"id": number(1), "name": text("p")})
		},
		authoredtest.Children: func(ctx context.Context, recorder *authored.Recorder) error {
			if err := recorder.Put(ctx, authoredtest.Parents, authored.Row{"id": number(1), "name": text("p")}); err != nil {
				return err
			}
			return recorder.Put(ctx, authoredtest.Children, authored.Row{"id": number(1), "parent_id": number(1), "label": text("c")})
		},
	}
	for name, write := range writes {
		scratch := authoredtest.Open(t, clockAt(2026, time.March))
		empty := scratch.Canonical(t)
		record(t, scratch, write)
		after := scratch.Canonical(t)
		if after == empty {
			t.Errorf("a change to %s did not move the dump", name)
		}
		if !strings.Contains(after, `"`+name+`": [`+"\n") {
			t.Errorf("%s holds no row in the dump after a change to it:\n%s", name, after)
		}
	}
}

func TestEverythingSeesATableThatIsNotJournaled(t *testing.T) {
	scratch := authoredtest.Open(t, clockAt(2026, time.March))
	canonical, everything := scratch.Canonical(t), scratch.Everything(t)

	scratch.TouchMeta(t, "an_observation", "x")

	if scratch.Canonical(t) != canonical {
		t.Error("a table that is not journaled moved the canonical dump")
	}
	if scratch.Everything(t) == everything {
		t.Error("a write to the meta table went unseen by Everything")
	}
}

func TestEffectsOfAChangeComeInTheOrderOfTheirSeq(t *testing.T) {
	scratch := authoredtest.Open(t, clockAt(2026, time.March))
	record(t, scratch, func(ctx context.Context, recorder *authored.Recorder) error {
		if err := recorder.Set(ctx, authoredtest.Facts, key(text("a")), authored.Row{"amount": number(5)}); err != nil {
			return err
		}
		return recorder.Put(ctx, authoredtest.Tags, authored.Row{"tx": text("a"), "tag": text("x")})
	})

	effects := scratch.EffectsOf(t, 1)

	if len(effects) != 2 || effects[0].Seq != 1 || effects[0].Table != authoredtest.Facts || effects[0].Column != "amount" ||
		effects[0].Before != "0" || effects[0].After != "5" || effects[1].Table != authoredtest.Tags || effects[1].Column != "" {
		t.Errorf("effects %+v", effects)
	}
}
