package metrics

import "github.com/caddyserver/caddy/v2"

type AdminMetrics struct{}

func NewAdminMetrics() caddy.AdminRouter { return new(AdminMetrics) }

func (m *AdminMetrics) Routes() []caddy.AdminRoute {
	return []caddy.AdminRoute{{Pattern: "/metrics", Handler: m.serveHTTP}}
}

func (*AdminMetrics) serveHTTP() {}
