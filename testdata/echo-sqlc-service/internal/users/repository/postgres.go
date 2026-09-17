package repository

import (
	"context"

	database "example.com/echo-sqlc-service/internal/database/sqlc"
	"example.com/echo-sqlc-service/internal/users/model"
)

type UserQueries interface {
	GetUser(context.Context, int64) (database.User, error)
}

type Postgres struct {
	queries UserQueries
}

func New(queries UserQueries) *Postgres {
	return &Postgres{queries: queries}
}

func (repository *Postgres) GetByID(ctx context.Context, id int64) (model.User, error) {
	row, err := repository.queries.GetUser(ctx, id)
	if err != nil {
		return model.User{}, err
	}
	return model.User{ID: row.ID, Name: row.Name}, nil
}
