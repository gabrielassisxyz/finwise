-- The journal: every change made to authored data, with its author, and what it
-- did, effect by effect, so it can be undone. Nothing writes these tables yet.
--
-- An instant is text in UTC with milliseconds, 2026-12-01T01:30:00.000Z, so it
-- sorts as it reads; a day is the Brazil day, YYYY-MM-DD.

CREATE TABLE changes (
  -- AUTOINCREMENT: an id is never reused, so a stale handle never points at a
  -- newer change.
  id        INTEGER PRIMARY KEY AUTOINCREMENT,
  at        TEXT NOT NULL
            CHECK (at GLOB '[0-9][0-9][0-9][0-9]-[0-9][0-9]-[0-9][0-9]T[0-9][0-9]:[0-9][0-9]:[0-9][0-9].[0-9][0-9][0-9]Z'),
  day       TEXT NOT NULL
            CHECK (day GLOB '[0-9][0-9][0-9][0-9]-[0-9][0-9]-[0-9][0-9]'),
  -- person: an HTTP session; rule: the step that takes in a new snapshot and
  -- classifies new transactions by rule; model: a later nightly model call.
  author    TEXT NOT NULL CHECK (author IN ('person', 'rule', 'model')),
  method    TEXT NOT NULL CHECK (method <> ''),
  -- 1 when a later write appended its effects to this change.
  extended  INTEGER NOT NULL DEFAULT 0 CHECK (extended IN (0, 1)),
  undone_at TEXT
            CHECK (undone_at GLOB '[0-9][0-9][0-9][0-9]-[0-9][0-9]-[0-9][0-9]T[0-9][0-9]:[0-9][0-9]:[0-9][0-9].[0-9][0-9][0-9]Z')
) STRICT;

-- One row per effect, in the order seq within its change. key is the row's key
-- as canonical JSON, an array in column order. col is the column for a table of
-- independent facts per key, and null for a table whose rows are facts by
-- existing. before and after are JSON, null on the side where there was nothing.
CREATE TABLE change_effects (
  change_id INTEGER NOT NULL REFERENCES changes (id),
  seq       INTEGER NOT NULL CHECK (seq >= 1),
  tbl       TEXT NOT NULL CHECK (tbl <> ''),
  key       TEXT NOT NULL CHECK (json_valid(key) AND json_type(key) = 'array'),
  col       TEXT CHECK (col <> ''),
  before    TEXT CHECK (json_valid(before)),
  after     TEXT CHECK (json_valid(after)),
  PRIMARY KEY (change_id, seq),
  CHECK (before IS NOT NULL OR after IS NOT NULL)
) STRICT;

-- Values of the app itself, one per key, read and written only through
-- functions of the package that owns this file. The keys are not a CHECK:
-- this file is frozen by its checksum, and each later key would otherwise cost
-- a rebuild of the table.
CREATE TABLE meta (
  key   TEXT PRIMARY KEY NOT NULL CHECK (key <> ''),
  value TEXT NOT NULL,
  CHECK (key <> 'tracking_start_month' OR value GLOB '[0-9][0-9][0-9][0-9]-[0-9][0-9]')
) STRICT;
