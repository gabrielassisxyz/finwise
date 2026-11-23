package authored

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"slices"
)

// Shape says what one effect on a journaled table is about.
type Shape int

const (
	// ShapeAttribute: one row of independent facts per key; an effect is one
	// column changing.
	ShapeAttribute Shape = iota + 1
	// ShapeSet: rows that are facts by existing; an effect is one row appearing,
	// disappearing or being replaced at its key.
	ShapeSet
)

// Kind says which of two kinds an attribute table is. They differ at one
// point: whether a change can own a row.
type Kind int

const (
	// KindOptionalFacts: every column has a default and a row at its defaults
	// is the same as no row, so no change owns a row and the first fact written
	// about a key creates its row without owning it.
	KindOptionalFacts Kind = iota + 1
	// KindCreatedWhole: some columns are required and creating a row is a unit
	// of the change that created it, so a later change to any column of the row
	// makes that creation stale.
	KindCreatedWhole
)

// Column is one column of a journaled table.
type Column struct {
	Name string
	// Default is what the column holds when its row is missing, in a table of
	// optional facts only. The recorder writes every column explicitly, so this
	// and not the schema's DEFAULT is what a missing row reads as.
	Default Value
}

// Ownership says that a table's rows belong to rows of another, without
// being part of them.
type Ownership struct {
	// Owner is a set table of the same registry.
	Owner string
	// Columns are this table's columns that hold the owner's key, in the
	// owner's key order.
	Columns []string
	// OnUndo is what an undo that removes the owner's row may do with an owned
	// row it does not itself restore. Every owned table declares one.
	OnUndo OwnedRows
}

// OwnedRows says whether an owned table's rows mean anything without their
// owner, which decides whether an undo may take along one that another change
// wrote.
type OwnedRows int

const (
	// RowsFollowTheirOwner: a row has no meaning without its owner, so an undo
	// that removes the owner takes it along, whichever change wrote it.
	RowsFollowTheirOwner OwnedRows = iota + 1
	// RowsHoldTheirOwner: a row is the person's data in its own right, so an
	// undo that would remove the owner from under one it does not restore is
	// refused as stale.
	RowsHoldTheirOwner
)

// TableStatements are the registered constants that read and write a table;
// no SQL is assembled from names at run time. Every SELECT returns the
// table's columns in Columns order.
type TableStatements struct {
	// SelectRow takes the key in Key order and returns at most one row.
	SelectRow string
	// SelectAll returns every row; the dump sorts them itself.
	SelectAll string
	// Insert takes every column in Columns order.
	Insert string
	// Update takes every column that is not in Key, in Columns order, then the
	// key in Key order. A set table whose columns are all key has none.
	Update string
	// Delete takes the key in Key order.
	Delete string
	// SelectOwned, for an owned table, takes the owner's key in the owner's key
	// order and returns the rows it owns.
	SelectOwned string
}

// Table is one journaled table.
type Table struct {
	Name    string
	Shape   Shape
	Kind    Kind
	Columns []Column
	// Key names the key columns in key order.
	Key        []string
	OwnedBy    *Ownership
	Statements TableStatements
}

// RestoreCheck looks at the database inside an undo's transaction, after the
// restore and before the commit, and says whether the restored state is one
// the app allows. It must only read. restored are the units the undo put back.
type RestoreCheck func(ctx context.Context, tx *sql.Tx, restored []Unit) (allowed bool, err error)

// Registry is the closed list of journaled tables and the checks every undo
// runs. A table that is not in it cannot be written through the recorder.
type Registry struct {
	tables map[string]*Table
	owned  map[string][]*Table
	checks []RestoreCheck
}

// productionTables is the package's own registry. Each authored table arrives
// with its migration and adds its entry here, naming registered constants.
var productionTables = []Table{
	budgetVersionsTable,
	budgetVersionLinesTable,
	scenariosTable,
	closedMonthsTable,
	settingsEntry(),
	txStateEntry,
	txTagsEntry,
	txSplitPartsEntry,
	establishmentsEntry,
	establishmentTagsEntry,
	historyStepsEntry,
	historyReasonsEntry,
	potSettingsEntry(),
	potLinesEntry(),
	subscriptionsEntry(),
	closingStepsEntry,
	transferMarksEntry,
	dismissedWarningsEntry,
}

// notJournaledTables are the tables of finwise.db deliberately kept out of the
// journal. A write to one happens in the transaction of the change it
// accompanies and is not an effect of it: undoing the change leaves it.
var notJournaledTables = map[string]string{
	"changes":           "the journal itself",
	"change_effects":    "the journal itself",
	"schema_migrations": "the migrations runner's record",
	"meta":              "values of the app itself, not authored",
	"tx_index":          "what the database has observed of the ledger's transactions, not a decision",
	"tx_ids":            "what the database has observed of the ledger's transactions, not a decision",
	"lines":             "a budget line's id, once minted, exists for good",
	"pin":               "changing the PIN is not a recorded change and has no undo",
}

// productionChecks are the checks every undo of the app runs.
var productionChecks = []RestoreCheck{noStrayLineReferences, closedMonthsKeepTheirBudget}

// AppTables is a copy of the app's own journaled tables, for a registry that
// adds tables of its own to them.
func AppTables() []Table { return slices.Clone(productionTables) }

// AppChecks is a copy of the checks every undo of the app runs, for a registry
// that journals the app's tables beside its own: an undo over them must refuse
// what the app's own refuses.
func AppChecks() []RestoreCheck { return slices.Clone(productionChecks) }

func productionRegistry() (*Registry, error) {
	return NewRegistry(productionTables, productionChecks...)
}

// NewRegistry validates tables and returns the registry they form.
func NewRegistry(tables []Table, checks ...RestoreCheck) (*Registry, error) {
	registry := &Registry{tables: map[string]*Table{}, owned: map[string][]*Table{}, checks: checks}
	for i := range tables {
		table := tables[i]
		if err := validateTable(&table); err != nil {
			return nil, err
		}
		if _, taken := registry.tables[table.Name]; taken {
			return nil, fmt.Errorf("authored: %s is registered twice", table.Name)
		}
		registry.tables[table.Name] = &table
	}
	for _, table := range registry.tables {
		if table.OwnedBy == nil {
			continue
		}
		if err := validateOwnership(registry, table); err != nil {
			return nil, err
		}
		registry.owned[table.OwnedBy.Owner] = append(registry.owned[table.OwnedBy.Owner], table)
	}
	for owner := range registry.owned {
		slices.SortFunc(registry.owned[owner], func(a, b *Table) int { return compareText(a.Name, b.Name) })
	}
	return registry, nil
}

func validateTable(table *Table) error {
	if table.Name == "" {
		return errors.New("authored: a journaled table has no name")
	}
	if _, excluded := notJournaledTables[table.Name]; excluded {
		return fmt.Errorf("authored: %s is deliberately not journaled", table.Name)
	}
	if err := validateShape(table); err != nil {
		return err
	}
	if err := validateColumns(table); err != nil {
		return err
	}
	return validateStatements(table)
}

func validateShape(table *Table) error {
	switch {
	case table.Shape == ShapeAttribute && (table.Kind == KindOptionalFacts || table.Kind == KindCreatedWhole):
		return nil
	case table.Shape == ShapeSet && table.Kind == 0:
		return nil
	}
	return fmt.Errorf("authored: %s has no valid shape and kind", table.Name)
}

func validateColumns(table *Table) error {
	names := map[string]bool{}
	for _, column := range table.Columns {
		if column.Name == "" || names[column.Name] {
			return fmt.Errorf("authored: %s has an empty or repeated column", table.Name)
		}
		names[column.Name] = true
	}
	if len(table.Key) == 0 {
		return fmt.Errorf("authored: %s has no key", table.Name)
	}
	keys := map[string]bool{}
	for _, key := range table.Key {
		if !names[key] || keys[key] {
			return fmt.Errorf("authored: %s has a key column that is not one of its columns, or is repeated", table.Name)
		}
		keys[key] = true
	}
	if table.Shape == ShapeAttribute && len(table.Key) == len(table.Columns) {
		return fmt.Errorf("authored: %s holds no fact besides its key", table.Name)
	}
	return nil
}

func validateStatements(table *Table) error {
	statements := table.Statements
	if statements.SelectRow == "" || statements.SelectAll == "" || statements.Insert == "" || statements.Delete == "" {
		return fmt.Errorf("authored: %s lacks a statement", table.Name)
	}
	if statements.Update == "" && len(table.Key) < len(table.Columns) {
		return fmt.Errorf("authored: %s has columns besides its key and no update", table.Name)
	}
	if (statements.SelectOwned == "") != (table.OwnedBy == nil) {
		return fmt.Errorf("authored: %s names a statement for owned rows exactly when it is owned", table.Name)
	}
	return nil
}

// An owner and the rows it owns are both set tables, and one level deep: the
// write side needs a history step's reason and nothing more.
func validateOwnership(registry *Registry, table *Table) error {
	owner, ok := registry.tables[table.OwnedBy.Owner]
	if !ok || owner.Shape != ShapeSet || owner.OwnedBy != nil || table.Shape != ShapeSet {
		return fmt.Errorf("authored: %s is owned by %s, which is not an unowned set table of the registry", table.Name, table.OwnedBy.Owner)
	}
	if len(table.OwnedBy.Columns) != len(owner.Key) {
		return fmt.Errorf("authored: %s does not name a column for every key column of %s", table.Name, owner.Name)
	}
	for _, name := range table.OwnedBy.Columns {
		if table.column(name) == nil {
			return fmt.Errorf("authored: %s names an owner column it does not have", table.Name)
		}
	}
	if onUndo := table.OwnedBy.OnUndo; onUndo != RowsFollowTheirOwner && onUndo != RowsHoldTheirOwner {
		return fmt.Errorf("authored: %s declares neither that its rows follow %s nor that they hold it", table.Name, owner.Name)
	}
	return nil
}

func (registry *Registry) table(name string) (*Table, error) {
	table, ok := registry.tables[name]
	if !ok {
		return nil, &NotJournaledError{Table: name}
	}
	return table, nil
}

// names is every registered table, sorted.
func (registry *Registry) names() []string {
	names := make([]string, 0, len(registry.tables))
	for name := range registry.tables {
		names = append(names, name)
	}
	slices.Sort(names)
	return names
}

// NotJournaledError is a recorded write to a table the registry does not name.
// Nothing was written.
type NotJournaledError struct {
	Table string
}

func (err *NotJournaledError) Error() string {
	return fmt.Sprintf("authored: %s is not a journaled table, so the recorder does not write it", err.Table)
}

func (table *Table) column(name string) *Column {
	for i := range table.Columns {
		if table.Columns[i].Name == name {
			return &table.Columns[i]
		}
	}
	return nil
}

func (table *Table) isKey(name string) bool {
	return slices.Contains(table.Key, name)
}

func (table *Table) createdWhole() bool {
	return table.Shape == ShapeAttribute && table.Kind == KindCreatedWhole
}

func (table *Table) optionalFacts() bool {
	return table.Shape == ShapeAttribute && table.Kind == KindOptionalFacts
}

// defaults is the row a missing key of a table of optional facts reads as.
func (table *Table) defaults(key []Value) Row {
	row := Row{}
	for _, column := range table.Columns {
		row[column.Name] = column.Default
	}
	for i, name := range table.Key {
		row[name] = key[i]
	}
	return row
}

// atDefaults says whether a row of a table of optional facts is the same as
// no row.
func (table *Table) atDefaults(row Row) bool {
	for _, column := range table.Columns {
		if !table.isKey(column.Name) && row[column.Name] != column.Default {
			return false
		}
	}
	return true
}

func (table *Table) keyOf(row Row) []Value {
	key := make([]Value, len(table.Key))
	for i, name := range table.Key {
		key[i] = row[name]
	}
	return key
}

// verify runs a table's SELECTs and compares the columns they return with the
// entry's, so a statement that returns them in another order, which would
// put one column's value into another, is caught before anything is written.
func (registry *Registry) verify(ctx context.Context, tx *sql.Tx) error {
	for _, name := range registry.names() {
		table := registry.tables[name]
		want := make([]string, len(table.Columns))
		for i, column := range table.Columns {
			want[i] = column.Name
		}
		statements := table.Statements
		rows, err := tx.QueryContext(ctx, statements.SelectAll)
		if err := verifyColumns(rows, err, want); err != nil {
			return fmt.Errorf("authored: %s: %w", name, err)
		}
		rows, err = tx.QueryContext(ctx, statements.SelectRow, make([]any, len(table.Key))...)
		if err := verifyColumns(rows, err, want); err != nil {
			return fmt.Errorf("authored: %s: %w", name, err)
		}
		if table.OwnedBy == nil {
			continue
		}
		rows, err = tx.QueryContext(ctx, statements.SelectOwned, make([]any, len(table.OwnedBy.Columns))...)
		if err := verifyColumns(rows, err, want); err != nil {
			return fmt.Errorf("authored: %s: %w", name, err)
		}
	}
	return nil
}

func verifyColumns(rows *sql.Rows, err error, want []string) error {
	if err != nil {
		return err
	}
	defer rows.Close()
	got, err := rows.Columns()
	if err != nil {
		return err
	}
	if !slices.Equal(got, want) {
		return fmt.Errorf("a statement returns %v, the entry names %v", got, want)
	}
	return nil
}

func compareText(a, b string) int {
	switch {
	case a < b:
		return -1
	case a > b:
		return 1
	}
	return 0
}
