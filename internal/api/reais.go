package api

import (
	"strconv"

	"github.com/gabrielassisxyz/finwise/internal/money"
)

// Reais is an amount as the contract carries it: a JSON number in reais with
// two decimals. It is the only place cents become decimal text, written from
// the integer itself, so no float ever touches an amount on its way out.
type Reais money.Cents

// MarshalJSON writes 1999 as 19.99 and -6510 as -65.10.
func (amount Reais) MarshalJSON() ([]byte, error) {
	return []byte(decimalOf(money.Cents(amount))), nil
}

// decimalOf works on the magnitude as unsigned so that the most negative
// integer does not overflow when its sign is dropped.
func decimalOf(cents money.Cents) string {
	magnitude := uint64(cents)
	sign := ""
	if cents < 0 {
		magnitude, sign = -magnitude, "-"
	}
	fraction := strconv.FormatUint(magnitude%100, 10)
	if len(fraction) < 2 {
		fraction = "0" + fraction
	}
	return sign + strconv.FormatUint(magnitude/100, 10) + "." + fraction
}
