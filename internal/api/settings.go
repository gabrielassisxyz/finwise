package api

import (
	"context"
	"encoding/json"
	"time"

	"github.com/gabrielassisxyz/finwise/internal/authored"
	"github.com/gabrielassisxyz/finwise/internal/brday"
)

// The settings are what the person chose, kept in the app's database and
// answered with their defaults where nothing is stored, beside what the bank's
// records support: the card, its closing day, the connections, the last
// synchronisation and the pots with the bucket the pots screen already gives
// them.

type settingsPotWire struct {
	ID          string   `json:"id"`
	Name        *string  `json:"name"`
	Bank        string   `json:"bank"`
	CDIPercent  *Percent `json:"cdiPercent"`
	Bucket      *string  `json:"bucket"`
	LinkedLines []string `json:"linkedLines"`
}

type calibrationBandWire struct {
	From              int  `json:"from"`
	To                int  `json:"to"`
	CorrectionPercent *int `json:"correctionPercent"`
}

type categorizationWire struct {
	Threshold          int                   `json:"threshold"`
	MinExamples        int                   `json:"minExamples"`
	Calibration        []calibrationBandWire `json:"calibration"`
	SuggestedThreshold *int                  `json:"suggestedThreshold"`
}

type cardClosingWire struct {
	Mode      string `json:"mode"`
	BankDay   *int   `json:"bankDay"`
	ManualDay int    `json:"manualDay"`
}

type connectionWire struct {
	Bank   string `json:"bank"`
	Status string `json:"status"`
}

type momentWire struct {
	Day  string `json:"day"`
	Time string `json:"time"`
}

type dataSettingsWire struct {
	Connections []connectionWire `json:"connections"`
	LastSync    *momentWire      `json:"lastSync"`
	LastBackup  *momentWire      `json:"lastBackup"`
}

type settingsOverviewWire struct {
	Pots           []settingsPotWire  `json:"pots"`
	Establishments []any              `json:"establishments"`
	People         []any              `json:"people"`
	CardTitle      *string            `json:"cardTitle"`
	CardLabel      string             `json:"cardLabel"`
	Categorization categorizationWire `json:"categorization"`
	Closing        cardClosingWire    `json:"closing"`
	Data           dataSettingsWire   `json:"data"`
}

func registerSettings(registry *Registry) {
	registry.Register("settings.overview", readSettingsOverview)
}

// readSettingsOverview asks for no yield: the settings show none, and the
// statement behind it is the slow part of reading the pots.
func readSettingsOverview(ctx context.Context, _ []json.RawMessage, views Views) (any, error) {
	// The bank's day is shown beside the person's, so the card is read without
	// the person's closing day: a manual day must not pass for the bank's.
	card, err := loadCard(ctx, views.Ledger)
	if err != nil {
		return nil, err
	}
	pots, err := loadPots(ctx, views.Ledger, false)
	if err != nil {
		return nil, err
	}
	state, err := views.Ledger.SyncState(ctx)
	if err != nil {
		return nil, err
	}
	lastBackup, err := lastBackupOf(ctx, views)
	if err != nil {
		return nil, err
	}
	chosen, err := chosenSettingsOf(ctx, views.Authored)
	if err != nil {
		return nil, err
	}
	return settingsOverviewWire{
		Pots:           settingsPotsOf(pots),
		Establishments: []any{},
		People:         []any{},
		CardTitle:      cardTitleOf(card),
		CardLabel:      chosen.cardLabel,
		Categorization: categorizationOf(chosen.threshold, chosen.minExamples),
		Closing:        cardClosingWire{Mode: string(chosen.closingMode), BankDay: bankClosingDayOf(card), ManualDay: int(chosen.manualDay)},
		Data:           dataSettingsOf(state, lastBackup, nowOf(ctx), pots.items),
	}, nil
}

// chosenSettings is what the person set, and the store's default for what they
// did not.
type chosenSettings struct {
	threshold, minExamples, manualDay int64
	cardLabel                         string
	closingMode                       authored.ClosingMode
}

func chosenSettingsOf(ctx context.Context, view *authored.View) (chosenSettings, error) {
	var chosen chosenSettings
	var err error
	if chosen.threshold, err = view.ConfidenceThreshold(ctx); err != nil {
		return chosenSettings{}, err
	}
	if chosen.minExamples, err = view.MinExamples(ctx); err != nil {
		return chosenSettings{}, err
	}
	if chosen.cardLabel, err = view.CardLabel(ctx); err != nil {
		return chosenSettings{}, err
	}
	if chosen.closingMode, err = view.CardClosingMode(ctx); err != nil {
		return chosenSettings{}, err
	}
	if chosen.manualDay, err = view.ManualClosingDay(ctx); err != nil {
		return chosenSettings{}, err
	}
	return chosen, nil
}

// lastBackupOf is nil without the app's database and before its first backup.
func lastBackupOf(ctx context.Context, views Views) (*time.Time, error) {
	instant, found, err := views.Authored.LastBackup(ctx)
	if err != nil || !found {
		return nil, err
	}
	return &instant, nil
}

// settingsPotsOf lists the pots still open, in the order and the bucket the pots
// screen gives them.
func settingsPotsOf(view *potsView) []settingsPotWire {
	pots := []settingsPotWire{}
	for _, entry := range view.entries {
		if entry.closed {
			continue
		}
		pots = append(pots, settingsPotWire{
			ID: entry.id, Name: entry.name, Bank: entry.bank, CDIPercent: entry.cdiPercent,
			Bucket: textPointer(entry.bucket), LinkedLines: []string{},
		})
	}
	return pots
}

func cardTitleOf(card *cardView) *string {
	if card == nil {
		return nil
	}
	return textPointer(card.title())
}

// bankClosingDayOf is the day of the month the bank closes the open bill on,
// and nil when there is no card or the ledger and the bank's rule cannot say.
func bankClosingDayOf(card *cardView) *int {
	if card == nil {
		return nil
	}
	open := card.openBillOf()
	if !open.known {
		return nil
	}
	day, ok := dayOfMonth(open.closing)
	if !ok {
		return nil
	}
	return &day
}

func dayOfMonth(day brday.Day) (int, bool) {
	if len(day) != len("2006-01-02") {
		return 0, false
	}
	value := int(day[8]-'0')*10 + int(day[9]-'0')
	return value, value >= 1 && value <= 31
}

// categorizationOf has no classification behind its calibration, whatever the
// person set: nothing classifies with a confidence yet, so every band is
// unjudged and nothing is suggested.
func categorizationOf(threshold, minExamples int64) categorizationWire {
	bands := make([]calibrationBandWire, 0, calibrationBandCount)
	for index := range calibrationBandCount {
		from := calibrationFirstBand + index*calibrationBandWidth
		to := from + calibrationBandWidth - 1
		if index == calibrationBandCount-1 {
			to = calibrationTop
		}
		bands = append(bands, calibrationBandWire{From: from, To: to})
	}
	return categorizationWire{Threshold: int(threshold), MinExamples: int(minExamples), Calibration: bands}
}
