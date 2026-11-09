package main

import (
	"context"
	"errors"
	"fmt"
	"log"
	"net"
	"net/http"
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
	"github.com/HuyDoan-2112/handle-massive-requests/internal/httpapi"
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

	// HTTP: health only.
	httpSrv := &http.Server{
		Addr:              fmt.Sprintf(":%s", cfg.Port),
		Handler:           httpapi.SetupRouter(),
		ReadHeaderTimeout: 5 * time.Second,
	}
	go func() {
		log.Printf("http listening on :%s", cfg.Port)
		if err := httpSrv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Fatalf("http server: %v", err)
		}
	}()

	// gRPC: ticket API.
	grpcSrv := grpc.NewServer()
	ticketv1.RegisterTicketServiceServer(grpcSrv, grpcapi.NewTicketServer(ticket.NewRepository(pool)))
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
	if err := httpSrv.Shutdown(shutdownCtx); err != nil {
		log.Printf("http shutdown: %v", err)
	}
	stopped := make(chan struct{})
	go func() { grpcSrv.GracefulStop(); close(stopped) }()
	select {
	case <-stopped:
	case <-shutdownCtx.Done():
		grpcSrv.Stop()
	}
}
