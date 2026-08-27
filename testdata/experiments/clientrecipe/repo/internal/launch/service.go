package launch

import "context"

type SecretReader interface {
	ReadSecret(context.Context, string) (string, error)
}

type LaunchNotifier interface {
	SendLaunch(context.Context, string) error
}

type NamespaceLister interface {
	ListNamespaces(context.Context) ([]string, error)
}

type LaunchHistory interface {
	RecentLaunches(context.Context, int) ([]string, error)
}

type Service struct {
	secrets  SecretReader
	clusters NamespaceLister
	history  LaunchHistory
	notifier LaunchNotifier
}

func NewService(
	secrets SecretReader,
	clusters NamespaceLister,
	history LaunchHistory,
	notifier LaunchNotifier,
) *Service {
	return &Service{secrets: secrets, clusters: clusters, history: history, notifier: notifier}
}

func (service *Service) Resolve(ctx context.Context, request Request) (string, error) {
	secret, err := service.secrets.ReadSecret(ctx, request.SecretKey)
	if err != nil {
		return "", err
	}
	if err := service.notifier.SendLaunch(ctx, request.Message); err != nil {
		return "", err
	}
	if _, err := service.clusters.ListNamespaces(ctx); err != nil {
		return "", err
	}
	if _, err := service.history.RecentLaunches(ctx, request.HistoryLimit); err != nil {
		return "", err
	}
	return secret, nil
}

func (service *Service) Run(ctx context.Context) error {
	handler := NewHandler(service.Resolve)
	return handler.HandleStartup(ctx)
}
