package budgetrules

import (
	"testing"

	"github.com/gabrielassisxyz/finwise/internal/money"
)

func TestAPercentIsWholeAndAHalfGoesTowardPositiveInfinity(t *testing.T) {
	cases := []struct {
		part, whole money.Cents
		want        int64
	}{
		{125, 1000, 13},
		{-125, 1000, -12},
		{380000, 1200000, 32},
		{230000, 1050000, 22},
		{-520000, 300000, -173},
		{-277000, 150000, -185},
		{1, 3, 33},
		{2, 3, 67},
		{-1, 3, -33},
		{-2, 3, -67},
		{0, 7, 0},
		{308, 400, 77},
		{290, 900, 32},
	}
	for _, c := range cases {
		if got := RoundedPercent(c.part, c.whole); got != c.want {
			t.Errorf("RoundedPercent(%d, %d) = %d, want %d", c.part, c.whole, got, c.want)
		}
	}
}

func TestAShareIsHeldBetweenZeroAndAHundred(t *testing.T) {
	cases := []struct {
		part, whole money.Cents
		want        int64
	}{
		{500, 0, 0},
		{0, 0, 0},
		{20600, 20000, 100},
		{-500, 20000, 0},
		{30800, 40000, 77},
		{0, 15000, 0},
		{29000, 90000, 32},
		{60000, 90000, 67},
		{1, 8, 13},
	}
	for _, c := range cases {
		if got := Share(c.part, c.whole); got != c.want {
			t.Errorf("Share(%d, %d) = %d, want %d", c.part, c.whole, got, c.want)
		}
	}
}
