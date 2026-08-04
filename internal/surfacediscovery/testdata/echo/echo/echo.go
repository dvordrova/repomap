package echo

import "net/http"

type Context interface{}

type HandlerFunc func(Context) error

type MiddlewareFunc func(HandlerFunc) HandlerFunc

type Route struct{}

type Echo struct{}

type Group struct {
	host       string
	prefix     string
	echo       *Echo
	middleware []MiddlewareFunc
}

func New() *Echo { return &Echo{} }

func (e *Echo) add(
	host, method, path string,
	handler HandlerFunc,
	middleware ...MiddlewareFunc,
) *Route {
	return &Route{}
}

func (e *Echo) Add(
	method, path string,
	handler HandlerFunc,
	middleware ...MiddlewareFunc,
) *Route {
	return e.add("", method, path, handler, middleware...)
}

func (e *Echo) GET(
	path string,
	handler HandlerFunc,
	middleware ...MiddlewareFunc,
) *Route {
	return e.Add(http.MethodGet, path, handler, middleware...)
}

func (e *Echo) Group(prefix string, middleware ...MiddlewareFunc) *Group {
	return &Group{prefix: prefix, echo: e, middleware: middleware}
}

func (g *Group) Add(
	method, path string,
	handler HandlerFunc,
	middleware ...MiddlewareFunc,
) *Route {
	all := append([]MiddlewareFunc{}, g.middleware...)
	all = append(all, middleware...)
	return g.echo.add(g.host, method, g.prefix+path, handler, all...)
}

func (g *Group) POST(
	path string,
	handler HandlerFunc,
	middleware ...MiddlewareFunc,
) *Route {
	return g.Add(http.MethodPost, path, handler, middleware...)
}
