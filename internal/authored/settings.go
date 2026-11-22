package authored

import (
	"context"
	"fmt"
)

// What each setting answers until the person sets it. They are stated here, and
// only here, because the rules that classify new transactions run below the
// API and need them too.
const (
	DefaultConfidenceThreshold int64       = 85
	DefaultMinExamples         int64       = 5
	DefaultCardLabel                       = "Físico"
	DefaultClosingMode         ClosingMode = ClosingAuto
	DefaultManualClosingDay    int64       = 28
)

// ClosingMode is how the card's bill closes: by the day the bank gives, or by a
// day the person chose.
type ClosingMode string

const (
	ClosingAuto   ClosingMode = "auto"
	ClosingManual ClosingMode = "manual"
)

const settingsTable = "settings"

// The keys of the settings table; the table refuses any other.
const (
	keyConfidenceThreshold = "confidence_threshold"
	keyMinExamples         = "min_examples"
	keyCardLabel           = "card_label"
	keyClosingMode         = "closing_mode"
	keyManualClosingDay    = "manual_closing_day"
)

const (
	statementSettingSelect    = `SELECT key, value FROM settings WHERE key = ?`
	statementSettingSelectAll = `SELECT key, value FROM settings`
	statementSettingInsert    = `INSERT INTO settings (key, value) VALUES (?, ?)`
	statementSettingUpdate    = `UPDATE settings SET value = ? WHERE key = ?`
	statementSettingDelete    = `DELETE FROM settings WHERE key = ?`
)

// settingsEntry is the registry entry of the settings table. It is a table of
// things created whole and not of optional facts because each key has a default
// of its own, and an optional-facts column has one for every row.
func settingsEntry() Table {
	return Table{
		Name: settingsTable, Shape: ShapeAttribute, Kind: KindCreatedWhole, Key: []string{"key"},
		Columns: []Column{{Name: "key"}, {Name: "value"}},
		Statements: TableStatements{
			SelectRow: statementSettingSelect, SelectAll: statementSettingSelectAll,
			Insert: statementSettingInsert, Update: statementSettingUpdate, Delete: statementSettingDelete,
		},
	}
}

// storedSetting is the value stored for key, and false when nothing is.
func (view *View) storedSetting(ctx context.Context, key string) (Value, bool, error) {
	row, found, err := view.JournaledRow(ctx, settingsTable, []Value{Text(key)})
	if err != nil || !found {
		return Null, false, err
	}
	return row["value"], true, nil
}

func (view *View) integerSetting(ctx context.Context, key string, fallback int64) (int64, error) {
	value, found, err := view.storedSetting(ctx, key)
	if err != nil || !found {
		return fallback, err
	}
	number, ok := value.Int64()
	if !ok {
		return 0, &TransactionError{cause: fmt.Errorf("authored: setting %s does not hold an integer", key)}
	}
	return number, nil
}

func (view *View) textSetting(ctx context.Context, key, fallback string) (string, error) {
	value, found, err := view.storedSetting(ctx, key)
	if err != nil || !found {
		return fallback, err
	}
	text, ok := value.Str()
	if !ok {
		return "", &TransactionError{cause: fmt.Errorf("authored: setting %s does not hold a text", key)}
	}
	return text, nil
}

// ConfidenceThreshold is how sure, in percent, the classifier must be before it
// files a purchase by itself.
func (view *View) ConfidenceThreshold(ctx context.Context) (int64, error) {
	return view.integerSetting(ctx, keyConfidenceThreshold, DefaultConfidenceThreshold)
}

// MinExamples is how many examples the classifier needs before it files by itself.
func (view *View) MinExamples(ctx context.Context) (int64, error) {
	return view.integerSetting(ctx, keyMinExamples, DefaultMinExamples)
}

// CardLabel is what the card is called.
func (view *View) CardLabel(ctx context.Context) (string, error) {
	return view.textSetting(ctx, keyCardLabel, DefaultCardLabel)
}

// CardClosingMode is whether the card's bill closes by the bank's date or by
// the manual day.
func (view *View) CardClosingMode(ctx context.Context) (ClosingMode, error) {
	mode, err := view.textSetting(ctx, keyClosingMode, string(DefaultClosingMode))
	return ClosingMode(mode), err
}

// ManualClosingDay is the day of the month the person chose for the bill to close.
func (view *View) ManualClosingDay(ctx context.Context) (int64, error) {
	return view.integerSetting(ctx, keyManualClosingDay, DefaultManualClosingDay)
}

// setSetting makes the setting hold value. A value equal to the stored one
// records nothing. A value equal to the default with nothing stored still
// stores a row, so that what the person chose outlasts a later change of the
// default; undoing it removes the row again.
func (recorder *Recorder) setSetting(ctx context.Context, key string, value Value) error {
	keyValue := []Value{Text(key)}
	row, found, err := recorder.View().JournaledRow(ctx, settingsTable, keyValue)
	if err != nil {
		return recorder.fail(err)
	}
	if !found {
		return recorder.Create(ctx, settingsTable, Row{"key": Text(key), "value": value})
	}
	if row["value"] == value {
		return nil
	}
	return recorder.Set(ctx, settingsTable, keyValue, Row{"value": value})
}

// SetConfidenceThreshold records the threshold, a whole number from 50 to 99.
func (recorder *Recorder) SetConfidenceThreshold(ctx context.Context, percent int64) error {
	return recorder.setSetting(ctx, keyConfidenceThreshold, Integer(percent))
}

// SetMinExamples records the minimum of examples, a whole number from 1 to 50.
func (recorder *Recorder) SetMinExamples(ctx context.Context, examples int64) error {
	return recorder.setSetting(ctx, keyMinExamples, Integer(examples))
}

// SetCardLabel records the card's label.
func (recorder *Recorder) SetCardLabel(ctx context.Context, label string) error {
	return recorder.setSetting(ctx, keyCardLabel, Text(label))
}

// SetClosingMode records how the card's bill closes.
func (recorder *Recorder) SetClosingMode(ctx context.Context, mode ClosingMode) error {
	return recorder.setSetting(ctx, keyClosingMode, Text(string(mode)))
}

// SetManualClosingDay records the manual closing day, a whole number from 1 to 28.
func (recorder *Recorder) SetManualClosingDay(ctx context.Context, day int64) error {
	return recorder.setSetting(ctx, keyManualClosingDay, Integer(day))
}

// SetCardClosing records the closing mode and the manual day in the change it
// is called in: they are two facts that the one choice on the settings screen
// sets together.
func (recorder *Recorder) SetCardClosing(ctx context.Context, mode ClosingMode, day int64) error {
	if err := recorder.SetClosingMode(ctx, mode); err != nil {
		return err
	}
	return recorder.SetManualClosingDay(ctx, day)
}
