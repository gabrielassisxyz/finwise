package api

import (
	"bytes"
	"errors"
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

// UnmarshalJSON refuses null like any other non-number, so an amount that was
// left out can never read as zero; a field that may be absent is a *Reais,
// which the decoder leaves nil for null without calling this.
func (amount *Reais) UnmarshalJSON(raw []byte) error {
	cents, err := ParseReais(raw)
	if err != nil {
		return err
	}
	*amount = Reais(cents)
	return nil
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

// maxTypedCents is one billion reais. What a person types is held to it, a
// tighter bound than the ledger's, which is trusted up to 1e12.
const maxTypedCents = 100_000_000_000

var (
	errNotAJSONNumber    = errors.New("api: the value is not a JSON number")
	errAmountOutOfBounds = errors.New("api: the amount is beyond one billion reais")
)

// ParseReais reads a JSON number in reais into cents from its decimal text, so
// that 1.005 is 101 and not the 100 a float would make of it. Beyond two places
// it rounds half away from zero.
func ParseReais(raw []byte) (money.Cents, error) {
	number, err := readJSONNumber(raw)
	if err != nil {
		return 0, err
	}
	// Twelve digits is the most a value within the bound can have before rounding.
	magnitude, remainder, ok := number.scaled(2, 12)
	if !ok {
		return 0, errAmountOutOfBounds
	}
	if len(remainder) > 0 && remainder[0] >= '5' {
		magnitude++
	}
	if magnitude > maxTypedCents {
		return 0, errAmountOutOfBounds
	}
	if number.negative {
		return -money.Cents(magnitude), nil
	}
	return money.Cents(magnitude), nil
}

// jsonNumber is a JSON number as digits times a power of ten: the value is
// digits × 10^exponent. It never holds the number itself, so an exponent of a
// billion costs nothing.
type jsonNumber struct {
	negative bool
	digits   []byte // without leading zeros; empty for zero
	exponent int64
}

// maxExponent caps the exponent as it is read. Any value past it is out of every
// bound or rounds to zero, so saturating changes no answer and keeps the
// arithmetic on the exponent from overflowing.
const maxExponent = 1_000_000_000_000_000

// readJSONNumber accepts exactly the grammar of RFC 8259, section 6, around
// optional JSON whitespace.
func readJSONNumber(raw []byte) (jsonNumber, error) {
	text := bytes.Trim(raw, " \t\r\n")
	var number jsonNumber
	if len(text) > 0 && text[0] == '-' {
		number.negative, text = true, text[1:]
	}
	whole, text := leadingDigits(text)
	if len(whole) == 0 || (len(whole) > 1 && whole[0] == '0') {
		return jsonNumber{}, errNotAJSONNumber
	}
	var fraction []byte
	if len(text) > 0 && text[0] == '.' {
		if fraction, text = leadingDigits(text[1:]); len(fraction) == 0 {
			return jsonNumber{}, errNotAJSONNumber
		}
	}
	exponent, text, err := readExponent(text)
	if err != nil || len(text) != 0 {
		return jsonNumber{}, errNotAJSONNumber
	}
	number.digits = bytes.TrimLeft(joinDigits(whole, fraction), "0")
	number.exponent = exponent - int64(len(fraction))
	return number, nil
}

// joinDigits copies only when there is a fraction: the point sits between the
// two parts in the input.
func joinDigits(whole, fraction []byte) []byte {
	if len(fraction) == 0 {
		return whole
	}
	return append(append(make([]byte, 0, len(whole)+len(fraction)), whole...), fraction...)
}

// readExponent reads an optional e or E part, saturating at maxExponent.
func readExponent(text []byte) (int64, []byte, error) {
	if len(text) == 0 || (text[0] != 'e' && text[0] != 'E') {
		return 0, text, nil
	}
	text = text[1:]
	negative := false
	if len(text) > 0 && (text[0] == '+' || text[0] == '-') {
		negative, text = text[0] == '-', text[1:]
	}
	digits, rest := leadingDigits(text)
	if len(digits) == 0 {
		return 0, text, errNotAJSONNumber
	}
	var exponent int64
	for i := 0; i < len(digits) && exponent < maxExponent; i++ {
		exponent = exponent*10 + int64(digits[i]-'0')
	}
	exponent = min(exponent, maxExponent)
	if negative {
		exponent = -exponent
	}
	return exponent, rest, nil
}

func leadingDigits(text []byte) (digits, rest []byte) {
	end := 0
	for end < len(text) && text[end] >= '0' && text[end] <= '9' {
		end++
	}
	return text[:end], text[end:]
}

// belowHalfOfTheLastPlace is the remainder of a value too small to reach the
// first digit after the cut: that digit is a zero.
var belowHalfOfTheLastPlace = []byte{'0'}

// scaled is the magnitude times 10^places, cut into its whole part and the
// digits after it, the first of which decides a half. A whole part of more than
// maxDigits digits is refused before any of it is built.
func (number jsonNumber) scaled(places int64, maxDigits int) (uint64, []byte, bool) {
	if len(number.digits) == 0 {
		return 0, nil, true
	}
	length := int64(len(number.digits))
	shift := number.exponent + places
	if length+shift > int64(maxDigits) {
		return 0, nil, false
	}
	cut := min(length, length+shift)
	if cut < 0 {
		return 0, belowHalfOfTheLastPlace, true
	}
	var whole uint64
	for _, digit := range number.digits[:cut] {
		whole = whole*10 + uint64(digit-'0')
	}
	for ; shift > 0; shift-- {
		whole *= 10
	}
	return whole, number.digits[cut:], true
}
