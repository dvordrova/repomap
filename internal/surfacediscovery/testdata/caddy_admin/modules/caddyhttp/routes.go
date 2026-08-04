package caddyhttp

type Handler interface{}

type RouteList []int

func (routes RouteList) Compile(next Handler) Handler {
	return next
}
