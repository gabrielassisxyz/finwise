-- The PIN, once it has been changed in the app. No row exists until the first
-- change, and the PIN in the server's environment opens the app until then.
-- The table is not journaled: changing the PIN is not a recorded change and has
-- no undo.
--
-- hash is the stored form pbkdf2-sha256$<iterations>$<salt>$<key>, never the PIN.
-- changed_at is an instant in UTC with milliseconds, as the journal writes them.

CREATE TABLE pin (
  id         INTEGER PRIMARY KEY NOT NULL CHECK (id = 1),
  hash       TEXT NOT NULL,
  changed_at TEXT NOT NULL
) STRICT;
