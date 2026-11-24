package authored

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"time"

	"github.com/gabrielassisxyz/finwise/internal/brday"
)

// ModelAnswer is how the person answered a model suggestion in the weekly queue.
type ModelAnswer string

const (
	// ModelAnswerAccepted means the person accepted the line the model suggested.
	ModelAnswerAccepted ModelAnswer = "accepted"
	// ModelAnswerCorrected means the person chose another line instead.
	ModelAnswerCorrected ModelAnswer = "corrected"
)

func (answer ModelAnswer) valid() bool {
	return answer == ModelAnswerAccepted || answer == ModelAnswerCorrected
}

// ErrInvalidModelAnswer is a queue answer outside accepted and corrected.
var ErrInvalidModelAnswer = errors.New("authored: a model suggestion answer is accepted or corrected")

// ModelSuggestion is a stored row of model_suggestions. Line, Confidence and
// Justification are all set for a usable suggestion and all nil when the
// transaction was asked without a usable answer.
type ModelSuggestion struct {
	Key           TxKey
	Model         string
	AskedAt       time.Time
	Line          *LineID
	Confidence    *int64
	Justification *string
	Answer        ModelAnswer
	AnswerChange  *ChangeID
}

// ModelSuggestionInput is one transaction's outcome from a model request:
// either a usable suggestion (Line, Confidence, Justification all non-nil) or
// an unusable/omitted outcome (all three nil).
type ModelSuggestionInput struct {
	Key           TxKey
	Line          *LineID
	Confidence    *int64
	Justification *string
}

// PersonExample is the newest transaction of one establishment whose line the
// person set.
type PersonExample struct {
	Key              TxKey
	Day              brday.Day
	EstablishmentKey string
	Line             LineID
}

const (
	// ON CONFLICT (tx_key) DO NOTHING enforces "each once" without silencing
	// CHECK or foreign-key violations the way INSERT OR IGNORE would.
	statementInsertModelSuggestion = `
INSERT INTO model_suggestions (tx_key, model, asked_at, line_id, confidence, justification)
VALUES (?1, ?2, ?3, ?4, ?5, ?6)
ON CONFLICT (tx_key) DO NOTHING`

	statementModelSuggestionsOf = `
SELECT tx_key, model, asked_at, line_id, confidence, justification, answer, answer_change_id
FROM model_suggestions
WHERE tx_key IN (SELECT value FROM json_each(?))`

	statementRecordModelAnswer = `
UPDATE model_suggestions
SET answer = ?1, answer_change_id = ?2
WHERE tx_key = ?3 AND line_id IS NOT NULL`

	// Newest transaction per establishment whose line the person set, excluding
	// 11-digit CPF keys (transfers to a person). Callers also verify against the
	// ledger's own Person and Movement flags.
	statementPersonExamples = `
WITH ranked AS (
  SELECT i.tx_key, i.day, i.establishment_key, s.line_id,
         ROW_NUMBER() OVER (PARTITION BY i.establishment_key ORDER BY i.day DESC, i.tx_key DESC) AS rank
  FROM tx_state AS s
  JOIN tx_index AS i ON i.tx_key = s.tx_key
  WHERE s.classified_by = 'person'
    AND s.line_id IS NOT NULL
    AND NOT (i.establishment_key GLOB 'doc:[0-9][0-9][0-9][0-9][0-9][0-9][0-9][0-9][0-9][0-9][0-9]')
)
SELECT tx_key, day, establishment_key, line_id
FROM ranked
WHERE rank = 1
ORDER BY day DESC, tx_key DESC`
)

// RecordModelSuggestions stores one request's model suggestions in a single
// transaction. A transaction that already has a row is left untouched ("each
// once"). Nothing is written to the journal.
func (database *Database) RecordModelSuggestions(ctx context.Context, model string, askedAt time.Time, items []ModelSuggestionInput) error {
	if len(items) == 0 {
		return nil
	}
	instant := formatInstant(askedAt)
	return database.write(ctx, func(tx *sql.Tx) error {
		for _, item := range items {
			var line any
			if item.Line != nil {
				line = int64(*item.Line)
			}
			var confidence any
			if item.Confidence != nil {
				confidence = *item.Confidence
			}
			var justification any
			if item.Justification != nil {
				justification = *item.Justification
			}
			if _, err := tx.ExecContext(ctx, statementInsertModelSuggestion,
				int64(item.Key), model, instant, line, confidence, justification); err != nil {
				return err
			}
		}
		return nil
	})
}

// ModelSuggestions is every stored model suggestion for keys. A key never asked
// is absent; a key asked with no usable answer is present with Line nil.
func (view *View) ModelSuggestions(ctx context.Context, keys []TxKey) (map[TxKey]ModelSuggestion, error) {
	found := map[TxKey]ModelSuggestion{}
	if len(keys) == 0 {
		return found, nil
	}
	_, err := view.read(ctx, func(tx *sql.Tx) error {
		set, err := json.Marshal(keys)
		if err != nil {
			return err
		}
		rows, err := tx.QueryContext(ctx, statementModelSuggestionsOf, string(set))
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var key int64
			var model, askedAt string
			var line, confidence, answerChange sql.NullInt64
			var justification, answer sql.NullString
			if err := rows.Scan(&key, &model, &askedAt, &line, &confidence, &justification, &answer, &answerChange); err != nil {
				return err
			}
			parsedAt, err := time.Parse(instantLayout, askedAt)
			if err != nil {
				return err
			}
			suggestion := ModelSuggestion{
				Key:          TxKey(key),
				Model:        model,
				AskedAt:      parsedAt,
				Line:         nullable[LineID](line),
				Confidence:   nullable[int64](confidence),
				Answer:       ModelAnswer(answer.String),
				AnswerChange: nullable[ChangeID](answerChange),
			}
			if justification.Valid {
				text := justification.String
				suggestion.Justification = &text
			}
			found[TxKey(key)] = suggestion
		}
		return rows.Err()
	})
	return found, err
}

// RecordModelAnswer records in the recorder's open change transaction how the
// person answered the model's suggestion in the weekly queue. model_suggestions
// is not journaled: undoing the change leaves the row, and a later answer
// overwrites it.
func (recorder *Recorder) RecordModelAnswer(ctx context.Context, key TxKey, answer ModelAnswer) error {
	if !answer.valid() {
		return ErrInvalidModelAnswer
	}
	changeID, err := recorder.ID(ctx)
	if err != nil {
		return err
	}
	if _, err := recorder.tx.ExecContext(ctx, statementRecordModelAnswer, string(answer), int64(changeID), int64(key)); err != nil {
		return recorder.fail(err)
	}
	return nil
}

// PersonExamples is the newest transaction per establishment whose line the
// person set, newest first, excluding CPF-keyed transfers to a person.
func (view *View) PersonExamples(ctx context.Context) ([]PersonExample, error) {
	var examples []PersonExample
	_, err := view.read(ctx, func(tx *sql.Tx) error {
		rows, err := tx.QueryContext(ctx, statementPersonExamples)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var key, line int64
			var day, establishmentKey string
			if err := rows.Scan(&key, &day, &establishmentKey, &line); err != nil {
				return err
			}
			examples = append(examples, PersonExample{
				Key:              TxKey(key),
				Day:              brday.Day(day),
				EstablishmentKey: establishmentKey,
				Line:             LineID(line),
			})
		}
		return rows.Err()
	})
	return examples, err
}
