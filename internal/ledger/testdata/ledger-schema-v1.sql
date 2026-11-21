-- The collector's schema at user_version 1, as pluggy-sync's own migration
-- creates it, with no data. Regenerate by applying that migration to an empty
-- database and listing sqlite_master; never edit by hand.
CREATE TABLE runs (
  id           INTEGER PRIMARY KEY,
  started_at   TEXT NOT NULL,
  started_day  TEXT NOT NULL,
  finished_at  TEXT,
  finished_day TEXT,
  outcome      TEXT NOT NULL CHECK (outcome IN
                 ('running','complete','partial','failed','interrupted','migrated')),
  api_calls    INTEGER,
  code_version TEXT NOT NULL,
  item_ids     TEXT NOT NULL CHECK (json_valid(item_ids)),
  note         TEXT,
  CHECK (outcome = 'running' OR (finished_at IS NOT NULL AND finished_day IS NOT NULL))
);

CREATE TABLE scopes (
  id            INTEGER PRIMARY KEY,
  run_id        INTEGER NOT NULL REFERENCES runs(id),
  kind          TEXT NOT NULL CHECK (kind IN ('categories', 'items', 'consents', 'identities', 'investments', 'accounts', 'investment_transactions', 'transactions', 'bills', 'loans')),
  parent        TEXT NOT NULL,
  expected      INTEGER NOT NULL CHECK (expected IN (0,1)),
  outcome       TEXT NOT NULL CHECK (outcome IN
                  ('pending','complete','incomplete','unavailable','skipped')),
  infer_absence INTEGER NOT NULL DEFAULT 0 CHECK (infer_absence IN (0,1)),
  status        INTEGER,
  rows          INTEGER,
  min_date      TEXT,
  min_instant   TEXT,
  closed_at     TEXT,
  closed_day    TEXT,
  error         TEXT,
  UNIQUE (run_id, kind, parent),
  CHECK (infer_absence = 0 OR outcome = 'complete'),
  CHECK ((kind = 'categories') = (parent = '')),
  CHECK (outcome = 'pending'
         OR (closed_at IS NOT NULL AND closed_day IS NOT NULL AND rows IS NOT NULL))
);

CREATE TABLE responses (
  id           INTEGER PRIMARY KEY,
  run_id       INTEGER NOT NULL REFERENCES runs(id),
  scope_id     INTEGER NOT NULL REFERENCES scopes(id),
  method       TEXT NOT NULL,
  url          TEXT NOT NULL,
  status       INTEGER NOT NULL CHECK (status >= 0),
  headers      TEXT CHECK (headers IS NULL OR json_type(headers) = 'array'),
  body_zlib    BLOB,
  body_sha256  TEXT,
  raw_bytes    INTEGER,
  truncated    INTEGER NOT NULL DEFAULT 0 CHECK (truncated IN (0,1)),
  error        TEXT,
  received_at  TEXT NOT NULL,
  received_day TEXT NOT NULL,
  CHECK ((body_zlib IS NULL AND body_sha256 IS NULL AND raw_bytes IS NULL)
      OR (body_zlib IS NOT NULL AND body_sha256 IS NOT NULL
          AND raw_bytes IS NOT NULL AND raw_bytes >= 0)),
  CHECK (truncated = 0 OR error IS NOT NULL),
  CHECK (status > 0 OR error IS NOT NULL),
  CHECK (status > 0 OR body_zlib IS NULL)
);

CREATE INDEX responses_by_run ON responses(run_id, id);

CREATE INDEX responses_by_scope ON responses(scope_id);

CREATE TABLE page_derivations (
  response_id INTEGER PRIMARY KEY REFERENCES responses(id),
  valid       INTEGER NOT NULL CHECK (valid IN (0,1)),
  page_meta   TEXT,
  error       TEXT
);

CREATE TABLE documents (
  id          INTEGER PRIMARY KEY,
  kind        TEXT NOT NULL CHECK (kind IN ('categories', 'items', 'consents', 'identities', 'investments', 'accounts', 'investment_transactions', 'transactions', 'bills', 'loans')),
  provider_id TEXT NOT NULL CHECK (provider_id <> ''),
  UNIQUE (kind, provider_id)
);

CREATE TABLE versions (
  sha256 TEXT PRIMARY KEY,
  body   TEXT NOT NULL
);

CREATE TABLE observations (
  id           INTEGER PRIMARY KEY,
  run_id       INTEGER NOT NULL REFERENCES runs(id),
  scope_id     INTEGER NOT NULL REFERENCES scopes(id),
  response_id  INTEGER REFERENCES responses(id),
  position     INTEGER,
  document_id  INTEGER NOT NULL REFERENCES documents(id),
  parent       TEXT NOT NULL,
  sha256       TEXT NOT NULL REFERENCES versions(sha256),
  observed_at  TEXT NOT NULL,
  observed_day TEXT NOT NULL,
  UNIQUE (response_id, position),
  CHECK ((response_id IS NULL AND position IS NULL)
      OR (response_id IS NOT NULL AND position IS NOT NULL AND position >= 0))
);

CREATE INDEX observations_by_document ON observations(document_id, run_id, id);

CREATE INDEX observations_by_run ON observations(run_id, document_id, id);

CREATE INDEX observations_by_scope ON observations(scope_id);

CREATE TABLE legacy_sync_runs (
  id INTEGER PRIMARY KEY, started_at TEXT, finished_at TEXT, api_calls INTEGER, note TEXT
);

CREATE TABLE legacy_probes (
  seq INTEGER PRIMARY KEY, run_id INTEGER, path TEXT, status INTEGER,
  rows INTEGER, note TEXT, fetched_at TEXT
);

CREATE TABLE events (
  run_id        INTEGER NOT NULL REFERENCES runs(id),
  document_id   INTEGER NOT NULL REFERENCES documents(id),
  seq           INTEGER NOT NULL,
  event         TEXT NOT NULL CHECK (event IN
                  ('appeared','changed','reappeared','moved','removed','parent_gone')),
  old_sha256    TEXT REFERENCES versions(sha256),
  new_sha256    TEXT REFERENCES versions(sha256),
  old_parent    TEXT,
  new_parent    TEXT,
  effective_at  TEXT NOT NULL,
  effective_day TEXT NOT NULL,
  PRIMARY KEY (run_id, document_id, seq)
);

CREATE INDEX events_by_document ON events(document_id, run_id);

CREATE TABLE ambiguities (
  run_id            INTEGER NOT NULL REFERENCES runs(id),
  document_id       INTEGER NOT NULL REFERENCES documents(id),
  candidate_parents TEXT NOT NULL CHECK (json_valid(candidate_parents)),
  reason            TEXT NOT NULL,
  PRIMARY KEY (run_id, document_id)
);

CREATE TABLE projection_state (
  id            INTEGER PRIMARY KEY CHECK (id = 1),
  projected_run INTEGER NOT NULL REFERENCES runs(id),
  code_version  TEXT NOT NULL
);

CREATE TABLE document_state (
  document_id    INTEGER PRIMARY KEY REFERENCES documents(id),
  state          TEXT NOT NULL CHECK (state IN ('present','removed','parent_gone')),
  first_seen_run INTEGER NOT NULL REFERENCES runs(id),
  last_seen_run  INTEGER NOT NULL REFERENCES runs(id),
  state_run      INTEGER NOT NULL REFERENCES runs(id),
  ambiguous_run  INTEGER REFERENCES runs(id),
  parent         TEXT NOT NULL,
  sha256         TEXT NOT NULL REFERENCES versions(sha256),
  observed_at    TEXT NOT NULL,
  observed_day   TEXT NOT NULL
);

CREATE INDEX document_state_by_parent ON document_state(parent, state);

CREATE VIEW scope_coverage AS
SELECT
  k.kind,
  k.parent,
  (SELECT max(run_id) FROM scopes s
    WHERE s.kind = k.kind AND s.parent = k.parent AND s.outcome = 'complete') AS last_complete_run,
  (SELECT closed_at FROM scopes s
    WHERE s.kind = k.kind AND s.parent = k.parent AND s.outcome = 'complete'
    ORDER BY run_id DESC LIMIT 1) AS last_complete_at,
  (SELECT max(run_id) FROM scopes s
    WHERE s.kind = k.kind AND s.parent = k.parent AND s.infer_absence = 1) AS last_absence_safe_run,
  (SELECT min_date FROM scopes s
    WHERE s.kind = k.kind AND s.parent = k.parent AND s.infer_absence = 1
    ORDER BY run_id DESC LIMIT 1) AS last_min_date,
  (SELECT max(run_id) FROM scopes s
    WHERE s.kind = k.kind AND s.parent = k.parent) AS last_attempt_run,
  (SELECT outcome FROM scopes s
    WHERE s.kind = k.kind AND s.parent = k.parent ORDER BY run_id DESC LIMIT 1) AS last_outcome
FROM (SELECT DISTINCT kind, parent FROM scopes) k;

CREATE TABLE cutover (
  id             INTEGER PRIMARY KEY CHECK (id = 1),
  completed_at   TEXT NOT NULL,
  from_states    TEXT NOT NULL,
  acceptance_run INTEGER REFERENCES runs(id)
);

CREATE TABLE items (
  id            TEXT PRIMARY KEY,
  connector     TEXT, status TEXT, execution_status TEXT,
  last_updated  TEXT, next_auto_sync TEXT,
  raw            TEXT NOT NULL,
  fetched_at     TEXT NOT NULL,
  first_seen_run INTEGER NOT NULL,
  last_seen_run  INTEGER NOT NULL,
  state          TEXT NOT NULL CHECK (state IN ('present','removed','parent_gone')),
  ambiguous_run  INTEGER
);

CREATE TABLE accounts (
  id            TEXT PRIMARY KEY,
  item_id       TEXT, type TEXT, subtype TEXT, name TEXT,
  balance       REAL, currency TEXT,
  raw            TEXT NOT NULL,
  fetched_at     TEXT NOT NULL,
  first_seen_run INTEGER NOT NULL,
  last_seen_run  INTEGER NOT NULL,
  state          TEXT NOT NULL CHECK (state IN ('present','removed','parent_gone')),
  ambiguous_run  INTEGER
);

CREATE TABLE transactions (
  id            TEXT PRIMARY KEY,
  account_id    TEXT, date TEXT, description TEXT,
  amount        REAL, currency TEXT, type TEXT, category TEXT,
  raw            TEXT NOT NULL,
  fetched_at     TEXT NOT NULL,
  first_seen_run INTEGER NOT NULL,
  last_seen_run  INTEGER NOT NULL,
  state          TEXT NOT NULL CHECK (state IN ('present','removed','parent_gone')),
  ambiguous_run  INTEGER
);

CREATE INDEX transactions_by_date ON transactions(date DESC, id);

CREATE INDEX transactions_by_account ON transactions(account_id, date DESC);

CREATE TABLE bills (
  id            TEXT PRIMARY KEY,
  account_id    TEXT, due_date TEXT, closing_date TEXT,
  total_amount  REAL, minimum_payment REAL,
  raw            TEXT NOT NULL,
  fetched_at     TEXT NOT NULL,
  first_seen_run INTEGER NOT NULL,
  last_seen_run  INTEGER NOT NULL,
  state          TEXT NOT NULL CHECK (state IN ('present','removed','parent_gone')),
  ambiguous_run  INTEGER
);

CREATE TABLE investments (
  id            TEXT PRIMARY KEY,
  item_id       TEXT, type TEXT, subtype TEXT, name TEXT,
  balance       REAL, currency TEXT,
  raw            TEXT NOT NULL,
  fetched_at     TEXT NOT NULL,
  first_seen_run INTEGER NOT NULL,
  last_seen_run  INTEGER NOT NULL,
  state          TEXT NOT NULL CHECK (state IN ('present','removed','parent_gone')),
  ambiguous_run  INTEGER
);

CREATE TABLE investment_transactions (
  id            TEXT PRIMARY KEY,
  investment_id TEXT, date TEXT, amount REAL,
  raw            TEXT NOT NULL,
  fetched_at     TEXT NOT NULL,
  first_seen_run INTEGER NOT NULL,
  last_seen_run  INTEGER NOT NULL,
  state          TEXT NOT NULL CHECK (state IN ('present','removed','parent_gone')),
  ambiguous_run  INTEGER
);

CREATE TABLE consents (
  id            TEXT PRIMARY KEY,
  item_id       TEXT, expires_at TEXT, revoked_at TEXT,
  raw            TEXT NOT NULL,
  fetched_at     TEXT NOT NULL,
  first_seen_run INTEGER NOT NULL,
  last_seen_run  INTEGER NOT NULL,
  state          TEXT NOT NULL CHECK (state IN ('present','removed','parent_gone')),
  ambiguous_run  INTEGER
);

CREATE TABLE identities (
  id            TEXT PRIMARY KEY,
  item_id       TEXT, document TEXT, full_name TEXT,
  raw            TEXT NOT NULL,
  fetched_at     TEXT NOT NULL,
  first_seen_run INTEGER NOT NULL,
  last_seen_run  INTEGER NOT NULL,
  state          TEXT NOT NULL CHECK (state IN ('present','removed','parent_gone')),
  ambiguous_run  INTEGER
);

CREATE TABLE categories (
  id            TEXT PRIMARY KEY,
  description   TEXT, parent_id TEXT,
  raw            TEXT NOT NULL,
  fetched_at     TEXT NOT NULL,
  first_seen_run INTEGER NOT NULL,
  last_seen_run  INTEGER NOT NULL,
  state          TEXT NOT NULL CHECK (state IN ('present','removed','parent_gone')),
  ambiguous_run  INTEGER
);

CREATE TRIGGER responses_no_update BEFORE UPDATE ON responses BEGIN SELECT RAISE(ABORT, 'responses is append-only'); END;

CREATE TRIGGER responses_no_delete BEFORE DELETE ON responses BEGIN SELECT RAISE(ABORT, 'responses is append-only'); END;

CREATE TRIGGER page_derivations_no_update BEFORE UPDATE ON page_derivations BEGIN SELECT RAISE(ABORT, 'page_derivations is append-only'); END;

CREATE TRIGGER page_derivations_no_delete BEFORE DELETE ON page_derivations BEGIN SELECT RAISE(ABORT, 'page_derivations is append-only'); END;

CREATE TRIGGER documents_no_update BEFORE UPDATE ON documents BEGIN SELECT RAISE(ABORT, 'documents is append-only'); END;

CREATE TRIGGER documents_no_delete BEFORE DELETE ON documents BEGIN SELECT RAISE(ABORT, 'documents is append-only'); END;

CREATE TRIGGER versions_no_update BEFORE UPDATE ON versions BEGIN SELECT RAISE(ABORT, 'versions is append-only'); END;

CREATE TRIGGER versions_no_delete BEFORE DELETE ON versions BEGIN SELECT RAISE(ABORT, 'versions is append-only'); END;

CREATE TRIGGER observations_no_update BEFORE UPDATE ON observations BEGIN SELECT RAISE(ABORT, 'observations is append-only'); END;

CREATE TRIGGER observations_no_delete BEFORE DELETE ON observations BEGIN SELECT RAISE(ABORT, 'observations is append-only'); END;

CREATE TRIGGER legacy_sync_runs_no_update BEFORE UPDATE ON legacy_sync_runs BEGIN SELECT RAISE(ABORT, 'legacy_sync_runs is append-only'); END;

CREATE TRIGGER legacy_sync_runs_no_delete BEFORE DELETE ON legacy_sync_runs BEGIN SELECT RAISE(ABORT, 'legacy_sync_runs is append-only'); END;

CREATE TRIGGER legacy_probes_no_update BEFORE UPDATE ON legacy_probes BEGIN SELECT RAISE(ABORT, 'legacy_probes is append-only'); END;

CREATE TRIGGER legacy_probes_no_delete BEFORE DELETE ON legacy_probes BEGIN SELECT RAISE(ABORT, 'legacy_probes is append-only'); END;

CREATE TRIGGER cutover_no_update BEFORE UPDATE ON cutover BEGIN SELECT RAISE(ABORT, 'cutover is append-only'); END;

CREATE TRIGGER cutover_no_delete BEFORE DELETE ON cutover BEGIN SELECT RAISE(ABORT, 'cutover is append-only'); END;

CREATE TRIGGER runs_no_delete BEFORE DELETE ON runs BEGIN SELECT RAISE(ABORT, 'runs rows are never deleted'); END;

CREATE TRIGGER runs_close_once BEFORE UPDATE ON runs
WHEN OLD.outcome <> 'running'
  OR NEW.id IS NOT OLD.id
  OR NEW.started_at IS NOT OLD.started_at
  OR NEW.started_day IS NOT OLD.started_day
  OR NEW.code_version IS NOT OLD.code_version
  OR NEW.item_ids IS NOT OLD.item_ids
BEGIN
  SELECT RAISE(ABORT, 'runs: only the terminal fields may change, and only while running');
END;

CREATE TRIGGER scopes_no_delete BEFORE DELETE ON scopes BEGIN SELECT RAISE(ABORT, 'scopes rows are never deleted'); END;

CREATE TRIGGER scopes_close_once BEFORE UPDATE ON scopes
WHEN OLD.outcome <> 'pending'
  OR NEW.id IS NOT OLD.id
  OR NEW.run_id IS NOT OLD.run_id
  OR NEW.kind IS NOT OLD.kind
  OR NEW.parent IS NOT OLD.parent
  OR NEW.expected IS NOT OLD.expected
BEGIN
  SELECT RAISE(ABORT, 'scopes: only the terminal fields may change, and only while pending');
END;

CREATE VIEW run_coverage AS
SELECT
  r.id AS run_id, r.started_at, r.started_day, r.finished_at, r.finished_day, r.outcome,
  r.api_calls,
  (SELECT coalesce(sum(expected), 0) FROM scopes s WHERE s.run_id = r.id) AS expected,
  (SELECT count(*) FROM scopes s WHERE s.run_id = r.id AND s.expected = 1
     AND s.outcome = 'complete') AS complete,
  (SELECT count(*) FROM scopes s WHERE s.run_id = r.id AND s.expected = 1
     AND s.outcome = 'unavailable') AS unavailable,
  (SELECT count(*) FROM scopes s WHERE s.run_id = r.id AND s.expected = 1
     AND s.outcome = 'incomplete') AS incomplete,
  (SELECT count(*) FROM scopes s WHERE s.run_id = r.id AND s.expected = 1
     AND s.outcome = 'skipped') AS skipped,
  (SELECT count(*) FROM scopes s WHERE s.run_id = r.id AND s.expected = 1
     AND s.outcome = 'pending') AS pending,
  (SELECT count(*) FROM ambiguities a WHERE a.run_id = r.id) AS ambiguous_documents
FROM runs r;

CREATE VIEW daily_sightings AS
SELECT o.document_id, o.run_id, o.observed_day AS day, o.id AS observation_id,
       o.parent, o.sha256, o.observed_at, EXISTS (SELECT 1 FROM ambiguities a WHERE a.run_id = o.run_id AND a.document_id = o.document_id) AS ambiguous
FROM observations o
WHERE o.run_id = (SELECT max(x.run_id) FROM observations x
                  WHERE x.document_id = o.document_id AND x.observed_day = o.observed_day)
  AND o.id = CASE WHEN EXISTS (SELECT 1 FROM ambiguities a WHERE a.run_id = o.run_id AND a.document_id = o.document_id)
    THEN (SELECT y.id FROM observations y
          WHERE y.document_id = o.document_id AND y.run_id = o.run_id
            AND y.observed_day = o.observed_day
          ORDER BY y.parent, y.sha256, y.id DESC LIMIT 1)
    ELSE (SELECT max(y.id) FROM observations y
          WHERE y.document_id = o.document_id AND y.run_id = o.run_id
            AND y.observed_day = o.observed_day)
  END;

CREATE VIEW account_balance_history AS
SELECT s.run_id, s.day, s.observed_at, json_extract(v.body, '$.updatedAt') AS provider_updated_at,
       d.provider_id AS account_id, s.observation_id,
       json_extract(v.body, '$.name') AS name,
       json_extract(v.body, '$.type') AS type,
       json_extract(v.body, '$.currencyCode') AS currency,
       json_extract(v.body, '$.balance') AS balance,
       json_extract(v.body, '$.bankData.closingBalance') AS closing_balance,
       json_extract(v.body, '$.bankData.automaticallyInvestedBalance')
         AS automatically_invested_balance,
       json_extract(v.body, '$.creditData.creditLimit') AS credit_limit,
       json_extract(v.body, '$.creditData.availableCreditLimit') AS available_credit_limit
FROM daily_sightings s
JOIN documents d ON d.id = s.document_id AND d.kind = 'accounts'
JOIN versions v ON v.sha256 = s.sha256;

CREATE VIEW reserved_balance_history AS
SELECT run_id, day, observed_at, provider_updated_at, account_id, observation_id, position,
       reserve_id, stable_identity, reserve_name,
       CASE WHEN n = 1 THEN json_extract(entry, '$.availableAmounts[0].currencyCode') END
         AS currency,
       CASE WHEN n = 1 THEN json_extract(entry, '$.availableAmounts[0].amount') END AS amount,
       CASE WHEN n = 1 THEN json_extract(entry, '$.availableAmounts[0].remuneration.indexer') END
         AS indexer,
       CASE WHEN n = 1
         THEN json_extract(entry, '$.availableAmounts[0].remuneration.postFixedIndexerPercentage')
       END AS indexer_ratio,
       CASE WHEN n = 1
             AND json_extract(entry, '$.availableAmounts[0].remuneration.indexer') = 'CDI'
         THEN json_extract(entry, '$.availableAmounts[0].remuneration.postFixedIndexerPercentage')
              * 100
       END AS cdi_percentage,
       CASE WHEN n IS NULL THEN 'availableAmounts is not an array'
            WHEN n = 0 THEN 'no availableAmounts element'
            WHEN n > 1 THEN 'several availableAmounts elements'
            ELSE 'ok' END AS amount_status
FROM (
  SELECT s.run_id, s.day, s.observed_at, json_extract(v.body, '$.updatedAt') AS provider_updated_at,
         d.provider_id AS account_id, s.observation_id, r.key AS position, r.value AS entry,
         json_extract(r.value, '$.identification') AS reserve_id,
         CASE WHEN json_type(r.value, '$.identification') = 'text'
               AND json_extract(r.value, '$.identification') <> ''
               AND (SELECT count(*) FROM json_each(v.body, '$.bankData.reservedBalances') q
                    WHERE json_extract(q.value, '$.identification')
                        = json_extract(r.value, '$.identification')) = 1
              THEN 1 ELSE 0 END AS stable_identity,
         json_extract(r.value, '$.name') AS reserve_name,
         CASE WHEN json_type(r.value, '$.availableAmounts') = 'array'
              THEN json_array_length(r.value, '$.availableAmounts') END AS n
  FROM daily_sightings s
  JOIN documents d ON d.id = s.document_id AND d.kind = 'accounts'
  JOIN versions v ON v.sha256 = s.sha256,
  json_each(v.body, '$.bankData.reservedBalances') r
  WHERE json_type(v.body, '$.bankData.reservedBalances') = 'array'
);

CREATE VIEW investment_history AS
SELECT s.run_id, s.day, s.observed_at, json_extract(v.body, '$.updatedAt') AS provider_updated_at,
       json_extract(v.body, '$.date') AS provider_date,
       d.provider_id AS investment_id,
       json_extract(v.body, '$.name') AS name,
       json_extract(v.body, '$.currencyCode') AS currency,
       json_extract(v.body, '$.amountOriginal') AS amount_original,
       json_extract(v.body, '$.amount') AS amount_gross,
       json_extract(v.body, '$.taxes') AS taxes,
       json_extract(v.body, '$.taxes2') AS taxes2,
       json_extract(v.body, '$.amountWithdrawal') AS amount_net,
       json_extract(v.body, '$.rate') AS rate,
       json_extract(v.body, '$.rateType') AS rate_type,
       json_extract(v.body, '$.dueDate') AS due_date,
       json_extract(v.body, '$.status') AS status
FROM daily_sightings s
JOIN documents d ON d.id = s.document_id AND d.kind = 'investments'
JOIN versions v ON v.sha256 = s.sha256;

CREATE VIEW transaction_versions AS
SELECT document_id, provider_id, account_id, run_id, day, observed_at, provider_updated_at,
       sha256, status, date, amount, currency, description, category, bill_post_date, events,
       ambiguous
FROM (
  SELECT o.document_id, d.provider_id, o.parent AS account_id, o.run_id, o.observed_day AS day,
         o.observed_at, json_extract(v.body, '$.updatedAt') AS provider_updated_at, o.sha256,
         json_extract(v.body, '$.status') AS status,
         json_extract(v.body, '$.date') AS date,
         json_extract(v.body, '$.amount') AS amount,
         json_extract(v.body, '$.currencyCode') AS currency,
         json_extract(v.body, '$.description') AS description,
         json_extract(v.body, '$.category') AS category,
         json_extract(v.body, '$.creditCardMetadata.billPostDate') AS bill_post_date,
         (SELECT group_concat(event, ',') FROM (
            SELECT e.event FROM events e
            WHERE e.run_id = o.run_id AND e.document_id = o.document_id ORDER BY e.seq)) AS events,
         EXISTS (SELECT 1 FROM ambiguities a WHERE a.run_id = o.run_id AND a.document_id = o.document_id) AS ambiguous,
         lag(o.sha256) OVER w AS previous_sha256,
         lag(o.parent) OVER w AS previous_parent,
         lag(EXISTS (SELECT 1 FROM ambiguities a WHERE a.run_id = o.run_id AND a.document_id = o.document_id)) OVER w AS previous_ambiguous
  FROM observations o
  JOIN documents d ON d.id = o.document_id AND d.kind = 'transactions'
  JOIN versions v ON v.sha256 = o.sha256
  WINDOW w AS (PARTITION BY o.document_id ORDER BY o.run_id, o.id)
)
WHERE previous_sha256 IS NULL OR previous_sha256 <> sha256 OR previous_parent <> account_id
   OR previous_ambiguous <> ambiguous OR events IS NOT NULL;

CREATE VIEW document_coverage AS
SELECT d.id AS document_id, d.kind, d.provider_id, ds.parent, ds.state, ds.ambiguous_run,
       CASE d.kind
    WHEN 'transactions' THEN date(json_extract(v.body, '$.date'))
    WHEN 'investment_transactions' THEN substr(json_extract(v.body, '$.date'), 1, 10)
    WHEN 'bills' THEN substr(json_extract(v.body, '$.dueDate'), 1, 10)
  END AS business_date,
       c.last_absence_safe_run, c.last_min_date,
       CASE WHEN c.last_absence_safe_run IS NOT NULL
             AND c.last_absence_safe_run = (SELECT projected_run FROM projection_state WHERE id = 1)
             AND (d.kind NOT IN ('transactions', 'investment_transactions', 'bills')
                  OR (c.last_min_date IS NOT NULL AND CASE d.kind
    WHEN 'transactions' THEN date(json_extract(v.body, '$.date'))
    WHEN 'investment_transactions' THEN substr(json_extract(v.body, '$.date'), 1, 10)
    WHEN 'bills' THEN substr(json_extract(v.body, '$.dueDate'), 1, 10)
  END >= c.last_min_date))
            THEN 1 ELSE 0 END AS covered
FROM document_state ds
JOIN documents d ON d.id = ds.document_id
JOIN versions v ON v.sha256 = ds.sha256
LEFT JOIN scope_coverage c ON c.kind = d.kind AND c.parent = ds.parent;

CREATE VIEW pending_posted_candidates AS
SELECT p.account_id, p.id AS pending_id, p.date AS pending_date,
       p.fetched_at AS pending_observed_at, pc.covered AS pending_covered,
       q.id AS posted_id, q.date AS posted_date, q.fetched_at AS posted_observed_at,
       p.amount, p.currency, p.description
FROM transactions p
JOIN transactions q ON q.account_id = p.account_id AND q.id <> p.id
JOIN document_coverage pc ON pc.kind = 'transactions' AND pc.provider_id = p.id
JOIN document_coverage qc ON qc.kind = 'transactions' AND qc.provider_id = q.id
WHERE json_extract(p.raw, '$.status') = 'PENDING' AND json_extract(q.raw, '$.status') = 'POSTED'
  AND p.state = 'present' AND q.state = 'present'
  AND p.ambiguous_run IS NULL AND q.ambiguous_run IS NULL
  AND qc.covered = 1
  AND p.currency IS q.currency AND p.amount = q.amount AND p.description IS q.description
  AND date(p.date) = date(q.date);

