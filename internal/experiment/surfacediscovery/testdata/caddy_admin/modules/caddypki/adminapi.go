package caddypki

import "github.com/caddyserver/caddy/v2"

type adminAPI struct{}

const adminPKIEndpointBase = "/pki/"

func NewAdminAPI() caddy.AdminRouter { return new(adminAPI) }

func (a *adminAPI) Routes() []caddy.AdminRoute {
	return []caddy.AdminRoute{{Pattern: adminPKIEndpointBase, Handler: a.handleAPIEndpoints}}
}

func (*adminAPI) handleAPIEndpoints() {}
