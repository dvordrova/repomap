package main

import (
	"net/http"

	"github.com/labstack/echo/v4"
)

func handler(echo.Context) error { return nil }

func audit(next echo.HandlerFunc) echo.HandlerFunc { return next }

func register(group *echo.Group) {
	group.POST("/runs", handler, audit)
}

func main() {
	e := echo.New()
	e.Add(http.MethodDelete, "/direct", handler)
	e.GET("/health", handler, audit)
	register(e.Group("/api"))
	register(e.Group("/admin"))
}
