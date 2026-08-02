package command

import (
	"net"
	"net/http"
)

type route struct {
	pattern string
	handler http.Handler
}

type siteHandler struct{}

func (siteHandler) ServeHTTP(http.ResponseWriter, *http.Request) {}

type serverOwner struct {
	server *http.Server
}

var callback func()

func Register(run func()) {
	callback = run
}

func Execute() {
	if callback != nil {
		callback()
	}
}

func addRoute(mux *http.ServeMux, pattern string, handler http.Handler) {
	mux.Handle(pattern, handler)
}

func suppliedRoutes() []route {
	return nil
}

func run() {
	mux := http.NewServeMux()
	addRoute(mux, "/config/", http.HandlerFunc(siteHandler{}.ServeHTTP))
	addRoute(mux, "/stop", http.HandlerFunc(siteHandler{}.ServeHTTP))
	for _, candidate := range suppliedRoutes() {
		addRoute(mux, candidate.pattern, candidate.handler)
	}

	listener, _ := net.Listen("tcp", "localhost:0")
	adminServer := &http.Server{Handler: mux}
	go adminServer.Serve(listener)

	owner := &serverOwner{}
	owner.server = &http.Server{Handler: siteHandler{}}
	go owner.server.Serve(listener)
}

func init() {
	Register(run)
}
