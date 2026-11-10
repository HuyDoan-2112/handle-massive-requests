package bootstrap

import (
	"github.com/jackc/pgx/v5/pgxpool"
	"google.golang.org/grpc"
	"google.golang.org/grpc/health"
	healthpb "google.golang.org/grpc/health/grpc_health_v1"
	"google.golang.org/grpc/reflection"

	ticketv1 "github.com/HuyDoan-2112/handle-massive-requests/gen/ticket/v1"
	"github.com/HuyDoan-2112/handle-massive-requests/internal/grpcapi"
	"github.com/HuyDoan-2112/handle-massive-requests/internal/ticket"
)

// NewGRPCServer wires application dependencies and registers the gRPC services.
func NewGRPCServer(pool *pgxpool.Pool) *grpc.Server {
	repository := ticket.NewRepository(pool)
	service := ticket.NewService(repository)
	ticketServer := grpcapi.NewTicketServer(service)
	server := grpc.NewServer()

	ticketv1.RegisterTicketServiceServer(server, ticketServer)
	healthpb.RegisterHealthServer(server, health.NewServer())
	reflection.Register(server) // lets grpcurl / Postman discover the API

	return server
}
