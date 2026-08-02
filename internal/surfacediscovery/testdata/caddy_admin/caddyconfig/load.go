package caddyconfig

import "github.com/caddyserver/caddy/v2"

type adminLoad struct{}

func NewAdminLoad() caddy.AdminRouter { return adminLoad{} }

func (al adminLoad) Routes() []caddy.AdminRoute {
	_ = caddy.AdminRoute{Pattern: "/not-returned", Handler: al.handleLoad}
	if false {
		return []caddy.AdminRoute{{Pattern: "/unreachable", Handler: al.handleLoad}}
	}
	handler := al.handleLoad
	return []caddy.AdminRoute{
		{Pattern: "/load", Handler: al.handleLoad},
		{Pattern: "/adapt", Handler: al.handleAdapt},
		{Pattern: "/wrapped", Handler: wrap(al.handleLoad)},
		{Pattern: "/variable", Handler: handler},
	}
}

func wrap(handler any) any { return handler }

func (adminLoad) handleLoad()  {}
func (adminLoad) handleAdapt() {}
