package connect

import (
	"net/http"
)

type Context interface{}

type Request struct{}
type Response struct{}

type Handler struct {
	path    string
	handler http.Handler
}

type HandlerOption func(*Handler)

type ServeMux struct {
	handlers map[string]http.Handler
}

func NewServeMux(options ...ServeMuxOption) *ServeMux {
	return &ServeMux{handlers: map[string]http.Handler{}}
}

type ServeMuxOption func(*ServeMux)

func (m *ServeMux) Handle(pattern string, handler http.Handler) {
	m.handlers[pattern] = handler
}

func (m *ServeMux) ServeHTTP(writer http.ResponseWriter, request *http.Request) {
	if handler := m.handlers[request.URL.Path]; handler != nil {
		handler.ServeHTTP(writer, request)
	}
}

func NewUnaryHandler(procedure string, handler any, opts ...HandlerOption) *Handler {
	return &Handler{path: procedure}
}

func (h *Handler) ServeHTTP(writer http.ResponseWriter, request *http.Request) {}
