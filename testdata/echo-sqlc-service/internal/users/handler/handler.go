package handler

import (
	"context"
	"net/http"
	"strconv"

	"example.com/echo-sqlc-service/internal/users/model"
	"github.com/labstack/echo/v4"
)

type Service interface {
	GetUser(context.Context, int64) (model.User, error)
}

type Handler struct {
	service   Service
	converter Converter
}

func New(service Service, converter Converter) *Handler {
	return &Handler{service: service, converter: converter}
}

func (handler *Handler) GetUser(ctx echo.Context) error {
	id, err := strconv.ParseInt(ctx.Param("id"), 10, 64)
	if err != nil {
		return ctx.JSON(http.StatusBadRequest, map[string]string{"error": "invalid user id"})
	}

	user, err := handler.service.GetUser(ctx.Request().Context(), id)
	if err != nil {
		return ctx.JSON(http.StatusInternalServerError, map[string]string{"error": "get user"})
	}
	return ctx.JSON(http.StatusOK, handler.converter.ToResponse(user))
}
