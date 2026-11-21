package ledger

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"time"

	"github.com/gabrielassisxyz/finwise/internal/brday"
	"github.com/gabrielassisxyz/finwise/internal/money"
)

// A value the reader cannot parse is an error, never a skipped row: a silently
// dropped transaction is the failure this package exists to prevent.

func parseInstant(text, field string) (time.Time, error) {
	instant, err := time.Parse(time.RFC3339Nano, text)
	if err != nil {
		return time.Time{}, fmt.Errorf("ledger: %s is not an ISO-8601 instant", field)
	}
	return instant.UTC(), nil
}

func optionalInstant(text sql.NullString, field string) (*time.Time, error) {
	if !text.Valid {
		return nil, nil
	}
	instant, err := parseInstant(text.String, field)
	if err != nil {
		return nil, err
	}
	return &instant, nil
}

// dayOfDatePart takes the calendar date a midnight-UTC business date carries,
// as the collector's own views do (pluggysync/views.py, BUSINESS_DATE_SQL).
func dayOfDatePart(text, field string) (brday.Day, error) {
	const dateLength = len("2006-01-02")
	if len(text) < dateLength {
		return "", fmt.Errorf("ledger: %s does not start with a YYYY-MM-DD date", field)
	}
	day, err := brday.ParseDay(text[:dateLength])
	if err != nil {
		return "", fmt.Errorf("ledger: %s does not start with a YYYY-MM-DD date", field)
	}
	return day, nil
}

func optionalDayOfDatePart(text sql.NullString, field string) (*brday.Day, error) {
	if !text.Valid {
		return nil, nil
	}
	day, err := dayOfDatePart(text.String, field)
	if err != nil {
		return nil, err
	}
	return &day, nil
}

func requiredCents(amount sql.NullFloat64, field string) (money.Cents, error) {
	if !amount.Valid {
		return 0, fmt.Errorf("ledger: %s is null", field)
	}
	cents, err := money.FromFloat(amount.Float64)
	if err != nil {
		return 0, fmt.Errorf("ledger: %s: %w", field, err)
	}
	return cents, nil
}

func optionalCents(amount sql.NullFloat64, field string) (*money.Cents, error) {
	if !amount.Valid {
		return nil, nil
	}
	cents, err := requiredCents(amount, field)
	if err != nil {
		return nil, err
	}
	return &cents, nil
}

func centsFromJSON(amount *float64, field string) (*money.Cents, error) {
	if amount == nil {
		return nil, nil
	}
	return optionalCents(sql.NullFloat64{Float64: *amount, Valid: true}, field)
}

func stringPointer(text sql.NullString) *string {
	if !text.Valid {
		return nil
	}
	return &text.String
}

func decodeRaw(raw, table string, into any) error {
	if err := json.Unmarshal([]byte(raw), into); err != nil {
		return fmt.Errorf("ledger: %s.raw: %w", table, err)
	}
	return nil
}
