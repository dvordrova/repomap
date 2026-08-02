package command

import (
	"example.com/modular/internal/admin"
	"example.com/modular/internal/config"
	"example.com/modular/internal/lifecycle"
	"example.com/modular/internal/registry"
	"example.com/modular/internal/security"
	"example.com/modular/internal/web"
)

func Main() {
	config.Load()
	registry.LookupModule()
	lifecycle.Start()
	admin.StartAdmin()
	web.ServeHTTP()
	security.ConfigureTLS()
}
