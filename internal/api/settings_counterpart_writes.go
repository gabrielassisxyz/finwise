package api

import (
	"context"
	"encoding/json"
)

// The settings' two writes about an establishment or a person, which the
// settings name by key. They are the transaction panel's own writes reached by
// another door: each checks the key and then does what the panel's handler does
// through the same functions, so a nickname or a default line set in one place
// is the one the other shows. Neither indexes a transaction, because both are
// kept by the key alone; idAndText reads the key where the panel's handlers read
// a transaction's id, both being the first of two texts.

func registerSettingsCounterpartWrites(registry *Registry) {
	registry.RegisterWrite("settings.setNickname", setCounterpartNickname)
	registry.RegisterWrite("settings.setDefaultLine", setCounterpartDefaultLine)
}

// setCounterpartNickname names the establishment or the person behind the key;
// a blank nickname takes the name away. A key only movements carry is refused
// as the panel refuses a movement.
func setCounterpartNickname(ctx context.Context, args []json.RawMessage, writes Writes) (any, error) {
	key, nickname, err := idAndText(args)
	if err != nil {
		return nil, err
	}
	if _, err := counterpartNamedBy(ctx, writes, key); err != nil {
		return nil, err
	}
	if err := writes.Recorder.SetNickname(ctx, key, nickname); err != nil {
		return nil, err
	}
	return changeResultOf(ctx, writes.Recorder)
}

// setCounterpartDefaultLine is checked for the key before the line: a key that
// names nobody, or a person, is refused as not found whatever the line is, so
// the answer does not depend on the budget of a month the person was not asking
// about.
func setCounterpartDefaultLine(ctx context.Context, args []json.RawMessage, writes Writes) (any, error) {
	key, line, err := idAndText(args)
	if err != nil {
		return nil, err
	}
	known, err := counterpartNamedBy(ctx, writes, key)
	if err != nil {
		return nil, err
	}
	if known.person {
		return nil, refuse(reasonNotFound)
	}
	if err := storeDefaultLine(ctx, writes, key, line); err != nil {
		return nil, err
	}
	return changeResultOf(ctx, writes.Recorder)
}
