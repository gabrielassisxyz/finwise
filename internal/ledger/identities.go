package ledger

import (
	"context"
	"database/sql"
	"fmt"
)

// Identity is a holder's identity record as the provider returns it.
type Identity struct {
	ID       string
	ItemID   string
	Document string
	FullName string
}

// Identities returns every current identity, by id.
func (view *View) Identities(ctx context.Context) ([]Identity, error) {
	var identities []Identity
	err := view.read(ctx, func(tx *sql.Tx) error {
		rows, err := tx.QueryContext(ctx, queryIdentities)
		if err != nil {
			return fmt.Errorf("ledger: identities: %w", err)
		}
		defer rows.Close()
		for rows.Next() {
			var identity Identity
			var itemID, document, fullName sql.NullString
			if err := rows.Scan(&identity.ID, &itemID, &document, &fullName); err != nil {
				return fmt.Errorf("ledger: identities: %w", err)
			}
			identity.ItemID, identity.Document, identity.FullName = itemID.String, document.String, fullName.String
			identities = append(identities, identity)
		}
		return rows.Err()
	})
	return identities, err
}
