package txrules

import (
	"regexp"
	"strings"
	"time"

	"github.com/gabrielassisxyz/finwise/internal/brday"
)

// Normalisation stops at case, accents, runs of spaces and a trailing
// instalment marker. Punctuation and digits stay: folding them as well would
// merge establishments whose names differ only there, and a wrong merge is
// harder to see than a missed one.
var (
	instalmentMarker = regexp.MustCompile(`\s*\b\d{1,2}/\d{1,2}$`)
	accentFolds      = strings.NewReplacer(
		"Á", "A", "À", "A", "Â", "A", "Ã", "A", "Ä", "A",
		"É", "E", "È", "E", "Ê", "E", "Ë", "E",
		"Í", "I", "Ì", "I", "Î", "I", "Ï", "I",
		"Ó", "O", "Ò", "O", "Ô", "O", "Õ", "O", "Ö", "O",
		"Ú", "U", "Ù", "U", "Û", "U", "Ü", "U",
		"Ç", "C", "Ñ", "N",
	)
)

func collapseSpaces(text string) string {
	return strings.Join(strings.Fields(text), " ")
}

// foldText is the form keywords are matched in.
func foldText(text string) string {
	return accentFolds.Replace(strings.ToUpper(collapseSpaces(text)))
}

// establishmentText is foldText without the instalment marker, so every
// instalment of one purchase shares it.
func establishmentText(text string) string {
	return instalmentMarker.ReplaceAllString(foldText(text), "")
}

// displayName is the description as the person should read it.
func displayName(text string) string {
	return instalmentMarker.ReplaceAllString(collapseSpaces(text), "")
}

func hasPrefixWord(folded, keyword string) bool {
	if !strings.HasPrefix(folded, keyword) {
		return false
	}
	rest := folded[len(keyword):]
	return rest == "" || rest[0] == ' ' || rest[0] == '-'
}

func containsWord(folded, word string) bool {
	for _, field := range strings.FieldsFunc(folded, func(r rune) bool { return !isLetterOrDigit(r) }) {
		if field == word {
			return true
		}
	}
	return false
}

func isLetterOrDigit(r rune) bool {
	return (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9')
}

// daysApart is the absolute number of calendar days between two Brazil days.
func daysApart(first, second brday.Day) int {
	a, errA := time.Parse("2006-01-02", string(first))
	b, errB := time.Parse("2006-01-02", string(second))
	if errA != nil || errB != nil {
		return 1 << 30
	}
	days := int(a.Sub(b).Hours() / 24)
	if days < 0 {
		return -days
	}
	return days
}

func digitsOf(text string) string {
	var digits strings.Builder
	for _, r := range text {
		if r >= '0' && r <= '9' {
			digits.WriteRune(r)
		}
	}
	return digits.String()
}
