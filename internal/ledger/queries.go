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

// The README's "caixinha_yield" (pluggy-sync README, section "The yield of each
// caixinha between two days", lines 339-555 at the time of writing). Its `reason`
// column says when a reserve's yield cannot be computed, and the reader passes
// it through.
//
// The README runs it as one statement: these shared CTEs and three SELECTs joined
// by UNION ALL, then ORDER BY reserve_id, reserve_name. The SQLite this module links (modernc 1.62,
// SQLite 3.54) refuses that whole statement with "VIEWs and/or subqueries nested
// too deep", which the SQLite of the collector's own tests does not, so each SELECT
// runs as its own statement over the same CTEs and the rows are concatenated.
// The text of every CTE and every SELECT is the README's, character for character.
const potYieldCTEs = `
WITH
cutoffs AS (
  SELECT a.observation_id, a.day, a.run_id, a.provider_updated_at
  FROM account_balance_history a
  JOIN daily_sightings s ON s.observation_id = a.observation_id
  JOIN versions v ON v.sha256 = s.sha256
  WHERE a.account_id = :account_id AND s.ambiguous = 0
    AND json_type(v.body, '$.bankData.reservedBalances') = 'array'
),
prev AS (SELECT * FROM cutoffs WHERE day <= :d1 ORDER BY day DESC LIMIT 1),
curr AS (SELECT * FROM cutoffs WHERE day <= :d2 ORDER BY day DESC LIMIT 1),
span AS (
  SELECT p.observation_id AS prev_obs, c.observation_id AS curr_obs, c.run_id AS curr_run,
         p.provider_updated_at AS start_at, c.provider_updated_at AS end_at,
         p.day AS prev_day, c.day AS curr_day
  FROM prev p, curr c
  WHERE datetime(p.provider_updated_at) IS NOT NULL
    AND datetime(c.provider_updated_at) IS NOT NULL
    AND datetime(p.provider_updated_at) < datetime(c.provider_updated_at)
),
span_reserves AS (
  SELECT r.*
  FROM reserved_balance_history r
  JOIN cutoffs o ON o.observation_id = r.observation_id, span
  WHERE o.day BETWEEN span.prev_day AND span.curr_day
),
name_ids AS (
  SELECT reserve_name, count(DISTINCT reserve_id) AS ids, min(reserve_id) AS one_id,
         max(CASE WHEN stable_identity = 0 THEN 1 ELSE 0 END) AS unstable_named
  FROM span_reserves WHERE reserve_name IS NOT NULL GROUP BY reserve_name
),
id_names AS (
  SELECT reserve_id, count(DISTINCT reserve_name) AS names
  FROM span_reserves WHERE stable_identity = 1 GROUP BY reserve_id
),
account_reason AS (
  SELECT CASE WHEN NOT EXISTS (
    SELECT 1 FROM scopes s, span
    WHERE s.run_id = span.curr_run AND s.kind = 'transactions' AND s.parent = :account_id
      AND s.outcome = 'complete' AND s.infer_absence = 1 AND s.min_instant IS NOT NULL
      AND datetime(s.min_instant) <= datetime(span.start_at)
  ) THEN 'coverage gap' END AS reason
),
latest_tx AS (
  SELECT o.document_id, o.run_id, d.provider_id, v.body
  FROM observations o
  JOIN documents d ON d.id = o.document_id AND d.kind = 'transactions'
  JOIN versions v ON v.sha256 = o.sha256
  WHERE o.parent = :account_id AND o.observed_day <= :d2
    AND o.id = (SELECT max(x.id) FROM observations x
                WHERE x.document_id = o.document_id AND x.observed_day <= :d2
                  AND NOT EXISTS (SELECT 1 FROM ambiguities am
                                  WHERE am.run_id = x.run_id AND am.document_id = x.document_id))
),
transfers AS (
  SELECT t.provider_id, t.document_id,
         json_extract(t.body, '$.date') AS date,
         json_extract(t.body, '$.amount') AS amount,
         json_extract(t.body, '$.currencyCode') AS currency,
         json_extract(t.body, '$.description') AS description,
         json_extract(t.body, '$.status') AS status,
         EXISTS (SELECT 1 FROM events e
                 WHERE e.document_id = t.document_id AND e.run_id > t.run_id
                   AND e.event IN ('removed', 'parent_gone') AND e.effective_day <= :d2) AS gone
  FROM latest_tx t, span
  WHERE json_extract(t.body, '$.operationType') = 'TRANSFERENCIA_SALDO_RESERVADO'
    AND datetime(json_extract(t.body, '$.date')) > datetime(span.start_at)
    AND datetime(json_extract(t.body, '$.date')) <= datetime(span.end_at)
),
judged AS (
  SELECT tr.*,
    CASE WHEN description LIKE 'Dinheiro reservado %' THEN substr(description, 20)
         WHEN description LIKE 'Dinheiro retirado %' THEN substr(description, 19) END AS reserve_name,
    CASE
      WHEN gone THEN 'transfer removed'
      WHEN status IS NOT 'POSTED' THEN 'transfer pending'
      WHEN provider_id IN (SELECT pending_id FROM pending_posted_candidates
                           UNION SELECT posted_id FROM pending_posted_candidates)
        THEN 'transfer unresolved'
      WHEN description NOT LIKE 'Dinheiro reservado %' AND description NOT LIKE 'Dinheiro retirado %'
        THEN 'unmatched transfer'
      WHEN (description LIKE 'Dinheiro reservado %' AND NOT amount < 0)
        OR (description LIKE 'Dinheiro retirado %' AND NOT amount > 0) THEN 'transfer sign'
      WHEN currency IS NOT 'BRL' THEN 'currency'
      WHEN typeof(amount) NOT IN ('integer', 'real') OR abs(amount) >= 1e12
        OR abs(amount * 100 - round(amount * 100)) > 1e-7 THEN 'amount'
    END AS transfer_reason
  FROM transfers tr
),
resolved AS (
  SELECT j.provider_id, j.date, j.reserve_name,
    CASE WHEN n.ids = 1 AND n.unstable_named = 0 THEN n.one_id END AS reserve_id,
    CASE
      WHEN j.transfer_reason IS NOT NULL THEN j.transfer_reason
      WHEN n.reserve_name IS NULL THEN 'unmatched transfer'
      WHEN n.ids > 1 OR n.unstable_named = 1 THEN 'name reused'
      WHEN i.names > 1 THEN 'name changed'
    END AS reason,
    CAST(round(j.amount * 100) AS INTEGER) AS cents
  FROM judged j
  LEFT JOIN name_ids n ON n.reserve_name = j.reserve_name
  LEFT JOIN id_names i ON i.reserve_id = n.one_id
),
prev_entries AS (SELECT r.* FROM reserved_balance_history r, span WHERE r.observation_id = span.prev_obs),
curr_entries AS (SELECT r.* FROM reserved_balance_history r, span WHERE r.observation_id = span.curr_obs),
stable AS (
  SELECT reserve_id FROM prev_entries WHERE stable_identity = 1
  UNION SELECT reserve_id FROM curr_entries WHERE stable_identity = 1
),
per_reserve AS (
  SELECT s.reserve_id,
    coalesce(c.reserve_name, p.reserve_name) AS reserve_name,
    p.reserve_id IS NOT NULL AS in_prev, c.reserve_id IS NOT NULL AS in_curr,
    EXISTS (SELECT 1 FROM prev_entries u WHERE u.reserve_id = s.reserve_id AND u.stable_identity = 0)
      OR EXISTS (SELECT 1 FROM curr_entries u WHERE u.reserve_id = s.reserve_id AND u.stable_identity = 0)
      AS unstable_somewhere,
    p.amount AS prev_amount, c.amount AS curr_amount,
    p.amount_status AS prev_status, c.amount_status AS curr_status,
    p.currency AS prev_currency, c.currency AS curr_currency,
    (SELECT sum(cents) FROM resolved r WHERE r.reserve_id = s.reserve_id AND r.reason IS NULL)
      AS transfers_cents,
    (SELECT reason FROM resolved r WHERE r.reserve_id = s.reserve_id AND r.reason IS NOT NULL
     ORDER BY date LIMIT 1) AS transfer_reason
  FROM stable s
  LEFT JOIN prev_entries p ON p.reserve_id = s.reserve_id AND p.stable_identity = 1
  LEFT JOIN curr_entries c ON c.reserve_id = s.reserve_id AND c.stable_identity = 1
),
decided AS (
  SELECT reserve_id, reserve_name,
    CASE WHEN in_prev THEN CAST(round(prev_amount * 100) AS INTEGER) END AS prev_cents,
    CASE WHEN in_curr THEN CAST(round(curr_amount * 100) AS INTEGER) END AS curr_cents,
    transfers_cents,
    CASE
      WHEN (SELECT reason FROM account_reason) IS NOT NULL THEN (SELECT reason FROM account_reason)
      WHEN unstable_somewhere THEN 'unstable'
      WHEN NOT in_curr THEN 'closed or missing at D2'
      WHEN (in_prev AND prev_currency IS NOT 'BRL') OR curr_currency IS NOT 'BRL' THEN 'currency'
      WHEN (in_prev AND (prev_status <> 'ok' OR typeof(prev_amount) NOT IN ('integer', 'real')
                         OR abs(prev_amount) >= 1e12
                         OR abs(prev_amount * 100 - round(prev_amount * 100)) > 1e-7))
        OR curr_status <> 'ok' OR typeof(curr_amount) NOT IN ('integer', 'real')
        OR abs(curr_amount) >= 1e12
        OR abs(curr_amount * 100 - round(curr_amount * 100)) > 1e-7 THEN 'amount'
      ELSE transfer_reason
    END AS reason
  FROM per_reserve
)
`

// queryPotYieldReserves is the first SELECT: one row per stable reserve.
const queryPotYieldReserves = potYieldCTEs + `SELECT reserve_id, reserve_name, prev_cents, curr_cents, transfers_cents,
       CASE WHEN reason IS NULL
            THEN (curr_cents - coalesce(prev_cents, 0)) + coalesce(transfers_cents, 0) END AS yield_cents,
       reason
FROM decided`

// queryPotYieldUnstable is the second: one row per entry whose identity is not stable.
const queryPotYieldUnstable = potYieldCTEs + `SELECT reserve_id, reserve_name, prev_cents, curr_cents, NULL, NULL, 'unstable'
FROM (
  SELECT reserve_id, reserve_name, CAST(round(amount * 100) AS INTEGER) AS prev_cents,
         NULL AS curr_cents FROM prev_entries WHERE stable_identity = 0
  UNION ALL
  SELECT reserve_id, reserve_name, NULL, CAST(round(amount * 100) AS INTEGER)
  FROM curr_entries WHERE stable_identity = 0
)`

// queryPotYieldTransfers is the third: one row per transfer that names no stable reserve.
const queryPotYieldTransfers = potYieldCTEs + `SELECT NULL, reserve_name, NULL, NULL, cents, NULL, reason
FROM resolved WHERE reserve_id IS NULL`

// The statements below give the yield of every pair of sightings without the
// collector's one above, whose cost is the same however narrow the window. They
// read what that statement reads (its CTE names are in the comments), once for
// the whole ledger, and leave the arithmetic to internal/potrules. A time is
// handed over as datetime() of it, as the statement compares it, so a
// sub-second difference and an offset weigh exactly as they do there.

// queryYieldSightings is "cutoffs": the account's unambiguous sightings whose
// reservedBalances is an array.
const queryYieldSightings = `
SELECT a.account_id, a.day, a.run_id, datetime(a.provider_updated_at)
FROM account_balance_history a
JOIN observations o ON o.id = a.observation_id
JOIN versions v ON v.sha256 = o.sha256
WHERE json_type(v.body, '$.bankData.reservedBalances') = 'array'
  AND NOT EXISTS (SELECT 1 FROM ambiguities am
                  WHERE am.run_id = o.run_id AND am.document_id = o.document_id)
ORDER BY a.account_id, a.day`

// queryYieldEntries is the reserve entries of every sighting, with the checks
// "decided" makes on an amount done here, in the same SQL, so that a value the
// statement would call unreadable is one here too.
const queryYieldEntries = `
SELECT account_id, day, reserve_id, reserve_name, stable_identity, currency IS 'BRL',
       amount_status = 'ok' AND typeof(amount) IN ('integer', 'real') AND abs(amount) < 1e12
         AND abs(amount * 100 - round(amount * 100)) <= 1e-7,
       CAST(round(amount * 100) AS INTEGER)
FROM reserved_balance_history
ORDER BY account_id, day, position`

// queryYieldTransfers is every unambiguous version of every document that was
// ever a reserve transfer ("latest_tx", "transfers" and "judged" without the
// parts that depend on the window), so the version a window sees is chosen from
// them. fault is the first of "judged"'s reasons that depends on the row alone.
const queryYieldTransfers = `
SELECT document_id, provider_id, id, run_id, parent, observed_day,
       operation_type IS 'TRANSFERENCIA_SALDO_RESERVADO', date, datetime(date), status IS NOT 'POSTED',
       CASE WHEN description LIKE 'Dinheiro reservado %' THEN substr(description, 20)
            WHEN description LIKE 'Dinheiro retirado %' THEN substr(description, 19) END,
       CASE
         WHEN description NOT LIKE 'Dinheiro reservado %' AND description NOT LIKE 'Dinheiro retirado %'
           THEN 'unmatched transfer'
         WHEN (description LIKE 'Dinheiro reservado %' AND NOT amount < 0)
           OR (description LIKE 'Dinheiro retirado %' AND NOT amount > 0) THEN 'transfer sign'
         WHEN currency IS NOT 'BRL' THEN 'currency'
         WHEN typeof(amount) NOT IN ('integer', 'real') OR abs(amount) >= 1e12
           OR abs(amount * 100 - round(amount * 100)) > 1e-7 THEN 'amount'
       END,
       CAST(round(amount * 100) AS INTEGER)
FROM (
  SELECT o.document_id, d.provider_id, o.id, o.run_id, o.parent, o.observed_day,
         json_extract(v.body, '$.operationType') AS operation_type,
         json_extract(v.body, '$.date') AS date,
         json_extract(v.body, '$.amount') AS amount,
         json_extract(v.body, '$.currencyCode') AS currency,
         json_extract(v.body, '$.description') AS description,
         json_extract(v.body, '$.status') AS status
  FROM observations o
  JOIN documents d ON d.id = o.document_id AND d.kind = 'transactions'
  JOIN versions v ON v.sha256 = o.sha256
  WHERE o.document_id IN (` + reserveTransferDocuments + `)
    AND NOT EXISTS (SELECT 1 FROM ambiguities am
                    WHERE am.run_id = o.run_id AND am.document_id = o.document_id)
)
ORDER BY document_id, id`

const reserveTransferDocuments = `
  SELECT o.document_id FROM observations o
  JOIN documents d ON d.id = o.document_id AND d.kind = 'transactions'
  JOIN versions v ON v.sha256 = o.sha256
  WHERE json_extract(v.body, '$.operationType') = 'TRANSFERENCIA_SALDO_RESERVADO'`

// queryYieldRemovals is the events "transfers" counts as gone.
const queryYieldRemovals = `
SELECT document_id, run_id, effective_day FROM events
WHERE event IN ('removed', 'parent_gone') AND document_id IN (` + reserveTransferDocuments + `)
ORDER BY document_id, run_id`

// queryYieldWalks is "account_reason"'s scopes: the walks that went back far
// enough to be trusted to hold every transfer, by the run they belong to.
const queryYieldWalks = `
SELECT parent, run_id, datetime(min_instant) FROM scopes
WHERE kind = 'transactions' AND outcome = 'complete' AND infer_absence = 1 AND min_instant IS NOT NULL
ORDER BY parent, run_id`

// queryYieldUnresolved is the ids "judged" calls unresolved.
const queryYieldUnresolved = `
SELECT pending_id FROM pending_posted_candidates
UNION SELECT posted_id FROM pending_posted_candidates`

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

// queryBalanceHistory is one row per account per day it was sighted, the same
// sightings the pot and yield statements above start from.
const queryBalanceHistory = `
SELECT account_id, day, balance
FROM account_balance_history
ORDER BY account_id, day`

// queryReserveHistory is the README's reserve history over every day, without the
// account's latest-sighting cut that queryPotsAsOf applies.
const queryReserveHistory = `
SELECT account_id, day, position, reserve_id, reserve_name, stable_identity,
       currency, amount, cdi_percentage, amount_status
FROM reserved_balance_history
ORDER BY account_id, day, position`

// queryInvestmentPrincipals reads amountOriginal from the investment's own body.
const queryInvestmentPrincipals = `
SELECT id, json_extract(raw, '$.amountOriginal')
FROM investments
WHERE ` + currentRows + `
ORDER BY id`

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

// queryProjectedRun is the run whose observations the current-state tables
// hold; neither the latest run nor the latest complete one is always it.
const queryProjectedRun = `
SELECT projected_run FROM projection_state WHERE id = 1`

// allQueries lists every statement above by name so the schema test can prepare
// them all; a test fails if a query constant is missing from it.
var allQueries = map[string]string{
	"queryAccounts":             queryAccounts,
	"queryCleanTransactions":    queryCleanTransactions,
	"queryBills":                queryBills,
	"queryPotsAsOf":             queryPotsAsOf,
	"queryPotYieldReserves":     queryPotYieldReserves,
	"queryPotYieldUnstable":     queryPotYieldUnstable,
	"queryPotYieldTransfers":    queryPotYieldTransfers,
	"queryYieldSightings":       queryYieldSightings,
	"queryYieldEntries":         queryYieldEntries,
	"queryYieldTransfers":       queryYieldTransfers,
	"queryYieldRemovals":        queryYieldRemovals,
	"queryYieldWalks":           queryYieldWalks,
	"queryYieldUnresolved":      queryYieldUnresolved,
	"queryInvestments":          queryInvestments,
	"queryInvestmentHistory":    queryInvestmentHistory,
	"queryBalanceHistory":       queryBalanceHistory,
	"queryReserveHistory":       queryReserveHistory,
	"queryInvestmentPrincipals": queryInvestmentPrincipals,
	"queryIdentities":           queryIdentities,
	"queryConnections":          queryConnections,
	"queryCompleteRunFinishes":  queryCompleteRunFinishes,
	"queryLatestRun":            queryLatestRun,
	"queryProjectedRun":         queryProjectedRun,
}
