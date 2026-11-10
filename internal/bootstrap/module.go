package bootstrap

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5/pgxpool"
	"go.uber.org/fx"
	"google.golang.org/grpc/health"

	"github.com/HuyDoan-2112/handle-massive-requests/internal/config"
	"github.com/HuyDoan-2112/handle-massive-requests/internal/grpcapi"
	"github.com/HuyDoan-2112/handle-massive-requests/internal/ticket"
)

var Module = fx.Module("api",
	fx.Provide(
		config.LoadConfig,
		newPool,
		ticket.NewRepository,
		fx.Annotate(ticket.NewService, fx.From(new(*ticket.Repository))),
		fx.Annotate(grpcapi.NewTicketServer, fx.From(new(*ticket.Service))),
		health.NewServer,
		NewGRPCServer,
	),
	fx.Invoke(registerGRPCLifecycle),
)

func newPool(lifecycle fx.Lifecycle, cfg *config.Config) (*pgxpool.Pool, error) {
	pool, err := pgxpool.New(context.Background(), cfg.PostgresURL)
	if err != nil {
		return nil, fmt.Errorf("create pool: %w", err)
	}
	lifecycle.Append(fx.Hook{
		OnStart: func(ctx context.Context) error {
			if err := pool.Ping(ctx); err != nil {
				pool.Close()
				return fmt.Errorf("ping database: %w", err)
			}
			return nil
		},
		OnStop: func(context.Context) error {
			pool.Close()
			return nil
		},
	})
	return pool, nil
}
