package api

import (
	"encoding/json"
	"go/ast"
	"go/parser"
	"go/token"
	"math"
	"path/filepath"
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
