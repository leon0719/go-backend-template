-- +goose Up
-- 'archived' is a new terminal status for the atomic-transaction demo below
-- (articles.Repository.ArchiveWithEvent): dropping and recreating the CHECK
-- constraint is the only way to widen it, since Postgres has no ALTER CHECK.
ALTER TABLE articles DROP CONSTRAINT articles_status_check;
ALTER TABLE articles ADD CONSTRAINT articles_status_check CHECK (status IN ('draft', 'published', 'archived'));

-- article_events is an audit trail written in the SAME transaction as the
-- status change that produced it. The UNIQUE constraint is what the demo
-- integration test exploits to force the INSERT to fail after the UPDATE has
-- already run, proving the UPDATE rolls back too when the transaction aborts.
CREATE TABLE article_events (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    article_id UUID NOT NULL REFERENCES articles(id) ON DELETE CASCADE,
    event_type TEXT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (article_id, event_type)
);

CREATE INDEX idx_article_events_article_id ON article_events(article_id);

-- +goose Down
DROP TABLE article_events;

ALTER TABLE articles DROP CONSTRAINT articles_status_check;
ALTER TABLE articles ADD CONSTRAINT articles_status_check CHECK (status IN ('draft', 'published'));
