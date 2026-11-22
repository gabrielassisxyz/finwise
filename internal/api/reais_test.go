package api

import (
	"encoding/json"
	"go/ast"
	"go/parser"
	"go/token"
	"math"
	"math/rand/v2"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/gabrielassisxyz/finwise/internal/money"
)

func TestMoneyIsWrittenAsADecimalWithTwoPlaces(t *testing.T) {
	cases := []struct {
		cents money.Cents
		want  string
	}{
		{1999, "19.99"},
		{-6510, "-65.10"},
		{0, "0.00"},
		{5, "0.05"},
		{-5, "-0.05"},
		{100, "1.00"},
		{-100, "-1.00"},
		{123456789012, "1234567890.12"},
		{math.MaxInt64, "92233720368547758.07"},
		{math.MinInt64, "-92233720368547758.08"},
	}
	for _, c := range cases {
		encoded, err := json.Marshal(Reais(c.cents))
		if err != nil || string(encoded) != c.want {
			t.Errorf("%d cents encoded as %q (err %v), want %q", c.cents, encoded, err, c.want)
		}
	}
}

func TestAnAbsentFigureIsNullAndNotZero(t *testing.T) {
	encoded, err := json.Marshal(struct {
		Tax *Reais `json:"tax"`
	}{})
	if err != nil || string(encoded) != `{"tax":null}` {
		t.Fatalf("got %s, %v", encoded, err)
	}
}

// A float read and multiplied by a hundred is right on round figures and one
// cent off on 1.005, which is why most rows here are not round.
func TestAnAmountIsReadToTheCentFromItsDecimalText(t *testing.T) {
	cases := []struct {
		raw  string
		want money.Cents
	}{
		{"0", 0},
		{"-0", 0},
		{"19.99", 1999},
		{"-65.10", -6510},
		{"8000", 800000},
		{"0.005", 1},
		{"-0.005", -1},
		{"1.005", 101},
		{"-1.005", -101},
		{"1.0049", 100},
		{"2.675", 268},
		{"0.994999", 99},
		{"1e2", 10000},
		{"1.5e-1", 15},
		{"12E-1", 120},
		{"1e-400", 0},
		{"0e999999999", 0},
		{" 19.99\n", 1999},
		{"999999999.99", 99999999999},
		{"1000000000", 100000000000},
		{"1000000000.00", 100000000000},
		{"1e9", 100000000000},
		{"1000000000.004", 100000000000},
		{"-1000000000", -100000000000},
	}
	for _, c := range cases {
		got, err := ParseReais([]byte(c.raw))
		if err != nil || got != c.want {
			t.Errorf("ParseReais(%q) = %d, %v; want %d", c.raw, got, err, c.want)
		}
	}
}

func TestWhatIsNotAnAmountWithinOneBillionReaisIsRefused(t *testing.T) {
	refused := []string{
		"1000000000.005", "1000000000.01", "-1000000000.01", "1e10", "1e400", "1e999999999",
		`"19.99"`, "null", "true", "[1]", "{}", "", " ",
		"1.", ".5", "+1", "01", "-01", "NaN", "Infinity", "-Infinity", "1 2",
		"-", "1e", "1e+", "1.5e", "--1", "0x10", "1_000",
	}
	for _, raw := range refused {
		if got, err := ParseReais([]byte(raw)); err == nil {
			t.Errorf("ParseReais(%q) = %d, want a refusal", raw, got)
		}
	}
}

func TestWhatReaisWritesReadsBackAsTheSameAmount(t *testing.T) {
	amounts := []money.Cents{-maxTypedCents, -maxTypedCents + 1, -1, 0, 1, maxTypedCents - 1, maxTypedCents}
	generator := rand.New(rand.NewPCG(1, 2))
	for range 20000 {
		amounts = append(amounts, money.Cents(generator.Int64N(2*maxTypedCents+1)-maxTypedCents))
	}
	for cents := money.Cents(-1000); cents <= 1000; cents++ {
		amounts = append(amounts, cents)
	}
	for _, cents := range amounts {
		encoded, err := json.Marshal(Reais(cents))
		if err != nil {
			t.Fatal(err)
		}
		if got, err := ParseReais(encoded); err != nil || got != cents {
			t.Fatalf("%d cents written as %s read back as %d, %v", cents, encoded, got, err)
		}
	}
}

func TestAnAmountDecodesInsideTheArgumentAHandlerReads(t *testing.T) {
	var line struct {
		Amount Reais `json:"amount"`
	}
	if err := decodeArgument(json.RawMessage(`{"amount":19.99}`), &line); err != nil || line.Amount != 1999 {
		t.Errorf("decoded %d, %v; want 1999", line.Amount, err)
	}
	for _, raw := range []string{`{"amount":null}`, `{"amount":"19.99"}`} {
		var refused struct {
			Amount Reais `json:"amount"`
		}
		if err := decodeArgument(json.RawMessage(raw), &refused); err == nil {
			t.Errorf("%s decoded to %d, want a refusal", raw, refused.Amount)
		}
	}

	var optional struct {
		Amount *Reais `json:"amount"`
	}
	if err := decodeArgument(json.RawMessage(`{"amount":null}`), &optional); err != nil || optional.Amount != nil {
		t.Errorf("null decoded to %v, %v; want nil", optional.Amount, err)
	}
	if err := decodeArgument(json.RawMessage(`{"amount":-65.10}`), &optional); err != nil || optional.Amount == nil || *optional.Amount != -6510 {
		t.Errorf("-65.10 decoded to %v, %v; want -6510", optional.Amount, err)
	}
}

// The interface rounds these settings with Math.round, a half toward positive
// infinity, so the negative half is the row that tells it apart from rounding
// away from zero.
func TestAWholeNumberRoundsAHalfUpward(t *testing.T) {
	cases := []struct {
		raw  string
		want int
	}{
		{"73", 73},
		{"72.6", 73},
		{"72.5", 73},
		{"7.3e1", 73},
		{"72.4", 72},
		{"-72.5", -72},
		{"-72.50", -72},
		{"-72.6", -73},
		{"-72.51", -73},
		{"-72.500001", -73},
		{"0.5", 1},
		{"-0.5", 0},
		{"0.4", 0},
		{"-0", 0},
		{"1e-400", 0},
		{"0e999999999", 0},
		{"1000000000", 1000000000},
		{"1000000000.4", 1000000000},
		{"-1000000000.5", -1000000000},
	}
	for _, c := range cases {
		got, err := ParseWholeNumber([]byte(c.raw))
		if err != nil || got != c.want {
			t.Errorf("ParseWholeNumber(%q) = %d, %v; want %d", c.raw, got, err, c.want)
		}
	}
}

func TestWhatIsNotAWholeNumberWithinOneBillionIsRefused(t *testing.T) {
	refused := []string{
		"1000000000.5", "1000000001", "-1000000001", "-1000000000.6", "1e10", "1e400", "1e999999999",
		`"73"`, "null", "true", "", "[73]", "{}", "1.", ".5", "+1", "01", "NaN", "Infinity", "7 3",
	}
	for _, raw := range refused {
		if got, err := ParseWholeNumber([]byte(raw)); err == nil {
			t.Errorf("ParseWholeNumber(%q) = %d, want a refusal", raw, got)
		}
	}
}

// A parser that built the number before checking its bound would spend memory
// in proportion to the exponent, which is the person's to choose.
func TestNeitherParserBuildsTheNumberForAnEnormousExponent(t *testing.T) {
	parsers := map[string]func([]byte){
		"ParseReais":       func(raw []byte) { _, _ = ParseReais(raw) },
		"ParseWholeNumber": func(raw []byte) { _, _ = ParseWholeNumber(raw) },
	}
	modestInput, enormousInput := []byte("1e400"), []byte("1e999999999")
	for name, parse := range parsers {
		modest := bytesAllocatedPerCall(func() { parse(modestInput) })
		enormous := bytesAllocatedPerCall(func() { parse(enormousInput) })
		if enormous > modest {
			t.Errorf("%s allocates %d bytes for 1e999999999 against %d for 1e400", name, enormous, modest)
		}
	}
}

// bytesAllocatedPerCall measures on one processor, as testing.AllocsPerRun
// does, so that no other goroutine's allocation is counted.
func bytesAllocatedPerCall(call func()) uint64 {
	defer runtime.GOMAXPROCS(runtime.GOMAXPROCS(1))
	const calls = 100
	call()
	var before, after runtime.MemStats
	runtime.ReadMemStats(&before)
	for range calls {
		call()
	}
	runtime.ReadMemStats(&after)
	return (after.TotalAlloc - before.TotalAlloc) / calls
}

// The guard that keeps the rule from rotting: a handler that went through a
// float would pass every test on round figures and fail on 1.005.
func TestNoSourceFileOfTheHTTPLayerTouchesAFloat(t *testing.T) {
	files, err := filepath.Glob("*.go")
	if err != nil {
		t.Fatal(err)
	}
	for _, file := range files {
		if strings.HasSuffix(file, "_test.go") {
			continue
		}
		parsed, err := parser.ParseFile(token.NewFileSet(), file, nil, 0)
		if err != nil {
			t.Fatal(err)
		}
		ast.Inspect(parsed, func(node ast.Node) bool {
			switch found := node.(type) {
			case *ast.Ident:
				if found.Name == "float64" || found.Name == "float32" {
					t.Errorf("%s uses %s", file, found.Name)
				}
			case *ast.SelectorExpr:
				if found.Sel.Name == "ParseFloat" || found.Sel.Name == "FromFloat" {
					t.Errorf("%s calls %s", file, found.Sel.Name)
				}
			case *ast.BasicLit:
				if found.Kind == token.FLOAT {
					t.Errorf("%s has the float literal %s", file, found.Value)
				}
			}
			return true
		})
	}
}
