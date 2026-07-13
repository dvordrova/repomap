package caddy

type AdminRoute struct {
	Pattern string
	Handler any
}

type AdminRouter interface {
	Routes() []AdminRoute
}
