package reverseproxy

import "github.com/caddyserver/caddy/v2"

type adminUpstreams struct{}

func NewAdminUpstreams() caddy.AdminRouter { return adminUpstreams{} }

func (al adminUpstreams) Routes() []caddy.AdminRoute {
	return []caddy.AdminRoute{{Pattern: "/reverse_proxy/upstreams", Handler: al.handleUpstreams}}
}

func (adminUpstreams) handleUpstreams() {}
