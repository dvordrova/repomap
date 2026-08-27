package launch

import "context"

type ResolveCallback func(context.Context, Request) (string, error)

type Handler struct {
	resolve ResolveCallback
}

func NewHandler(resolve ResolveCallback) *Handler {
	return &Handler{resolve: resolve}
}

func (handler *Handler) HandleStartup(ctx context.Context) error {
	_, err := handler.resolve(ctx, Request{
		SecretKey:    "launch/bootstrap",
		Message:      "service started",
		HistoryLimit: 2,
	})
	return err
}
