package bootstrap

import (
	"context"
	"errors"
	"fmt"
	"log"
	"net"

	"go.uber.org/fx"
	"google.golang.org/grpc"
	"google.golang.org/grpc/health"
	healthpb "google.golang.org/grpc/health/grpc_health_v1"
	"google.golang.org/grpc/reflection"

	ticketv1 "github.com/HuyDoan-2112/handle-massive-requests/gen/ticket/v1"
	"github.com/HuyDoan-2112/handle-massive-requests/internal/config"
	"github.com/HuyDoan-2112/handle-massive-requests/internal/grpcapi"
)

// NewGRPCServer registers the application's gRPC services.
func NewGRPCServer(ticketServer *grpcapi.TicketServer, healthServer *health.Server) *grpc.Server {
	server := grpc.NewServer()

	ticketv1.RegisterTicketServiceServer(server, ticketServer)
	healthpb.RegisterHealthServer(server, healthServer)
	reflection.Register(server) // lets grpcurl / Postman discover the API

	return server
}

func registerGRPCLifecycle(lifecycle fx.Lifecycle, shutdown fx.Shutdowner, server *grpc.Server, cfg *config.Config) {
	lifecycle.Append(fx.Hook{
		OnStart: func(context.Context) error {
			lis, err := net.Listen("tcp", net.JoinHostPort("", cfg.GRPCPort))
			if err != nil {
				return fmt.Errorf("grpc listen: %w", err)
			}
			log.Printf("grpc listening on %s", lis.Addr())
			go func() {
				if err := server.Serve(lis); err != nil && !errors.Is(err, grpc.ErrServerStopped) {
					log.Printf("grpc server: %v", err)
					if err := shutdown.Shutdown(fx.ExitCode(1)); err != nil {
						log.Printf("request shutdown: %v", err)
					}
				}
			}()
			return nil
		},
		OnStop: func(ctx context.Context) error {
			stopped := make(chan struct{})
			go func() {
				server.GracefulStop()
				close(stopped)
			}()
			select {
			case <-stopped:
				return nil
			case <-ctx.Done():
				server.Stop()
				return ctx.Err()
			}
		},
	})
}
