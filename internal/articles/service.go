package articles

import (
	"context"

	"github.com/google/uuid"
	"github.com/hibiken/asynq"

	"go-backend-template/internal/db/sqlc"
	"go-backend-template/internal/tasks"
)

type articlesRepository interface {
	Create(ctx context.Context, userID uuid.UUID, title, body string) (sqlc.Article, error)
	GetOwned(ctx context.Context, id, userID uuid.UUID) (sqlc.Article, error)
	ListOwned(ctx context.Context, userID uuid.UUID, status, q string, limit, offset int32) ([]sqlc.Article, int64, error)
	Update(ctx context.Context, id, userID uuid.UUID, title, body *string) (sqlc.Article, error)
	Delete(ctx context.Context, id, userID uuid.UUID) error
	PublishIfDraft(ctx context.Context, id, userID uuid.UUID) (bool, error)
}

var _ articlesRepository = (*Repository)(nil)

type Service struct {
	repo    articlesRepository
	enqueue func(*asynq.Task) error
}

func NewService(repo articlesRepository, enqueue func(*asynq.Task) error) *Service {
	return &Service{repo: repo, enqueue: enqueue}
}

func (s *Service) Create(ctx context.Context, userID uuid.UUID, title, body string) (sqlc.Article, error) {
	return s.repo.Create(ctx, userID, title, body)
}

func (s *Service) Get(ctx context.Context, id, userID uuid.UUID) (sqlc.Article, error) {
	return s.repo.GetOwned(ctx, id, userID)
}

func (s *Service) List(ctx context.Context, userID uuid.UUID, status, q string, page, pageSize int32) ([]sqlc.Article, int64, error) {
	offset := (page - 1) * pageSize
	return s.repo.ListOwned(ctx, userID, status, q, pageSize, offset)
}

func (s *Service) Update(ctx context.Context, id, userID uuid.UUID, title, body *string) (sqlc.Article, error) {
	return s.repo.Update(ctx, id, userID, title, body)
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
		task, err := tasks.NewArticlePublishedTask(id.String())
		if err != nil {
			return sqlc.Article{}, err
		}
		if err := s.enqueue(task); err != nil {
			return sqlc.Article{}, err
		}
	}
	return s.repo.GetOwned(ctx, id, userID)
}
