package api

import "errors"

// maxWholeNumber bounds a whole number the person sets; the handler then clamps
// it to its own range.
const maxWholeNumber = 1_000_000_000

var errWholeNumberOutOfBounds = errors.New("api: the number is beyond one billion")

// ParseWholeNumber reads a JSON number into the nearest whole number, a half
// upward (72.5 is 73, -72.5 is -72), which is what the interface's Math.round
// gives, but decided on the decimal text rather than on a float.
func ParseWholeNumber(raw []byte) (int, error) {
	number, err := readJSONNumber(raw)
	if err != nil {
		return 0, err
	}
	// Ten digits is the most a value within the bound can have before rounding.
	magnitude, remainder, ok := number.scaled(0, 10)
	if !ok {
		return 0, errWholeNumberOutOfBounds
	}
	if roundsMagnitudeUp(number.negative, remainder) {
		magnitude++
	}
	if magnitude > maxWholeNumber {
		return 0, errWholeNumberOutOfBounds
	}
	if number.negative {
		return -int(magnitude), nil
	}
	return int(magnitude), nil
}

// roundsMagnitudeUp rounds toward positive infinity on an exact half: a
// positive value moves away from zero at a half, a negative one only past it.
func roundsMagnitudeUp(negative bool, remainder []byte) bool {
	if len(remainder) == 0 || remainder[0] < '5' {
		return false
	}
	if !negative || remainder[0] > '5' {
		return true
	}
	for _, digit := range remainder[1:] {
		if digit != '0' {
			return true
		}
	}
	return false
}
