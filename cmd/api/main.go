package main

import (
	"context"
	"fmt"
	"log"
	"net"
	"os"
	"os/signal"
	"syscall"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/health"
	healthpb "google.golang.org/grpc/health/grpc_health_v1"
	"google.golang.org/grpc/reflection"

	ticketv1 "github.com/HuyDoan-2112/handle-massive-requests/gen/ticket/v1"
	"github.com/HuyDoan-2112/handle-massive-requests/internal/config"
	"github.com/HuyDoan-2112/handle-massive-requests/internal/db"
	"github.com/HuyDoan-2112/handle-massive-requests/internal/grpcapi"
	"github.com/HuyDoan-2112/handle-massive-requests/internal/ticket"
)

func main() {
	cfg, err := config.LoadConfig()
	if err != nil {
		log.Fatalf("failed to load config: %v", err)
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	pool, err := db.NewPool(ctx, cfg.PostgresURL)
	if err != nil {
		log.Fatalf("failed to connect to database: %v", err)
	}
	defer pool.Close()

	// gRPC: ticket API.
	repository := ticket.NewRepository(pool)
	service := ticket.NewService(repository)
	ticketServer := grpcapi.NewTicketServer(service)
	grpcSrv := grpc.NewServer()
	ticketv1.RegisterTicketServiceServer(grpcSrv, ticketServer)
	healthpb.RegisterHealthServer(grpcSrv, health.NewServer())
	reflection.Register(grpcSrv) // lets grpcurl / Postman discover the API

	lis, err := net.Listen("tcp", fmt.Sprintf(":%s", cfg.GRPCPort))
	if err != nil {
		log.Fatalf("grpc listen: %v", err)
	}
	go func() {
		log.Printf("grpc listening on :%s", cfg.GRPCPort)
		if err := grpcSrv.Serve(lis); err != nil {
			log.Fatalf("grpc server: %v", err)
		}
	}()

	<-ctx.Done()
	log.Println("shutting down")

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	stopped := make(chan struct{})
	go func() { grpcSrv.GracefulStop(); close(stopped) }()
	select {
	case <-stopped:
	case <-shutdownCtx.Done():
		grpcSrv.Stop()
	}
}
