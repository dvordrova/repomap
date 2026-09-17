package service

import (
	"context"

	"example.com/echo-sqlc-service/internal/users/model"
)

type Repository interface {
	GetByID(context.Context, int64) (model.User, error)
}

type Service struct {
	repository Repository
}

func New(repository Repository) *Service {
	return &Service{repository: repository}
}

func (service *Service) GetUser(ctx context.Context, id int64) (model.User, error) {
	return service.repository.GetByID(ctx, id)
}
