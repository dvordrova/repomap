package main

import (
	"github.com/beego/beego/v2/server/web"
)

type APIController struct{}

func (c *APIController) Get() {}

func initAPI() {
	ns := web.NewNamespace("/",
		web.NSNamespace("/api",
			web.NSInclude(
				&APIController{},
			),
		),
	)
	web.AddNamespace(ns)
}

func main() {
	initAPI()
}
