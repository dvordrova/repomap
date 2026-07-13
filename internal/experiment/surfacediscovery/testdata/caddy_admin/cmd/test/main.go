package main

import (
	"github.com/caddyserver/caddy/v2"
	"github.com/caddyserver/caddy/v2/caddyconfig"
	"github.com/caddyserver/caddy/v2/modules/caddyhttp"
	"github.com/caddyserver/caddy/v2/modules/caddyhttp/reverseproxy"
	"github.com/caddyserver/caddy/v2/modules/caddypki"
	"github.com/caddyserver/caddy/v2/modules/metrics"
)

func main() {
	routers := []caddy.AdminRouter{
		caddyconfig.NewAdminLoad(),
		metrics.NewAdminMetrics(),
		caddypki.NewAdminAPI(),
		reverseproxy.NewAdminUpstreams(),
	}
	for _, router := range routers {
		_ = router.Routes()
	}
	_ = caddyhttp.RouteList{}.Compile(nil)
}
