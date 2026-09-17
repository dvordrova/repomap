package handler

import "example.com/echo-sqlc-service/internal/users/model"

//go:generate goverter gen .

// goverter:converter
// goverter:output:file ./converter_gen.go
type Converter interface {
	ToResponse(model.User) UserResponse
}

type UserResponse struct {
	ID   int64  `json:"id"`
	Name string `json:"name"`
}
