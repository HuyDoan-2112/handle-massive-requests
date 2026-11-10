package ticket

import (
	"context"
	"errors"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Each test gets a temporary schema so it cannot modify application tables.
func repositoryForTest(t *testing.T) (*Repository, context.Context) {
	t.Helper()
	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("set TEST_DATABASE_URL to run PostgreSQL integration tests")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	t.Cleanup(cancel)
	admin, err := pgx.Connect(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { admin.Close(context.Background()) })
	schema := "ticket_test_" + strings.ReplaceAll(uuid.NewString(), "-", "")
	identifier := pgx.Identifier{schema}.Sanitize()
	if _, err := admin.Exec(ctx, "CREATE SCHEMA "+identifier); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		cleanupCtx, stop := context.WithTimeout(context.Background(), 5*time.Second)
		defer stop()
		if _, err := admin.Exec(cleanupCtx, "DROP SCHEMA "+identifier+" CASCADE"); err != nil {
			t.Errorf("remove test schema: %v", err)
		}
	})
	cfg, err := pgxpool.ParseConfig(dsn)
	if err != nil {
		t.Fatal(err)
	}
	cfg.ConnConfig.RuntimeParams["search_path"] = schema
	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)
	migration, err := os.ReadFile("../../migrations/001_create_tickets.sql")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, string(migration)); err != nil {
		t.Fatal(err)
	}
	return NewRepository(pool), ctx
}

func TestRepositoryCRUD(t *testing.T) {
	repo, ctx := repositoryForTest(t)
	in := validTicket()
	created, err := repo.Create(ctx, in)
	if err != nil {
		t.Fatal(err)
	}
	if created.ID == uuid.Nil || created.Status != StatusAvailable || created.CreatedAt.IsZero() || created.UpdatedAt.IsZero() {
		t.Fatalf("missing database defaults: %+v", created)
	}
	got, err := repo.Get(ctx, created.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.Name != in.Name || got.DepartureLocation != in.DepartureLocation || got.Destination != in.Destination ||
		got.PriceCents != in.PriceCents || got.Type != in.Type || !got.TimeStart.Equal(in.TimeStart) || !got.TimeEnd.Equal(in.TimeEnd) {
		t.Fatalf("ticket did not round-trip: %+v", got)
	}
	if err := repo.Delete(ctx, created.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := repo.Get(ctx, created.ID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("get deleted ticket: %v, want ErrNotFound", err)
	}
	if err := repo.Delete(ctx, created.ID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("delete missing ticket: %v, want ErrNotFound", err)
	}
}

func TestRepositoryListFiltersAndCursor(t *testing.T) {
	repo, ctx := repositoryForTest(t)
	base := validTicket()
	for i := range 4 {
		in := base
		if i == 2 {
			in.TimeStart = in.TimeStart.Add(time.Hour)
		}
		if i == 3 {
			in.Destination = "Danang"
		}
		if _, err := repo.Create(ctx, in); err != nil {
			t.Fatal(err)
		}
	}
	f := ListFilter{DepartureLocation: "Hanoi", Destination: "Saigon", Limit: 2}
	first, err := repo.List(ctx, f)
	if err != nil {
		t.Fatal(err)
	}
	if len(first) != 2 || !first[0].TimeStart.Equal(first[1].TimeStart) || first[0].ID.String() >= first[1].ID.String() {
		t.Fatalf("expected two tickets ordered by time and ID: %+v", first)
	}
	f.AfterTimeStart, f.AfterID = first[1].TimeStart, first[1].ID
	next, err := repo.List(ctx, f)
	if err != nil || len(next) != 1 || !next[0].TimeStart.Equal(base.TimeStart.Add(time.Hour)) {
		t.Fatalf("next page = %+v, %v", next, err)
	}
	f.AfterTimeStart, f.AfterID = time.Time{}, uuid.Nil
	f.TimeStartFrom, f.TimeStartTo = base.TimeStart, base.TimeStart.Add(time.Hour)
	window, err := repo.List(ctx, f)
	if err != nil || len(window) != 2 {
		t.Fatalf("inclusive lower/exclusive upper window = %+v, %v", window, err)
	}
	f.Destination = "missing"
	empty, err := repo.List(ctx, f)
	if err != nil || len(empty) != 0 {
		t.Fatalf("empty search = %+v, %v", empty, err)
	}
}

func TestRepositoryStatusTransitions(t *testing.T) {
	repo, ctx := repositoryForTest(t)
	created, err := repo.Create(ctx, validTicket())
	if err != nil {
		t.Fatal(err)
	}
	for _, to := range []Status{StatusReserved, StatusAvailable, StatusReserved, StatusSold} {
		got, err := repo.UpdateStatus(ctx, created.ID, to)
		if err != nil || got.Status != to {
			t.Fatalf("transition to %s = %+v, %v", to, got, err)
		}
	}
	if _, err := repo.UpdateStatus(ctx, created.ID, StatusCancelled); !errors.Is(err, ErrInvalidTransition) {
		t.Fatalf("cancel sold ticket: %v, want ErrInvalidTransition", err)
	}
	if _, err := repo.UpdateStatus(ctx, uuid.New(), StatusReserved); !errors.Is(err, ErrNotFound) {
		t.Fatalf("reserve missing ticket: %v, want ErrNotFound", err)
	}
}

func TestRepositoryConcurrentReservation(t *testing.T) {
	repo, ctx := repositoryForTest(t)
	created, err := repo.Create(ctx, validTicket())
	if err != nil {
		t.Fatal(err)
	}
	start := make(chan struct{})
	results := make(chan error, 2)
	for range 2 {
		go func() {
			<-start
			_, err := repo.UpdateStatus(ctx, created.ID, StatusReserved)
			results <- err
		}()
	}
	close(start)
	successes, conflicts := 0, 0
	for range 2 {
		err := <-results
		if err == nil {
			successes++
		} else if errors.Is(err, ErrInvalidTransition) {
			conflicts++
		} else {
			t.Errorf("unexpected reservation error: %v", err)
		}
	}
	if successes != 1 || conflicts != 1 {
		t.Fatalf("successes = %d, conflicts = %d; want one each", successes, conflicts)
	}
	got, err := repo.Get(ctx, created.ID)
	if err != nil || got.Status != StatusReserved {
		t.Fatalf("final ticket = %+v, %v", got, err)
	}
}

// Cancel only the diagnostic SELECT after an UPDATE returns no matching row.
type cancelLookupTracer struct{ cancel context.CancelFunc }

func (tr cancelLookupTracer) TraceQueryStart(ctx context.Context, _ *pgx.Conn, data pgx.TraceQueryStartData) context.Context {
	if strings.HasPrefix(data.SQL, "SELECT ") {
		tr.cancel()
	}
	return ctx
}

func (cancelLookupTracer) TraceQueryEnd(context.Context, *pgx.Conn, pgx.TraceQueryEndData) {}

func TestRepositoryStatusLookupPreservesFailure(t *testing.T) {
	repo, ctx := repositoryForTest(t)
	created, err := repo.Create(ctx, validTicket())
	if err != nil {
		t.Fatal(err)
	}
	lookupCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	cfg := repo.pool.Config()
	cfg.ConnConfig.Tracer = cancelLookupTracer{cancel: cancel}
	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	// Selling an available ticket fails the transition and triggers the lookup.
	_, err = NewRepository(pool).UpdateStatus(lookupCtx, created.ID, StatusSold)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("lookup error = %v, want context.Canceled", err)
	}
}
