-- +goose Up
-- The service lowercases addresses before every read and write, so "A@x.com"
-- and "a@x.com" are one account. That invariant lived only in Go, which meant
-- any future code path that forgot to call normalizeEmail -- an admin tool, a
-- bulk import, a psql session -- could quietly create the duplicate the
-- application believes cannot exist. Enforce it in the database instead.
--
-- Existing rows are folded first so the index can be built. Addresses that
-- differ only by case are already ambiguous, so keep the oldest row (the
-- account that has existed longest) and delete the rest rather than failing
-- the migration and leaving the schema half-applied.
-- +goose StatementBegin
DELETE FROM users a
USING users b
WHERE lower(a.email) = lower(b.email)
  AND (a.created_at, a.id) > (b.created_at, b.id);
-- +goose StatementEnd

UPDATE users SET email = lower(email) WHERE email <> lower(email);

-- The plain UNIQUE on email stays: it is redundant with this index for
-- lowercase values but harmless, and dropping it would remove the constraint
-- backing any FK someone later adds.
CREATE UNIQUE INDEX idx_users_email_lower ON users (lower(email));

-- +goose Down
-- Only the index comes back off. The folding above is a one-way door: rows
-- deleted as case-duplicates and addresses rewritten to lowercase cannot be
-- restored from here, so rolling back relinquishes the constraint without
-- restoring the previous data. Restore from a backup if you need the old rows.
DROP INDEX idx_users_email_lower;
