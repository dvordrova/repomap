package main

import "github.com/gin-gonic/gin"

func create(*gin.Context) {}

func registerJSON(group *gin.RouterGroup, path string, handler gin.HandlerFunc) {
	group.GET(path, handler)
}

func main() {
	group := &gin.RouterGroup{}
	registerJSON(group, "/runs", create)
}
