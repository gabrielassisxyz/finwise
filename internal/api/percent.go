package api

// Percent is a rate as the bank states it, written with two decimals. The
// ledger holds it as a ratio, and a ratio times a hundred is not exact in
// floating point (1.1 becomes 110.00000000000001), so it is formatted once, when
// it is read, and never held as a float afterwards.
type Percent string

func (rate Percent) MarshalJSON() ([]byte, error) { return []byte(rate), nil }

func percentPointer(text string) *Percent {
	rate := Percent(text)
	return &rate
}
