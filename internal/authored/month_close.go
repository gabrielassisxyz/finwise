package authored

import (
	"context"
	"database/sql"
	"errors"
	"slices"

	"github.com/gabrielassisxyz/finwise/internal/brday"
)

// CloseMonth closes month on the day given, inside the open change: the month
// is written to closed_months and, when no version starts at it, a version is
// written there holding exactly the budget in force for it, read before
// anything is written and kept by the same function a save keeps a later
// closed month's budget with. With no version at all, that version has no
// line. A month already closed is refused with ErrMonthClosed and nothing is
// written.
//
// The version is a copy of what every month that followed the same version
// already had, so no month's budget moves: the close clears no line reference
// and needs no rule run after it. What a closed month keeps is then a row of
// its own, which a later save leaves alone.
func (recorder *Recorder) CloseMonth(ctx context.Context, month brday.Month, on brday.Day) error {
	if err := recorder.usable(); err != nil {
		return err
	}
	copies, err := planClose(ctx, recorder.tx, month)
	if errors.Is(err, ErrMonthClosed) {
		return err
	}
	if err != nil {
		return recorder.fail(err)
	}
	return recorder.guard(recorder.writeClose(ctx, month, on, copies))
}

func planClose(ctx context.Context, tx *sql.Tx, month brday.Month) ([]plannedVersion, error) {
	closed, err := closedMonths(ctx, tx)
	if err != nil {
		return nil, err
	}
	if slices.Contains(closed, month) {
		return nil, ErrMonthClosed
	}
	inForce, err := budgetInForce(ctx, tx, month)
	if err != nil {
		return nil, err
	}
	var own []brday.Month
	if inForce.Exists && inForce.From == month {
		own = append(own, month)
	}
	return keptClosedVersions(ctx, tx, []brday.Month{month}, own)
}

func (recorder *Recorder) writeClose(ctx context.Context, month brday.Month, on brday.Day, copies []plannedVersion) error {
	closed, err := recorder.registry.table(closedMonthsTable.Name)
	if err != nil {
		return err
	}
	if err := recorder.put(ctx, closed, Row{"month": Text(string(month)), "closed_on": Text(string(on))}); err != nil {
		return err
	}
	if len(copies) == 0 {
		return nil
	}
	versions, err := recorder.registry.table(budgetVersionsTable.Name)
	if err != nil {
		return err
	}
	lines, err := recorder.registry.table(budgetVersionLinesTable.Name)
	if err != nil {
		return err
	}
	for _, version := range copies {
		if err := recorder.writeVersion(ctx, versions, lines, version); err != nil {
			return err
		}
	}
	return nil
}
