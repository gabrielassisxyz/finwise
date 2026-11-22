package authored_test

import (
	"context"
	"slices"
	"testing"

	"github.com/gabrielassisxyz/finwise/internal/authored"
)

// The line every database holds: the built-in fees line.
const feesLine authored.LineID = 1

func TestATagIsKeptInOneSpellingWhateverTheWayItWasTyped(t *testing.T) {
	for typed, want := range map[string]string{
		" viagem de trabalho ": "#viagem-de-trabalho",
		"#viagem-de-trabalho":  "#viagem-de-trabalho",
		"#":                    "",
		"   ":                  "",
		"":                     "",
		"##a":                  "#a",
		"# a b":                "#a-b",
		"a  b":                 "#a-b",
		"a\t\n b":              "#a-b",
		"#  #a":                "##a",
		"#Água fria":           "#Água-fria",
	} {
		if got := authored.NormalizeTag(typed); got != want {
			t.Errorf("NormalizeTag(%q) = %q, want %q", typed, got, want)
		}
	}
}

// ownTags is what the transaction at key holds, and fixedTags what the
// establishment does, in the order they are read.
func ownTags(t *testing.T, database *authored.Database, key authored.TxKey) []authored.Tag {
	t.Helper()
	view := database.View()
	defer view.Close()
	found, err := view.Transactions(t.Context(), []authored.TxKey{key})
	if err != nil {
		t.Fatal(err)
	}
	return found[key].Tags
}

func establishmentOf(t *testing.T, database *authored.Database, key string) authored.Establishment {
	t.Helper()
	view := database.View()
	defer view.Close()
	found, err := view.Establishments(t.Context(), []string{key})
	if err != nil {
		t.Fatal(err)
	}
	return found[key]
}

func tagsAt(names ...string) []authored.Tag {
	tags := make([]authored.Tag, len(names))
	for place, name := range names {
		tags[place] = authored.Tag{Name: name, Position: int64(place)}
	}
	return tags
}

func TestANewTagTakesTheNextPlaceAndNoTagIsEverRenumbered(t *testing.T) {
	database := openDatabase(t, databasePath(t), creationInstant, nil)
	key := indexed(t, database, seenRow("p-1", "2026-11-28", -100, "name:a"))
	add := func(tag string) authored.ChangeID {
		return changeBy(t, database, "transactions.addTag", func(recorder *authored.Recorder) error {
			return recorder.AddTag(context.Background(), key, tag)
		})
	}
	for _, tag := range []string{"a", "b", "c"} {
		add(tag)
	}
	removal := changeBy(t, database, "transactions.removeTag", func(recorder *authored.Recorder) error {
		return recorder.RemoveTag(context.Background(), key, "#b")
	})
	if got, want := ownTags(t, database, key), []authored.Tag{{Name: "#a", Position: 0}, {Name: "#c", Position: 2}}; !slices.Equal(got, want) {
		t.Errorf("after removing #b: %v, want %v", got, want)
	}
	add("d")
	if got, want := ownTags(t, database, key)[2], (authored.Tag{Name: "#d", Position: 3}); got != want {
		t.Errorf("a tag added after a removal is %v, want the place after every tag held, %v", got, want)
	}
	if err := database.Undo(t.Context(), removal, undoInstant); err != nil {
		t.Fatal(err)
	}
	want := []authored.Tag{{Name: "#a", Position: 0}, {Name: "#b", Position: 1}, {Name: "#c", Position: 2}, {Name: "#d", Position: 3}}
	if got := ownTags(t, database, key); !slices.Equal(got, want) {
		t.Errorf("after the undo of the removal: %v, want every tag where it was, %v", got, want)
	}
}

func TestAFixedTagFollowsTheSamePlacesAsAnOwnOne(t *testing.T) {
	database := openDatabase(t, databasePath(t), creationInstant, nil)
	for _, tag := range []string{"x", "y", " z "} {
		changeBy(t, database, "transactions.addFixedTag", func(recorder *authored.Recorder) error {
			return recorder.AddFixedTag(context.Background(), "name:a", tag)
		})
	}
	changeBy(t, database, "transactions.removeFixedTag", func(recorder *authored.Recorder) error {
		return recorder.RemoveFixedTag(context.Background(), "name:a", "#y")
	})
	changeBy(t, database, "transactions.addFixedTag", func(recorder *authored.Recorder) error {
		return recorder.AddFixedTag(context.Background(), "name:a", "w")
	})
	want := []authored.Tag{{Name: "#x", Position: 0}, {Name: "#z", Position: 2}, {Name: "#w", Position: 3}}
	if got := establishmentOf(t, database, "name:a").FixedTags; !slices.Equal(got, want) {
		t.Errorf("fixed tags %v, want %v", got, want)
	}
	if other := establishmentOf(t, database, "name:b"); len(other.FixedTags) != 0 {
		t.Errorf("another establishment holds %v", other.FixedTags)
	}
}

func TestAWriteThatLeavesEverythingAsItWasRecordsNothingAndWritesNoRow(t *testing.T) {
	database := openDatabase(t, databasePath(t), creationInstant, nil)
	key := indexed(t, database, seenRow("p-1", "2026-11-28", -100, "name:a"))
	change(t, database, func(recorder *authored.Recorder) error {
		ctx := context.Background()
		for _, write := range []func() error{
			func() error { return recorder.AddTag(ctx, key, "#a") },
			func() error { return recorder.AddFixedTag(ctx, "name:a", "#a") },
			func() error { return recorder.SetNote(ctx, key, "nota") },
			func() error { return recorder.SetNickname(ctx, "name:a", "Padaria") },
			func() error { return recorder.SetDefaultLine(ctx, "name:a", feesLine) },
		} {
			if err := write(); err != nil {
				return err
			}
		}
		return nil
	})
	before := canonical(t, database)
	asked := map[string]func(*authored.Recorder, context.Context) error{
		"a tag already held":           func(r *authored.Recorder, ctx context.Context) error { return r.AddTag(ctx, key, " #a ") },
		"a tag that spells as nothing": func(r *authored.Recorder, ctx context.Context) error { return r.AddTag(ctx, key, "#") },
		"a fixed tag already held":     func(r *authored.Recorder, ctx context.Context) error { return r.AddFixedTag(ctx, "name:a", "a") },
		"a fixed tag of nothing":       func(r *authored.Recorder, ctx context.Context) error { return r.AddFixedTag(ctx, "name:a", "  ") },
		"a tag it does not have":       func(r *authored.Recorder, ctx context.Context) error { return r.RemoveTag(ctx, key, "#zzz") },
		"a fixed tag it does not have": func(r *authored.Recorder, ctx context.Context) error { return r.RemoveFixedTag(ctx, "name:a", "#zzz") },
		"the same note":                func(r *authored.Recorder, ctx context.Context) error { return r.SetNote(ctx, key, "nota") },
		"the same nickname": func(r *authored.Recorder, ctx context.Context) error {
			return r.SetNickname(ctx, "name:a", "  Padaria ")
		},
		"the same default line": func(r *authored.Recorder, ctx context.Context) error {
			return r.SetDefaultLine(ctx, "name:a", feesLine)
		},
	}
	for label, write := range asked {
		id := changeBy(t, database, "transactions.write", func(recorder *authored.Recorder) error {
			return write(recorder, context.Background())
		})
		if effects := len(effectsOf(t, database, id)); effects != 0 {
			t.Errorf("%s recorded %d effects", label, effects)
		}
	}
	if after := canonical(t, database); after != before {
		t.Errorf("writes that change nothing changed the dump:\n%s\nwas\n%s", after, before)
	}
}

func TestANoteKeepsItsSpacesAndAnEmptyOneLeavesNoRow(t *testing.T) {
	database := openDatabase(t, databasePath(t), creationInstant, nil)
	key := indexed(t, database, seenRow("p-1", "2026-11-28", -100, "name:a"))
	empty := canonical(t, database)
	set := func(note string) authored.ChangeID {
		return changeBy(t, database, "transactions.setNote", func(recorder *authored.Recorder) error {
			return recorder.SetNote(context.Background(), key, note)
		})
	}
	set("  abastecer na sexta \n")
	if got := transactionStateOf(t, database, key).Note; got != "  abastecer na sexta \n" {
		t.Errorf("the note is %q, want it as typed", got)
	}
	set("")
	if got := canonical(t, database); got != empty {
		t.Errorf("an emptied note left a row:\n%s\nwant\n%s", got, empty)
	}
}

func transactionStateOf(t *testing.T, database *authored.Database, key authored.TxKey) authored.TransactionState {
	t.Helper()
	view := database.View()
	defer view.Close()
	found, err := view.Transactions(t.Context(), []authored.TxKey{key})
	if err != nil {
		t.Fatal(err)
	}
	return found[key].State
}

func TestANicknameIsTrimmedStoredByKeyAndABlankOneLeavesNoRow(t *testing.T) {
	database := openDatabase(t, databasePath(t), creationInstant, nil)
	empty := canonical(t, database)
	set := func(key, nickname string) {
		changeBy(t, database, "transactions.setNickname", func(recorder *authored.Recorder) error {
			return recorder.SetNickname(context.Background(), key, nickname)
		})
	}
	set("doc:11122233344", "  Tia Maria  ")
	set("name:a", "Padaria")
	if got := establishmentOf(t, database, "doc:11122233344").Nickname; got != "Tia Maria" {
		t.Errorf("the nickname is %q, want it trimmed", got)
	}
	set("doc:11122233344", " \t ")
	set("name:a", "")
	if got := canonical(t, database); got != empty {
		t.Errorf("blank nicknames left rows behind:\n%s\nwant\n%s", got, empty)
	}
}

func TestANicknameAndADefaultLineShareOneRowAndEachKeepsTheOther(t *testing.T) {
	database := openDatabase(t, databasePath(t), creationInstant, nil)
	changeBy(t, database, "transactions.setDefaultLine", func(recorder *authored.Recorder) error {
		return recorder.SetDefaultLine(context.Background(), "name:a", feesLine)
	})
	nicknaming := changeBy(t, database, "transactions.setNickname", func(recorder *authored.Recorder) error {
		return recorder.SetNickname(context.Background(), "name:a", "Padaria")
	})
	got := establishmentOf(t, database, "name:a")
	if got.Nickname != "Padaria" || got.DefaultLine == nil || *got.DefaultLine != feesLine {
		t.Fatalf("the establishment holds %+v, want both facts", got)
	}
	if err := database.Undo(t.Context(), nicknaming, undoInstant); err != nil {
		t.Fatal(err)
	}
	got = establishmentOf(t, database, "name:a")
	if got.Nickname != "" || got.DefaultLine == nil || *got.DefaultLine != feesLine {
		t.Errorf("after undoing the nickname the establishment holds %+v, want the default line alone", got)
	}
}

func TestAStoreWriteIsRefusedOnceItsChangeHasEnded(t *testing.T) {
	database := openDatabase(t, databasePath(t), creationInstant, nil)
	key := indexed(t, database, seenRow("p-1", "2026-11-28", -100, "name:a"))
	var kept *authored.Recorder
	change(t, database, func(recorder *authored.Recorder) error {
		kept = recorder
		return nil
	})
	ctx := context.Background()
	for label, err := range map[string]error{
		"AddTag": kept.AddTag(ctx, key, "a"), "AddFixedTag": kept.AddFixedTag(ctx, "name:a", "a"),
		"SetNote": kept.SetNote(ctx, key, "n"), "SetNickname": kept.SetNickname(ctx, "name:a", "n"),
		"SetDefaultLine": kept.SetDefaultLine(ctx, "name:a", feesLine),
	} {
		if err == nil {
			t.Errorf("%s wrote after its change ended", label)
		}
	}
}
