package authored

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strconv"

	"github.com/gabrielassisxyz/finwise/internal/brday"
	"github.com/gabrielassisxyz/finwise/internal/money"
)

// SubscriptionID is a subscription's identity, written on the wire as a
// decimal string.
type SubscriptionID int64

func (id SubscriptionID) String() string { return strconv.FormatInt(int64(id), 10) }

// ParseSubscriptionID reads the one spelling String writes; "07", "+7",
// "sub-1" and "" are the id of no subscription.
func ParseSubscriptionID(text string) (SubscriptionID, bool) {
	number, err := strconv.ParseInt(text, 10, 64)
	if err != nil || number < 1 {
		return 0, false
	}
	id := SubscriptionID(number)
	return id, id.String() == text
}

// SubscriptionStatus is what the person decided to do about a subscription.
// The four values are the contract's words; the table refuses any other.
type SubscriptionStatus string

const (
	StatusKeep            SubscriptionStatus = "Manter"
	StatusReview          SubscriptionStatus = "Analisar"
	StatusBackUpAndCancel SubscriptionStatus = "Backup e cancelar"
	StatusCancel          SubscriptionStatus = "Cancelar"
)

// SubscriptionStatuses are the four in the contract's order.
var SubscriptionStatuses = []SubscriptionStatus{StatusKeep, StatusReview, StatusBackUpAndCancel, StatusCancel}

// Subscription is one subscription as the app's database keeps it: what the
// person decided and the purchase it was made from, never a figure worked out
// from the charges.
type Subscription struct {
	ID               SubscriptionID
	EstablishmentKey string
	CreatedFrom      TxKey
	CreatedDay       brday.Day
	CreatedCents     money.Cents
	Name             string
	Line             *LineID
	Status           SubscriptionStatus
	StatusDay        brday.Day
	ReviewOn         brday.Day
	Note             string
}

// ErrNoSuchSubscription is a write about a subscription the database does not
// have. Nothing was written.
var ErrNoSuchSubscription = errors.New("authored: no subscription has that id")

const subscriptionsTable = "subscriptions"

const (
	subscriptionColumns = `id, establishment_key, created_from_tx_key, created_day, created_cents, name, line_id, status,
status_day, review_on, note`

	statementSubscriptionRow    = `SELECT ` + subscriptionColumns + ` FROM subscriptions WHERE id = ?`
	statementSubscriptionAll    = `SELECT ` + subscriptionColumns + ` FROM subscriptions`
	statementSubscriptionInsert = `INSERT INTO subscriptions (` + subscriptionColumns + `) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`
	statementSubscriptionUpdate = `
UPDATE subscriptions SET establishment_key = ?, created_from_tx_key = ?, created_day = ?, created_cents = ?, name = ?,
  line_id = ?, status = ?, status_day = ?, review_on = ?, note = ?
WHERE id = ?`
	statementSubscriptionDelete = `DELETE FROM subscriptions WHERE id = ?`

	statementSubscriptionsInOrder        = `SELECT ` + subscriptionColumns + ` FROM subscriptions ORDER BY id`
	statementSubscriptionOfEstablishment = `SELECT id FROM subscriptions WHERE establishment_key = ?`
	// The id a new subscription gets: one past the highest the table ever
	// held, which AUTOINCREMENT keeps in sqlite_sequence, so the id of a
	// creation that was undone is never handed out again.
	statementNextSubscriptionID = `SELECT coalesce((SELECT seq FROM sqlite_sequence WHERE name = 'subscriptions'), 0) + 1`
)

// subscriptionsEntry is a table of things created whole: a subscription has
// required facts, so creating one is a unit of its own, and a later change to
// any of its columns that is still in force makes that creation stale.
func subscriptionsEntry() Table {
	return Table{
		Name: subscriptionsTable, Shape: ShapeAttribute, Kind: KindCreatedWhole, Key: []string{"id"},
		Columns: []Column{
			{Name: "id"}, {Name: "establishment_key"}, {Name: "created_from_tx_key"}, {Name: "created_day"},
			{Name: "created_cents"}, {Name: "name"}, {Name: "line_id"}, {Name: "status"}, {Name: "status_day"},
			{Name: "review_on"}, {Name: "note"},
		},
		Statements: TableStatements{
			SelectRow: statementSubscriptionRow, SelectAll: statementSubscriptionAll,
			Insert: statementSubscriptionInsert, Update: statementSubscriptionUpdate, Delete: statementSubscriptionDelete,
		},
	}
}

// Subscriptions is every subscription in the order they were created; none on
// the empty view.
func (view *View) Subscriptions(ctx context.Context) ([]Subscription, error) {
	var subscriptions []Subscription
	_, err := view.read(ctx, func(tx *sql.Tx) error {
		table, err := view.registry().table(subscriptionsTable)
		if err != nil {
			return err
		}
		rows, err := tx.QueryContext(ctx, statementSubscriptionsInOrder)
		if err != nil {
			return err
		}
		found, err := scanRows(rows, table)
		if err != nil {
			return err
		}
		for _, row := range found {
			subscription, err := subscriptionOf(row)
			if err != nil {
				return err
			}
			subscriptions = append(subscriptions, subscription)
		}
		return nil
	})
	return subscriptions, err
}

// Subscription is the subscription with that id; found is false when there is
// none, and on the empty view.
func (view *View) Subscription(ctx context.Context, id SubscriptionID) (Subscription, bool, error) {
	row, found, err := view.JournaledRow(ctx, subscriptionsTable, []Value{Integer(int64(id))})
	if err != nil || !found {
		return Subscription{}, false, err
	}
	subscription, err := subscriptionOf(row)
	if err != nil {
		return Subscription{}, false, &TransactionError{cause: err}
	}
	return subscription, true, nil
}

// SubscriptionOfEstablishment is the subscription an establishment has;
// found is false when it has none, and on the empty view.
func (view *View) SubscriptionOfEstablishment(ctx context.Context, key string) (id SubscriptionID, found bool, err error) {
	_, err = view.read(ctx, func(tx *sql.Tx) error {
		var stored int64
		scanned := tx.QueryRowContext(ctx, statementSubscriptionOfEstablishment, key).Scan(&stored)
		if errors.Is(scanned, sql.ErrNoRows) {
			return nil
		}
		id, found = SubscriptionID(stored), scanned == nil
		return scanned
	})
	return id, found, err
}

func subscriptionOf(row Row) (Subscription, error) {
	id, idOK := row["id"].Int64()
	key, keyOK := row["establishment_key"].Str()
	from, fromOK := row["created_from_tx_key"].Int64()
	day, dayOK := row["created_day"].Str()
	cents, centsOK := row["created_cents"].Int64()
	name, nameOK := row["name"].Str()
	status, statusOK := row["status"].Str()
	statusDay, statusDayOK := row["status_day"].Str()
	review, reviewOK := row["review_on"].Str()
	note, noteOK := row["note"].Str()
	if !idOK || !keyOK || !fromOK || !dayOK || !centsOK || !nameOK || !statusOK || !statusDayOK || !reviewOK || !noteOK {
		return Subscription{}, fmt.Errorf("authored: a row of %s holds a value of another type than its column's", subscriptionsTable)
	}
	subscription := Subscription{
		ID: SubscriptionID(id), EstablishmentKey: key, CreatedFrom: TxKey(from), CreatedDay: brday.Day(day),
		CreatedCents: money.Cents(cents), Name: name, Status: SubscriptionStatus(status), StatusDay: brday.Day(statusDay),
		ReviewOn: brday.Day(review), Note: note,
	}
	if line, set := row["line_id"].Int64(); set {
		id := LineID(line)
		subscription.Line = &id
	}
	return subscription, nil
}

// CreateSubscription writes a new subscription under an id it mints, which is
// one effect of the change, and answers the id. The ID given is not read. The
// store checks no value: the table refuses an establishment that already has
// a subscription, a status outside the four and a negative amount, and a
// purchase key or a line that does not exist when the change commits.
func (recorder *Recorder) CreateSubscription(ctx context.Context, subscription Subscription) (SubscriptionID, error) {
	if err := recorder.usable(); err != nil {
		return 0, err
	}
	var id int64
	if err := recorder.tx.QueryRowContext(ctx, statementNextSubscriptionID).Scan(&id); err != nil {
		return 0, recorder.fail(err)
	}
	line := Null
	if subscription.Line != nil {
		line = Integer(int64(*subscription.Line))
	}
	row := Row{
		"id": Integer(id), "establishment_key": Text(subscription.EstablishmentKey),
		"created_from_tx_key": Integer(int64(subscription.CreatedFrom)), "created_day": Text(string(subscription.CreatedDay)),
		"created_cents": Integer(int64(subscription.CreatedCents)), "name": Text(subscription.Name), "line_id": line,
		"status": Text(string(subscription.Status)), "status_day": Text(string(subscription.StatusDay)),
		"review_on": Text(string(subscription.ReviewOn)), "note": Text(subscription.Note),
	}
	if err := recorder.Create(ctx, subscriptionsTable, row); err != nil {
		return 0, err
	}
	return SubscriptionID(id), nil
}

// SetSubscriptionStatus records a subscription's status and, in the same
// change, day as the day it was set. The status it already has records
// nothing and leaves its day alone, so the day a charge after cancelling is
// told by is the day the person decided, however often it is set again.
func (recorder *Recorder) SetSubscriptionStatus(ctx context.Context, id SubscriptionID, status SubscriptionStatus, day brday.Day) error {
	current, err := recorder.subscription(ctx, id)
	if err != nil || current.Status == status {
		return err
	}
	return recorder.setSubscription(ctx, id, Row{"status": Text(string(status)), "status_day": Text(string(day))})
}

// SetSubscriptionLine records a subscription's budget line, by its id.
func (recorder *Recorder) SetSubscriptionLine(ctx context.Context, id SubscriptionID, line LineID) error {
	if _, err := recorder.subscription(ctx, id); err != nil {
		return err
	}
	return recorder.setSubscription(ctx, id, Row{"line_id": Integer(int64(line))})
}

// SetSubscriptionReviewDay records the day a subscription is to be reviewed.
func (recorder *Recorder) SetSubscriptionReviewDay(ctx context.Context, id SubscriptionID, day brday.Day) error {
	if _, err := recorder.subscription(ctx, id); err != nil {
		return err
	}
	return recorder.setSubscription(ctx, id, Row{"review_on": Text(string(day))})
}

// SetSubscriptionNote records a subscription's note as it is given.
func (recorder *Recorder) SetSubscriptionNote(ctx context.Context, id SubscriptionID, note string) error {
	if _, err := recorder.subscription(ctx, id); err != nil {
		return err
	}
	return recorder.setSubscription(ctx, id, Row{"note": Text(note)})
}

// subscription is the subscription a write names, or ErrNoSuchSubscription,
// which leaves the change usable so that the caller can answer it.
func (recorder *Recorder) subscription(ctx context.Context, id SubscriptionID) (Subscription, error) {
	view := recorder.View()
	defer view.Close()
	subscription, found, err := view.Subscription(ctx, id)
	switch {
	case err != nil:
		return Subscription{}, recorder.fail(err)
	case !found:
		return Subscription{}, ErrNoSuchSubscription
	}
	return subscription, nil
}

func (recorder *Recorder) setSubscription(ctx context.Context, id SubscriptionID, values Row) error {
	return recorder.Set(ctx, subscriptionsTable, []Value{Integer(int64(id))}, values)
}
