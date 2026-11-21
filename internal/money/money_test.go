package money_test

import (
	"math"
	"testing"

	"github.com/gabrielassisxyz/finwise/internal/money"
)

func TestFromFloatRoundsHalfAwayFromZero(t *testing.T) {
	cases := []struct {
		amount float64
		want   money.Cents
	}{
		{19.99, 1999},
		{-0.005, -1},
		{0.005, 1},
		{0.004, 0},
		{-0.004, 0},
		{1.005, 101},
		{2.675, 268},
		{-2.675, -268},
		{0, 0},
		{100, 10000},
		{-1234.5, -123450},
		{0.1, 10},
		{999999999999.99, 99999999999999},
		{1e-7, 0},
	}
	for _, c := range cases {
		got, err := money.FromFloat(c.amount)
		if err != nil {
			t.Fatalf("FromFloat(%v): %v", c.amount, err)
		}
		if got != c.want {
			t.Errorf("FromFloat(%v) = %d, want %d", c.amount, got, c.want)
		}
	}
}

func TestSumOfOneThousandTenCentRowsIsExact(t *testing.T) {
	var total money.Cents
	var floatTotal float64
	for range 1000 {
		cents, err := money.FromFloat(0.10)
		if err != nil {
			t.Fatal(err)
		}
		total += cents
		floatTotal += 0.10
	}
	if total != 10000 {
		t.Fatalf("sum of cents = %d, want 10000", total)
	}
	if floatTotal == 100 {
		t.Fatal("the float sum happened to be exact; the test no longer shows what it guards against")
	}
}

func TestFromFloatRefusesWhatIsNotAnAmount(t *testing.T) {
	for _, amount := range []float64{math.NaN(), math.Inf(1), math.Inf(-1), 1e12, -1e12, 1e300} {
		if _, err := money.FromFloat(amount); err == nil {
			t.Errorf("FromFloat(%v) returned no error", amount)
		}
	}
}
