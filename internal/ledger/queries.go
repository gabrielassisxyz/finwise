package ledger

// Every statement the package runs is declared here, so that one test can
// prepare all of them against the collector's real schema. A statement written
// anywhere else escapes that test.

// currentRows is what exists now on every current-state table
// (pluggy-sync README, "The current-state tables", lines 130-137).
const currentRows = "state = 'present' AND ambiguous_run IS NULL"

const queryAccounts = `
SELECT id, item_id, type, subtype, name, balance, currency, raw
FROM accounts
WHERE ` + currentRows + `
ORDER BY id`

// queryCleanTransactions is the README's "clean list" (pluggy-sync README lines
// 282-296) with the columns the reader carries added to its select list.
const queryCleanTransactions = `
SELECT t.id, t.account_id, t.date, t.amount, t.description,
       t.currency, t.type, t.category, t.raw
FROM transactions t
JOIN document_coverage c ON c.kind = 'transactions' AND c.provider_id = t.id
WHERE t.state = 'present' AND t.ambiguous_run IS NULL AND c.covered = 1
  AND NOT (json_extract(t.raw, '$.status') = 'PENDING'
           AND t.id IN (SELECT pending_id FROM pending_posted_candidates))
ORDER BY t.date DESC, t.id`

const queryBills = `
SELECT id, account_id, due_date, closing_date, total_amount, minimum_payment
FROM bills
WHERE ` + currentRows + `
ORDER BY account_id, due_date, id`

// queryPotsAsOf is the README's "reserves_as_of" (pluggy-sync README lines
// 239-258), run once per account.
const queryPotsAsOf = `
WITH latest AS (
  SELECT observation_id, day, provider_updated_at FROM account_balance_history
  WHERE account_id = :account_id AND day <= :day
  ORDER BY day DESC LIMIT 1
)
SELECT l.day, l.provider_updated_at, r.position, r.reserve_id, r.reserve_name,
       r.stable_identity, r.currency, r.amount, r.cdi_percentage, r.amount_status
FROM latest l
LEFT JOIN reserved_balance_history r ON r.observation_id = l.observation_id
ORDER BY r.position`

const queryInvestments = `
SELECT id, item_id, type, subtype, name, balance, currency
FROM investments
WHERE ` + currentRows + `
ORDER BY id`

// queryInvestmentHistory is the README's "cdb_history" (pluggy-sync README
// lines 260-269) without amount_original, which is never summed.
const queryInvestmentHistory = `
SELECT day, provider_updated_at, currency, amount_gross,
       taxes, taxes2, amount_net, rate, rate_type, due_date, status
FROM investment_history
WHERE investment_id = :investment_id
ORDER BY day`

const queryIdentities = `
SELECT id, item_id, document, full_name
FROM identities
WHERE ` + currentRows + `
ORDER BY id`

// queryConnections reads the expiry from the item's raw body because
// consents.expires_at is null on every row the collector writes.
const queryConnections = `
SELECT id, connector, status, execution_status, last_updated, next_auto_sync,
       json_extract(raw, '$.consentExpiresAt')
FROM items
WHERE ` + currentRows + `
ORDER BY id`

const queryCompleteRunFinishes = `
SELECT finished_at FROM runs WHERE outcome = 'complete'`

const queryLatestRun = `
SELECT id, outcome, finished_at FROM runs ORDER BY id DESC LIMIT 1`

// allQueries lists every statement above by name so the schema test can prepare
// them all; a test fails if a query constant is missing from it.
var allQueries = map[string]string{
	"queryAccounts":            queryAccounts,
	"queryCleanTransactions":   queryCleanTransactions,
	"queryBills":               queryBills,
	"queryPotsAsOf":            queryPotsAsOf,
	"queryInvestments":         queryInvestments,
	"queryInvestmentHistory":   queryInvestmentHistory,
	"queryIdentities":          queryIdentities,
	"queryConnections":         queryConnections,
	"queryCompleteRunFinishes": queryCompleteRunFinishes,
	"queryLatestRun":           queryLatestRun,
}
