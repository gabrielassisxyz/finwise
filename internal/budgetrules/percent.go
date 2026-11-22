package budgetrules

import "github.com/gabrielassisxyz/finwise/internal/money"

// RoundedPercent is part as a whole percent of whole, rounded half toward
// positive infinity as the interface's Math.round does: 12.5 is 13 and -12.5 is
// -12. It is ⌊(200·part + whole) / (2·whole)⌋ with a floor division, because
// Go's division truncates toward zero and would round a negative share the
// wrong way. whole must be above zero; for any other whole it answers 0.
func RoundedPercent(part, whole money.Cents) int64 {
	if whole <= 0 {
		return 0
	}
	return floorDivision(200*int64(part)+int64(whole), 2*int64(whole))
}

// Share is the part of whole reached, as a whole percent held between 0 and
// 100, and 0 for a whole of zero: a bar cannot run backwards or past its end.
func Share(part, whole money.Cents) int64 {
	if whole <= 0 {
		return 0
	}
	return min(max(RoundedPercent(part, whole), 0), 100)
}

// floorDivision is the quotient rounded toward negative infinity, for a
// divisor above zero.
func floorDivision(dividend, divisor int64) int64 {
	quotient := dividend / divisor
	if dividend%divisor < 0 {
		quotient--
	}
	return quotient
}
