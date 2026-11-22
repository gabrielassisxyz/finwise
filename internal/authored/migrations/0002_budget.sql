-- The budget: a line's identity, the versions of the budget by month, the lines
-- of each version, the scenarios the person saved and the months they closed.
--
-- A month is text, YYYY-MM, so it sorts as it reads. An amount is integer cents.

-- A budget line's identity. AUTOINCREMENT: an id is never reused, so an id a
-- client holds never names a newer line. A line's name is not here: each
-- version owns the names its month shows. Not journaled; an id once minted
-- exists for good, and undoing a save leaves its ids behind, unused.
CREATE TABLE lines (
  id      INTEGER PRIMARY KEY AUTOINCREMENT,
  builtin INTEGER NOT NULL DEFAULT 0 CHECK (builtin IN (0, 1))
) STRICT;

-- The built-in fees line, with an explicit id so every database has it under
-- the same one. It belongs to no group and to no version.
INSERT INTO lines (id, builtin) VALUES (1, 1);

CREATE UNIQUE INDEX lines_one_builtin ON lines (builtin) WHERE builtin = 1;

CREATE TABLE budget_versions (
  from_month TEXT PRIMARY KEY NOT NULL
             CHECK (from_month GLOB '[0-9][0-9][0-9][0-9]-[0-9][0-9]')
) STRICT;

-- The key is the whole identity of a row: a rename or a change of id is a row
-- removed and a row written, never an update in place, so a save that hands a
-- name from one line to another never meets the uniqueness of names halfway.
CREATE TABLE budget_version_lines (
  from_month TEXT NOT NULL REFERENCES budget_versions (from_month),
  group_key  TEXT NOT NULL CHECK (group_key IN ('rec', 'fix', 'var', 'prov', 'res')),
  position   INTEGER NOT NULL CHECK (position >= 0),
  line_id    INTEGER NOT NULL REFERENCES lines (id),
  name       TEXT NOT NULL CHECK (trim(name) <> ''),
  cents      INTEGER NOT NULL CHECK (cents >= 0),
  flag       TEXT CHECK (flag IS NULL OR flag <> ''),
  PRIMARY KEY (from_month, line_id, name),
  UNIQUE (from_month, line_id),
  UNIQUE (from_month, name)
) STRICT;

CREATE TABLE scenarios (
  name     TEXT PRIMARY KEY NOT NULL CHECK (name <> ''),
  -- The list's order; a scenario saved again goes to the end.
  position INTEGER NOT NULL UNIQUE CHECK (position >= 0),
  doc      TEXT NOT NULL CHECK (json_valid(doc))
) STRICT;

CREATE TABLE closed_months (
  month     TEXT PRIMARY KEY NOT NULL
            CHECK (month GLOB '[0-9][0-9][0-9][0-9]-[0-9][0-9]'),
  closed_on TEXT NOT NULL
            CHECK (closed_on GLOB '[0-9][0-9][0-9][0-9]-[0-9][0-9]-[0-9][0-9]')
) STRICT;
