package subrules

import "slices"

// Cycle is how often a subscription is charged.
type Cycle int

const (
	Monthly Cycle = iota + 1
	Quarterly
	Semiannual
	Yearly
	Biennial
)

// cycleLength is a cycle's length in days, which a median gap is compared
// with, and in months, which its next charge and its equivalents count in.
type cycleLength struct {
	cycle  Cycle
	days   int
	months int
}

// cycleLengths are the five cycles from the shortest; that order is what makes
// the shorter one win when a median is as far from two of them.
var cycleLengths = []cycleLength{
	{Monthly, 30, 1},
	{Quarterly, 91, 3},
	{Semiannual, 182, 6},
	{Yearly, 365, 12},
	{Biennial, 730, 24},
}

// gapsConsidered is how many of the latest gaps between charges decide the
// cycle, so a plan that changed is read as it is now.
const gapsConsidered = 6

// Cycles are the five cycles from the shortest.
func Cycles() []Cycle {
	cycles := make([]Cycle, len(cycleLengths))
	for index, length := range cycleLengths {
		cycles[index] = length.cycle
	}
	return cycles
}

// Months is the cycle's length in months; zero for a value that is not one
// of the five.
func (cycle Cycle) Months() int {
	for _, length := range cycleLengths {
		if length.cycle == cycle {
			return length.months
		}
	}
	return 0
}

// cycleOf reads the cycle from the gaps between consecutive charge days:
// monthly with fewer than two days, otherwise the cycle nearest the median of
// the latest gaps.
func cycleOf(charges []Charge) (Cycle, error) {
	if len(charges) < 2 {
		return Monthly, nil
	}
	gaps := make([]int, 0, len(charges)-1)
	for index := 1; index < len(charges); index++ {
		gap, err := DaysBetween(charges[index-1].Day, charges[index].Day)
		if err != nil {
			return 0, err
		}
		gaps = append(gaps, gap)
	}
	return cycleNearest(medianGap(gaps)), nil
}

// medianGap is the median of the latest gaps; of an even count, the lower of
// the two middle ones.
func medianGap(gaps []int) int {
	latest := slices.Clone(gaps[max(0, len(gaps)-gapsConsidered):])
	slices.Sort(latest)
	return latest[(len(latest)-1)/2]
}

// cycleNearest is the cycle whose length is nearest the median, the shorter
// one when two are as near.
func cycleNearest(median int) Cycle {
	nearest := cycleLengths[0]
	for _, length := range cycleLengths[1:] {
		if distance(median, length.days) < distance(median, nearest.days) {
			nearest = length
		}
	}
	return nearest.cycle
}

func distance(a, b int) int {
	if a > b {
		return a - b
	}
	return b - a
}
