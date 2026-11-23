package api

import (
	"context"

	"github.com/gabrielassisxyz/finwise/internal/brday"
	"github.com/gabrielassisxyz/finwise/internal/budgetrules"
)

// monthLimit is the variable cap of the month asked, what was spent against
// it and its weekly guides, which the home and the weekly review show.
type monthLimit struct {
	limit budgetrules.MonthLimit
	weeks []budgetrules.WeekGuide
}

// monthLimitOf weighs the budget in force for the month asked, not the current
// month's, against that month's dispositions, as budget.view weighs its
// variable group; today is the request's Brazil day.
func monthLimitOf(ctx context.Context, views Views, month brday.Month) (monthLimit, error) {
	disposed, err := disposeMonth(ctx, views, month)
	if err != nil {
		return monthLimit{}, err
	}
	budget, err := views.Authored.BudgetOf(ctx, month)
	if err != nil {
		return monthLimit{}, err
	}
	plan := planOfBudget(budget)
	limit := budgetrules.VariableLimit(plan, budgetrules.LineFigures(disposed.outcomes))
	dated := make([]budgetrules.DatedOutcome, 0, len(disposed.outcomes))
	for i, outcome := range disposed.outcomes {
		dated = append(dated, budgetrules.DatedOutcome{Day: disposed.listed[i].Day, Outcome: outcome})
	}
	weeks, err := budgetrules.WeekGuides(month, brday.DayOf(nowOf(ctx)), limit.Cap, budgetrules.VariableSpendingByDay(plan, dated))
	if err != nil {
		return monthLimit{}, err
	}
	return monthLimit{limit: limit, weeks: weeks}, nil
}

// weekGuideWire is the contract's WeekGuide.
type weekGuideWire struct {
	Label   string `json:"label"`
	Guide   Reais  `json:"guide"`
	Spent   *Reais `json:"spent"`
	Current bool   `json:"current"`
}

// weeklyGuideWire is the contract's WeeklyGuide: the current week.
type weeklyGuideWire struct {
	Guide Reais `json:"guide"`
	Spent Reais `json:"spent"`
	Fill  int64 `json:"fill"`
}

func (month monthLimit) capWire() monthLimitWire {
	return monthLimitWire{Cap: Reais(month.limit.Cap), Spent: Reais(month.limit.Spent)}
}

func (month monthLimit) weeksWire() []weekGuideWire {
	weeks := make([]weekGuideWire, 0, len(month.weeks))
	for _, week := range month.weeks {
		wire := weekGuideWire{Label: week.Label, Guide: Reais(week.Guide), Current: week.Current}
		if week.Spent != nil {
			spent := Reais(*week.Spent)
			wire.Spent = &spent
		}
		weeks = append(weeks, wire)
	}
	return weeks
}

func (month monthLimit) weeklyLimitWire() weeklyLimit {
	return weeklyLimit{
		Cap: Reais(month.limit.Cap), Spent: Reais(month.limit.Spent), Remaining: Reais(month.limit.Remaining()),
		Over: month.limit.Over(), Fill: int(month.limit.Fill()),
	}
}

// currentWeekWire is nil when the month has no week holding today, which is
// every month but the current one, and a month with no cap.
func (month monthLimit) currentWeekWire() *weeklyGuideWire {
	for _, week := range month.weeks {
		if week.Current {
			return &weeklyGuideWire{Guide: Reais(week.Guide), Spent: Reais(*week.Spent), Fill: week.Fill()}
		}
	}
	return nil
}
