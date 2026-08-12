package articles

import (
	"context"
	"log/slog"

	"github.com/google/uuid"
	"github.com/hibiken/asynq"

	"go-backend-template/internal/db/sqlc"
	"go-backend-template/internal/tasks"
)

type articlesRepository interface {
	Create(ctx context.Context, userID uuid.UUID, title, body, summary string) (sqlc.Article, error)
	GetOwned(ctx context.Context, id, userID uuid.UUID) (sqlc.Article, error)
	ListOwned(ctx context.Context, userID uuid.UUID, status, q string, limit, offset int32) ([]sqlc.Article, int64, error)
	Update(ctx context.Context, id, userID uuid.UUID, title, body, summary *string) (sqlc.Article, error)
	Delete(ctx context.Context, id, userID uuid.UUID) error
	PublishIfDraft(ctx context.Context, id, userID uuid.UUID) (bool, error)
	ArchiveWithEvent(ctx context.Context, id, userID uuid.UUID) error
}

var _ articlesRepository = (*Repository)(nil)

type Service struct {
	repo    articlesRepository
	enqueue func(*asynq.Task) error
}

func NewService(repo articlesRepository, enqueue func(*asynq.Task) error) *Service {
	return &Service{repo: repo, enqueue: enqueue}
}

func (s *Service) Create(ctx context.Context, userID uuid.UUID, title, body, summary string) (sqlc.Article, error) {
	return s.repo.Create(ctx, userID, title, body, summary)
}

func (s *Service) Get(ctx context.Context, id, userID uuid.UUID) (sqlc.Article, error) {
	return s.repo.GetOwned(ctx, id, userID)
}

// maxOffset caps the SQL OFFSET. The computation is done in int64 because
// (page-1)*pageSize overflows int32 for large pages (e.g. page=100000000,
// page_size=100), wrapping to a NEGATIVE offset that Postgres rejects with a
// 500. Deep pagination past this point returns an empty page instead --
// keyset pagination is the right answer if you genuinely need to go deeper.
const maxOffset int64 = 1_000_000

func (s *Service) List(ctx context.Context, userID uuid.UUID, status, q string, page, pageSize int32) ([]sqlc.Article, int64, error) {
	offset := max((int64(page)-1)*int64(pageSize), 0)
	// Deliberately an if, not min(): gosec's G115 overflow check can't follow
	// the bound through min(), so it flags the int32 conversion below as an
	// unchecked overflow.
	if offset > maxOffset { //nolint:modernize
		offset = maxOffset
	}
	return s.repo.ListOwned(ctx, userID, status, q, pageSize, int32(offset))
}

func (s *Service) Update(ctx context.Context, id, userID uuid.UUID, title, body, summary *string) (sqlc.Article, error) {
	return s.repo.Update(ctx, id, userID, title, body, summary)
}

func (s *Service) Delete(ctx context.Context, id, userID uuid.UUID) error {
	return s.repo.Delete(ctx, id, userID)
}

func (s *Service) Publish(ctx context.Context, id, userID uuid.UUID) (sqlc.Article, error) {
	transitioned, err := s.repo.PublishIfDraft(ctx, id, userID)
	if err != nil {
		return sqlc.Article{}, err
	}
	if transitioned {
		if task, err := tasks.NewArticlePublishedTask(id.String()); err != nil {
			slog.ErrorContext(ctx, "failed to build article-published task; webhook will not be sent",
				"article_id", id, "error", err)
		} else if err := s.enqueue(task); err != nil {
			slog.ErrorContext(ctx, "failed to enqueue article-published task; webhook will not be sent",
				"article_id", id, "error", err)
		}
	}
	return s.repo.GetOwned(ctx, id, userID)
}

// Archive marks the article archived and writes an audit event in a single
// database transaction (see Repository.ArchiveWithEvent) -- unlike Publish,
// there is no cross-system step here, so this one has no dual-write gap.
func (s *Service) Archive(ctx context.Context, id, userID uuid.UUID) (sqlc.Article, error) {
	if err := s.repo.ArchiveWithEvent(ctx, id, userID); err != nil {
		return sqlc.Article{}, err
	}
	return s.repo.GetOwned(ctx, id, userID)
}
