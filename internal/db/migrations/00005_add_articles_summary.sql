-- +goose Up
-- NOT NULL needs a DEFAULT here: existing rows have no value, and without one
-- the ALTER fails on any non-empty table. Empty string rather than NULL keeps
-- the Go type a plain string instead of pgtype.Text, so callers never have to
-- check .Valid for a field that is semantically "not written yet".
ALTER TABLE articles ADD COLUMN summary TEXT NOT NULL DEFAULT '';

-- +goose Down
ALTER TABLE articles DROP COLUMN summary;
