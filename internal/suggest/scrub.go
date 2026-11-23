// Package suggest builds scrubbed model requests for pending establishment
// purchases, calls an OpenAI-compatible chat completions endpoint, validates
// untrusted responses without floats or the host clock, and stores usable and
// unusable outcomes in the app's database.
package suggest

import (
	"strings"
	"unicode"
)

// Replacement is the literal placeholder written in place of any CPF, CNPJ, or
// run of 8 or more digits (with separators allowed between them).
const Replacement = "[número]"

const minSensitiveDigits = 8

// ScrubText replaces anything CPF-shaped, CNPJ-shaped, or any run of 8 or more
// ASCII digits (with '.', '-', '/', '_', spaces or tabs allowed between them)
// by Replacement. Trailing separators after the final digit of a run are kept.
func ScrubText(text string) string {
	var out strings.Builder
	out.Grow(len(text))
	for i := 0; i < len(text); {
		if !isASCIIDigit(text[i]) {
			out.WriteByte(text[i])
			i++
			continue
		}
		digits, end := scanDigitRun(text, i)
		if digits >= minSensitiveDigits {
			out.WriteString(Replacement)
		} else {
			out.WriteString(text[i:end])
		}
		i = end
	}
	return out.String()
}

// ContainsSensitiveRun reports whether text contains any run that ScrubText
// would replace.
func ContainsSensitiveRun(text string) bool {
	return ScrubText(text) != text
}

// IsEmptyExampleName reports whether a scrubbed establishment name carries no
// letter or digit besides Replacement placeholders, in which case the example
// is dropped rather than sent.
func IsEmptyExampleName(scrubbed string) bool {
	stripped := strings.ReplaceAll(scrubbed, Replacement, "")
	for _, r := range stripped {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			return false
		}
	}
	return true
}

func scanDigitRun(text string, start int) (digits, end int) {
	lastDigit := start
	for j := start; j < len(text); j++ {
		if isASCIIDigit(text[j]) {
			digits++
			lastDigit = j
			continue
		}
		if !isDigitSeparator(text[j]) {
			break
		}
	}
	return digits, lastDigit + 1
}

func isASCIIDigit(b byte) bool {
	return b >= '0' && b <= '9'
}

func isDigitSeparator(b byte) bool {
	switch b {
	case '.', '-', '/', '_', ' ', '\t':
		return true
	}
	return false
}
