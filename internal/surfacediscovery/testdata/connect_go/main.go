package main

import (
	"net/http"

	"connectrpc.com/connect"
)

type Service struct{}

func (s *Service) List(ctx connect.Context, req *connect.Request) (*connect.Response, error) {
	return &connect.Response{}, nil
}

func NewServiceHandler(svc *Service, opts ...connect.HandlerOption) (string, http.Handler) {
	return "/example.connect.v1.Service/", connect.NewUnaryHandler("List", svc.List)
}

func main() {
	mux := connect.NewServeMux()
	path, handler := NewServiceHandler(&Service{})
	mux.Handle(path, handler)
	_ = http.ListenAndServe(":8080", mux)
}
