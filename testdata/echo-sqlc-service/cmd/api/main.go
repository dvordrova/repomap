package main

import (
	"log"

	"example.com/echo-sqlc-service/internal/app"
	"example.com/echo-sqlc-service/internal/users/handler"
	"github.com/labstack/echo/v4"
)

func main() {
	users, err := app.NewUsers()
	if err != nil {
		log.Fatal(err)
	}
	defer users.Close()

	usersHandler := handler.New(users.Service, &handler.ConverterImpl{})
	server := echo.New()
	server.GET("/users/:id", usersHandler.GetUser)
	log.Fatal(server.Start(":8080"))
}
