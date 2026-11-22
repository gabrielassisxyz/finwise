package api

import (
	"context"
	"encoding/json"
	"strings"
	"unicode"

	"github.com/gabrielassisxyz/finwise/internal/authored"
)

// The settings' writes. Each is one change of the settings alone, written
// through the store; none reads the ledger, so each succeeds while the snapshot
// is missing. A change of the threshold or of the minimum acts on the
// transactions classified after it and starts no pass over those already
// filed.

func registerSettingsWrites(registry *Registry) {
	registry.RegisterWrite("settings.setThreshold", setThreshold)
	registry.RegisterWrite("settings.setMinExamples", setMinExamples)
	registry.RegisterWrite("settings.setCardLabel", setCardLabel)
	registry.RegisterWrite("settings.setClosing", setClosing)
}

// wholeRange is the range a number the person sets is held to.
type wholeRange struct{ least, most int }

// The contract's THRESHOLD_RANGE, MIN_EXAMPLES_RANGE and CLOSING_DAY_RANGE. The
// store's table refuses a value outside them, so a value is held to its range
// before it is written, as the interface holds it.
var (
	thresholdRange   = wholeRange{least: 50, most: 99}
	minExamplesRange = wholeRange{least: 1, most: 50}
	closingDayRange  = wholeRange{least: 1, most: 28}
)

func (within wholeRange) hold(number int) int {
	return min(max(number, within.least), within.most)
}

// heldWholeNumber reads a number as the interface rounds it, a half upward,
// and holds it to the range. What JSON does not hold as a number, and a number
// beyond a billion, is not an argument the method takes.
func heldWholeNumber(raw json.RawMessage, within wholeRange) (int64, error) {
	number, err := ParseWholeNumber(raw)
	if err != nil {
		return 0, &badArgumentsError{}
	}
	return int64(within.hold(number)), nil
}

func onlyHeldWholeNumber(args []json.RawMessage, within wholeRange) (int64, error) {
	if len(args) != 1 {
		return 0, &badArgumentsError{}
	}
	return heldWholeNumber(args[0], within)
}

func setThreshold(ctx context.Context, args []json.RawMessage, writes Writes) (any, error) {
	percent, err := onlyHeldWholeNumber(args, thresholdRange)
	if err != nil {
		return nil, err
	}
	if err := writes.Recorder.SetConfidenceThreshold(ctx, percent); err != nil {
		return nil, err
	}
	return changeResultOf(ctx, writes.Recorder)
}

func setMinExamples(ctx context.Context, args []json.RawMessage, writes Writes) (any, error) {
	count, err := onlyHeldWholeNumber(args, minExamplesRange)
	if err != nil {
		return nil, err
	}
	if err := writes.Recorder.SetMinExamples(ctx, count); err != nil {
		return nil, err
	}
	return changeResultOf(ctx, writes.Recorder)
}

// setCardLabel keeps the label trimmed and an empty one as it is: the empty
// text is what the person chose, not a request for the default.
func setCardLabel(ctx context.Context, args []json.RawMessage, writes Writes) (any, error) {
	label, err := stringArgument(args)
	if err != nil {
		return nil, err
	}
	if err := writes.Recorder.SetCardLabel(ctx, trimLikeTheInterface(label)); err != nil {
		return nil, err
	}
	return changeResultOf(ctx, writes.Recorder)
}

// trimLikeTheInterface trims what JavaScript's String.prototype.trim trims, so
// a label is kept as the interface shows it: unlike Go's unicode.IsSpace, that
// includes the byte order mark and leaves the next-line character.
func trimLikeTheInterface(text string) string {
	return strings.TrimFunc(text, func(r rune) bool {
		switch r {
		case '\t', '\n', '\v', '\f', '\r', '\uFEFF', '\u2028', '\u2029':
			return true
		}
		return unicode.Is(unicode.Zs, r)
	})
}

// setClosing stores the mode and the day together whatever the mode: in auto
// the day is kept for when the person turns manual again.
func setClosing(ctx context.Context, args []json.RawMessage, writes Writes) (any, error) {
	if len(args) != 2 {
		return nil, &badArgumentsError{}
	}
	mode, err := closingModeOf(args[0])
	if err != nil {
		return nil, err
	}
	day, err := heldWholeNumber(args[1], closingDayRange)
	if err != nil {
		return nil, err
	}
	if err := writes.Recorder.SetCardClosing(ctx, mode, day); err != nil {
		return nil, err
	}
	return changeResultOf(ctx, writes.Recorder)
}

func closingModeOf(raw json.RawMessage) (authored.ClosingMode, error) {
	var text *string
	if err := decodeArgument(raw, &text); err != nil || text == nil {
		return "", &badArgumentsError{}
	}
	switch mode := authored.ClosingMode(*text); mode {
	case authored.ClosingAuto, authored.ClosingManual:
		return mode, nil
	}
	return "", &badArgumentsError{}
}
