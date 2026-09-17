package app

import (
	"database/sql"
	"os"

	database "example.com/echo-sqlc-service/internal/database/sqlc"
	"example.com/echo-sqlc-service/internal/users/repository"
	"example.com/echo-sqlc-service/internal/users/service"
	_ "github.com/lib/pq"
)

type Users struct {
	connection *sql.DB
	Service    *service.Service
}

func NewUsers() (*Users, error) {
	connection, err := sql.Open("postgres", os.Getenv("DATABASE_URL"))
	if err != nil {
		return nil, err
	}

	queries := database.New(connection)
	usersRepository := repository.New(queries)
	usersService := service.New(usersRepository)
	return &Users{connection: connection, Service: usersService}, nil
}

func (users *Users) Close() error {
	return users.connection.Close()
}
