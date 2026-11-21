// Package money holds amounts as integer cents. The ledger hands amounts over as
// floating point read out of JSON; they are converted once, at the boundary, so
// that nothing downstream ever adds floats.
package money

import (
	"fmt"
	"math"
	"strconv"
	"strings"
)

// Cents is an amount in hundredths of a currency unit.
type Cents int64

// maxMagnitude is the collector's own bound for an amount worth trusting
// (pluggy-sync README, "Monetary values come out of SQLite's JSON functions").
const maxMagnitude = 1e12

// FromFloat converts an amount to cents, rounding half away from zero.
//
// It rounds the shortest decimal the float prints as, not the float's binary
// value: 1.005 is stored a hair below 1.005, and multiplying it by 100 would
// round it down to 100 where the figure on the statement says 101.
func FromFloat(amount float64) (Cents, error) {
	if math.IsNaN(amount) || math.IsInf(amount, 0) {
		return 0, fmt.Errorf("money: amount %v is not finite", amount)
	}
	if math.Abs(amount) >= maxMagnitude {
		return 0, fmt.Errorf("money: amount %v is beyond ±%.0e", amount, maxMagnitude)
	}
	text := strconv.FormatFloat(amount, 'f', -1, 64)
	negative := strings.HasPrefix(text, "-")
	text = strings.TrimPrefix(text, "-")
	whole, fraction, _ := strings.Cut(text, ".")
	fraction += "000"

	units, err := strconv.ParseInt(whole, 10, 64)
	if err != nil {
		return 0, fmt.Errorf("money: amount %q: %w", text, err)
	}
	cents := units*100 + int64(fraction[0]-'0')*10 + int64(fraction[1]-'0')
	if fraction[2] >= '5' {
		cents++
	}
	if negative {
		cents = -cents
	}
	return Cents(cents), nil
}
