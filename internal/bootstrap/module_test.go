package bootstrap

import (
	"context"
	"errors"
	"io/fs"
	"net"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/jackc/puddle/v2"
	"go.uber.org/fx"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/health"
	healthpb "google.golang.org/grpc/health/grpc_health_v1"
	"google.golang.org/grpc/status"
	"google.golang.org/grpc/test/bufconn"

	ticketv1 "github.com/HuyDoan-2112/handle-massive-requests/gen/ticket/v1"
	"github.com/HuyDoan-2112/handle-massive-requests/internal/config"
)

func TestModuleReturnsConfigError(t *testing.T) {
	t.Chdir(t.TempDir())
	app := fx.New(Module, fx.NopLogger)
	if err := app.Err(); !errors.Is(err, fs.ErrNotExist) {
		t.Fatalf("initialization error = %v, want missing config file", err)
	}
}

func TestModuleClosesPoolOnFailedStart(t *testing.T) {
	var pool *pgxpool.Pool
	app := fx.New(Module, fx.NopLogger,
		fx.Replace(&config.Config{
			PostgresURL: "postgres://test:test@127.0.0.1:1/test?sslmode=disable",
			GRPCPort:    "0",
		}),
		fx.Populate(&pool))
	if err := app.Err(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)
	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()
	if err := app.Start(ctx); err == nil {
		t.Fatal("startup succeeded with an unreachable database")
	}
	if _, err := pool.Acquire(ctx); !errors.Is(err, puddle.ErrClosedPool) {
		t.Fatalf("Acquire() error = %v, want closed pool after startup failure", err)
	}
}

func TestModuleServesAndDrainsRPCs(t *testing.T) {
	// Replace only PostgreSQL; exercise the real Fx graph and gRPC handlers.
	pool, err := pgxpool.New(t.Context(), "postgres://test:test@127.0.0.1:1/test?sslmode=disable")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)
	var server *grpc.Server
	var healthServer *health.Server
	app := fx.New(Module, fx.NopLogger,
		fx.Replace(&config.Config{GRPCPort: "0"}, pool),
		fx.Populate(&server, &healthServer))
	if err := app.Err(); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()
	if err := app.Start(ctx); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(server.Stop)
	lis := bufconn.Listen(1 << 20)
	t.Cleanup(func() { lis.Close() })
	observed := &closeNotifyListener{Listener: lis, closed: make(chan struct{})}
	go server.Serve(observed)
	conn, err := grpc.NewClient("passthrough:///bufnet",
		grpc.WithContextDialer(func(ctx context.Context, _ string) (net.Conn, error) {
			return lis.DialContext(ctx)
		}),
		grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { conn.Close() })
	if _, err := ticketv1.NewTicketServiceClient(conn).CreateTicket(ctx, &ticketv1.CreateTicketRequest{}); status.Code(err) != codes.InvalidArgument {
		t.Fatalf("invalid ticket error = %v, want InvalidArgument", err)
	}
	streamCtx, streamCancel := context.WithCancel(ctx)
	defer streamCancel()
	stream, err := healthpb.NewHealthClient(conn).Watch(streamCtx, &healthpb.HealthCheckRequest{})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := stream.Recv(); err != nil {
		t.Fatal(err)
	}
	done := make(chan error, 1)
	go func() { done <- app.Stop(ctx) }()
	select {
	case <-observed.closed:
	case <-ctx.Done():
		t.Fatal("shutdown did not close the listener")
	}
	healthServer.SetServingStatus("", healthpb.HealthCheckResponse_NOT_SERVING)
	response, err := stream.Recv()
	if err != nil {
		t.Fatalf("active RPC was interrupted during shutdown: %v", err)
	}
	if response.GetStatus() != healthpb.HealthCheckResponse_NOT_SERVING {
		t.Fatalf("health status = %v, want NOT_SERVING", response.GetStatus())
	}
	streamCancel()
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("shutdown error = %v", err)
		}
	case <-ctx.Done():
		t.Fatal("shutdown did not finish after the active RPC ended")
	}
}

type closeNotifyListener struct {
	net.Listener
	closed chan struct{}
	once   sync.Once
}

func (l *closeNotifyListener) Close() error {
	err := l.Listener.Close()
	l.once.Do(func() { close(l.closed) })
	return err
}
