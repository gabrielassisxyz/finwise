package api

import (
	"reflect"
	"testing"

	"github.com/gabrielassisxyz/finwise/internal/ledger/ledgertest"
)

func TestTheYieldHistoryInOneReadingIsTheHistoryOfTheThreeReadingsOverTheFixtureLedgers(t *testing.T) {
	ledgers := map[string]*ledgertest.Fixture{
		"full": newFullLedger(t), "empty": newEmptyLedger(t), "yield scenario": yieldScenarioLedger(t),
	}
	versions, removals, unresolved := 0, 0, 0
	for name, f := range ledgers {
		snapshot := viewOf(t, f.Path)
		old, err := snapshot.YieldHistoryInThreeReadings(t.Context())
		if err != nil {
			t.Fatalf("%s, three readings: %v", name, err)
		}
		got, err := snapshot.YieldHistory(t.Context())
		if err != nil {
			t.Fatalf("%s, one reading: %v", name, err)
		}
		wanted, gotten := reflect.ValueOf(old), reflect.ValueOf(got)
		for index := range wanted.NumField() {
			if !reflect.DeepEqual(wanted.Field(index).Interface(), gotten.Field(index).Interface()) {
				t.Errorf("%s: %s reads as\n%+v\nin one reading, want\n%+v", name, wanted.Type().Field(index).Name,
					gotten.Field(index).Interface(), wanted.Field(index).Interface())
			}
		}
		versions, removals, unresolved = versions+len(got.Transfers), removals+len(got.Removals), unresolved+len(got.Unresolved)
	}
	if versions == 0 || removals == 0 || unresolved == 0 {
		t.Errorf("the ledgers hold %d versions, %d removals and %d unresolved ids, want some of each", versions, removals, unresolved)
	}
}
