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

type modelRunWire struct {
	Day    string `json:"day"`
	Time   string `json:"time"`
	Worked bool   `json:"worked"`
}

type modelWire struct {
	Name    string        `json:"name"`
	LastRun *modelRunWire `json:"lastRun"`
}

type categorizationWire struct {
	Threshold          int                   `json:"threshold"`
	MinExamples        int                   `json:"minExamples"`
	Calibration        []calibrationBandWire `json:"calibration"`
	SuggestedThreshold *int                  `json:"suggestedThreshold"`
	Model              *modelWire            `json:"model"`
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
	Pots           []settingsPotWire           `json:"pots"`
	Establishments []settingsEstablishmentWire `json:"establishments"`
	People         []settingsPersonWire        `json:"people"`
	CardTitle      *string                     `json:"cardTitle"`
	CardLabel      string                      `json:"cardLabel"`
	Categorization categorizationWire          `json:"categorization"`
	Closing        cardClosingWire             `json:"closing"`
	Data           dataSettingsWire            `json:"data"`
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
	pots, err := loadPotsWithChoices(ctx, views, false)
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
	counterparts, err := settingsCounterpartsOf(ctx, views)
	if err != nil {
		return nil, err
	}
	categorization, err := categorizationOf(ctx, views, chosen)
	if err != nil {
		return nil, err
	}
	return settingsOverviewWire{
		Pots:           settingsPotsOf(pots),
		Establishments: counterparts.establishments,
		People:         counterparts.people,
		CardTitle:      cardTitleOf(card),
		CardLabel:      chosen.cardLabel,
		Categorization: categorization,
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
// screen gives them, each with the names of the lines linked to it.
func settingsPotsOf(view *potsView) []settingsPotWire {
	pots := []settingsPotWire{}
	for _, entry := range view.entries {
		if entry.closed {
			continue
		}
		pots = append(pots, settingsPotWire{
			ID: entry.id, Name: entry.name, Bank: entry.bank, CDIPercent: entry.cdiPercent,
			Bucket: textPointer(entry.bucket), LinkedLines: append([]string{}, entry.linkedLines...),
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

// categorizationOf answers the calibration of the model configured now, and the
// model with the last time it was asked. The threshold that would be suggested
// stays unanswered: it only means something beside a switch that lets the model
// apply a line by itself, and there is none.
func categorizationOf(ctx context.Context, views Views, chosen chosenSettings) (categorizationWire, error) {
	counts, err := views.Authored.ModelAnswerCounts(ctx, views.ModelName, authored.ConfidenceBands{
		First: calibrationFirstBand, Width: calibrationBandWidth, Count: calibrationBandCount,
	})
	if err != nil {
		return categorizationWire{}, err
	}
	model, err := modelOf(ctx, views)
	if err != nil {
		return categorizationWire{}, err
	}
	return categorizationWire{
		Threshold:   int(chosen.threshold),
		MinExamples: int(chosen.minExamples),
		Calibration: calibrationOf(counts),
		Model:       model,
	}, nil
}

// calibrationOf is one band per count: the share the person corrected among
// the answered suggestions, from calibrationMinAnswered of them on and unjudged
// before.
func calibrationOf(counts []authored.AnswerCounts) []calibrationBandWire {
	bands := make([]calibrationBandWire, 0, len(counts))
	for index, counted := range counts {
		from := calibrationFirstBand + index*calibrationBandWidth
		to := from + calibrationBandWidth - 1
		if index == len(counts)-1 {
			to = calibrationTop
		}
		band := calibrationBandWire{From: from, To: to}
		if counted.Answered >= calibrationMinAnswered {
			percent := correctionPercentOf(counted)
			band.CorrectionPercent = &percent
		}
		bands = append(bands, band)
	}
	return bands
}

// correctionPercentOf is corrected over answered, a whole percent with the half
// rounded up, in integers: a float would move a band across the line the
// person reads at 2.5.
func correctionPercentOf(counted authored.AnswerCounts) int {
	return int((200*counted.Corrected + counted.Answered) / (2 * counted.Answered))
}

// modelOf is nil with no model configured, and carries the last run that made
// a request, nil before there was one.
func modelOf(ctx context.Context, views Views) (*modelWire, error) {
	if views.ModelName == "" {
		return nil, nil
	}
	model := &modelWire{Name: views.ModelName}
	run, found, err := views.Authored.LastModelRun(ctx)
	if err != nil || !found {
		return model, err
	}
	at := momentOf(run.At)
	model.LastRun = &modelRunWire{Day: at.Day, Time: at.Time, Worked: run.OK}
	return model, nil
}
