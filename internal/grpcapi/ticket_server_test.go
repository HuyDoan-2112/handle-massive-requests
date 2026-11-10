package grpcapi

import (
	"context"
	"net"
	"sort"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/status"
	"google.golang.org/grpc/test/bufconn"
	"google.golang.org/protobuf/types/known/timestamppb"

	ticketv1 "github.com/HuyDoan-2112/handle-massive-requests/gen/ticket/v1"
	"github.com/HuyDoan-2112/handle-massive-requests/internal/ticket"
)

// fakeStore supplies in-memory storage for gRPC tests through the real service.
type fakeStore struct {
	mu      sync.Mutex
	tickets map[uuid.UUID]ticket.Ticket
}

func (f *fakeStore) Create(_ context.Context, t ticket.Ticket) (ticket.Ticket, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	t.ID = uuid.New()
	t.Status = ticket.StatusAvailable
	t.CreatedAt, t.UpdatedAt = time.Now(), time.Now()
	f.tickets[t.ID] = t
	return t, nil
}

func (f *fakeStore) Get(_ context.Context, id uuid.UUID) (ticket.Ticket, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	t, ok := f.tickets[id]
	if !ok {
		return ticket.Ticket{}, ticket.ErrNotFound
	}
	return t, nil
}

func (f *fakeStore) List(_ context.Context, lf ticket.ListFilter) ([]ticket.Ticket, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	var out []ticket.Ticket
	for _, t := range f.tickets {
		if t.DepartureLocation != lf.DepartureLocation || t.Destination != lf.Destination {
			continue
		}
		if !lf.AfterTimeStart.IsZero() && !(t.TimeStart.After(lf.AfterTimeStart) ||
			(t.TimeStart.Equal(lf.AfterTimeStart) && t.ID.String() > lf.AfterID.String())) {
			continue
		}
		out = append(out, t)
	}
	sort.Slice(out, func(i, j int) bool {
		if !out[i].TimeStart.Equal(out[j].TimeStart) {
			return out[i].TimeStart.Before(out[j].TimeStart)
		}
		return out[i].ID.String() < out[j].ID.String()
	})
	if len(out) > lf.Limit {
		out = out[:lf.Limit]
	}
	return out, nil
}

func (f *fakeStore) UpdateStatus(_ context.Context, id uuid.UUID, to ticket.Status) (ticket.Ticket, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	t, ok := f.tickets[id]
	if !ok {
		return ticket.Ticket{}, ticket.ErrNotFound
	}
	for _, from := range ticket.AllowedFrom(to) {
		if t.Status == from {
			t.Status = to
			f.tickets[id] = t
			return t, nil
		}
	}
	return ticket.Ticket{}, ticket.ErrInvalidTransition
}

func (f *fakeStore) Delete(_ context.Context, id uuid.UUID) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if _, ok := f.tickets[id]; !ok {
		return ticket.ErrNotFound
	}
	delete(f.tickets, id)
	return nil
}

// newClient starts a real gRPC server over an in-memory listener.
func newClient(t *testing.T) ticketv1.TicketServiceClient {
	t.Helper()
	lis := bufconn.Listen(1 << 20)
	srv := grpc.NewServer()
	store := &fakeStore{tickets: map[uuid.UUID]ticket.Ticket{}}
	ticketv1.RegisterTicketServiceServer(srv, NewTicketServer(ticket.NewService(store)))
	go srv.Serve(lis)
	t.Cleanup(srv.Stop)

	conn, err := grpc.NewClient("passthrough:///bufnet",
		grpc.WithContextDialer(func(ctx context.Context, _ string) (net.Conn, error) { return lis.DialContext(ctx) }),
		grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	t.Cleanup(func() { conn.Close() })
	return ticketv1.NewTicketServiceClient(conn)
}

func validCreate(start time.Time) *ticketv1.CreateTicketRequest {
	return &ticketv1.CreateTicketRequest{
		Name:              "SE1",
		DepartureLocation: "Hanoi",
		Destination:       "Saigon",
		PriceCents:        125000,
		TicketType:        ticketv1.TicketType_TICKET_TYPE_ECONOMY,
		TimeStart:         timestamppb.New(start),
		TimeEnd:           timestamppb.New(start.Add(30 * time.Hour)),
	}
}

func wantCode(t *testing.T, err error, want codes.Code) {
	t.Helper()
	if got := status.Code(err); got != want {
		t.Fatalf("code = %v, want %v (err: %v)", got, want, err)
	}
}

func TestCreateAndGet(t *testing.T) {
	c := newClient(t)
	ctx := context.Background()

	created, err := c.CreateTicket(ctx, validCreate(time.Now()))
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if created.GetTicket().GetStatus() != ticketv1.TicketStatus_TICKET_STATUS_AVAILABLE {
		t.Fatalf("status = %v, want AVAILABLE", created.GetTicket().GetStatus())
	}

	got, err := c.GetTicket(ctx, &ticketv1.GetTicketRequest{Id: created.GetTicket().GetId()})
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if got.GetTicket().GetPriceCents() != 125000 || got.GetTicket().GetTicketType() != ticketv1.TicketType_TICKET_TYPE_ECONOMY {
		t.Fatalf("unexpected ticket: %v", got.GetTicket())
	}
}

func TestCreateValidation(t *testing.T) {
	c := newClient(t)
	now := time.Now()
	cases := map[string]func(*ticketv1.CreateTicketRequest){
		"missing name":   func(r *ticketv1.CreateTicketRequest) { r.Name = "" },
		"negative price": func(r *ticketv1.CreateTicketRequest) { r.PriceCents = -1 },
		"no type":        func(r *ticketv1.CreateTicketRequest) { r.TicketType = 0 },
		"end before start": func(r *ticketv1.CreateTicketRequest) {
			r.TimeEnd = timestamppb.New(now.Add(-time.Hour))
		},
	}
	for name, mutate := range cases {
		t.Run(name, func(t *testing.T) {
			req := validCreate(now)
			mutate(req)
			_, err := c.CreateTicket(context.Background(), req)
			wantCode(t, err, codes.InvalidArgument)
		})
	}
}

func TestGetErrors(t *testing.T) {
	c := newClient(t)
	_, err := c.GetTicket(context.Background(), &ticketv1.GetTicketRequest{Id: "not-a-uuid"})
	wantCode(t, err, codes.InvalidArgument)
	_, err = c.GetTicket(context.Background(), &ticketv1.GetTicketRequest{Id: uuid.NewString()})
	wantCode(t, err, codes.NotFound)
}

func TestListValidation(t *testing.T) {
	c := newClient(t)
	for name, req := range map[string]*ticketv1.ListTicketsRequest{
		"missing departure":   {Destination: "Saigon"},
		"missing destination": {DepartureLocation: "Hanoi"},
		"negative page size":  {DepartureLocation: "Hanoi", Destination: "Saigon", PageSize: -1},
		"invalid page token":  {DepartureLocation: "Hanoi", Destination: "Saigon", PageToken: "!"},
	} {
		t.Run(name, func(t *testing.T) {
			_, err := c.ListTickets(context.Background(), req)
			wantCode(t, err, codes.InvalidArgument)
		})
	}
}

func TestListPagination(t *testing.T) {
	c := newClient(t)
	ctx := context.Background()
	base := time.Date(2026, 12, 1, 8, 0, 0, 0, time.UTC)
	for i := range 5 {
		if _, err := c.CreateTicket(ctx, validCreate(base.Add(time.Duration(i)*time.Hour))); err != nil {
			t.Fatalf("create: %v", err)
		}
	}

	var seen []time.Time
	token := ""
	for pages := 0; ; pages++ {
		resp, err := c.ListTickets(ctx, &ticketv1.ListTicketsRequest{
			DepartureLocation: "Hanoi", Destination: "Saigon", PageSize: 2, PageToken: token,
		})
		if err != nil {
			t.Fatalf("list: %v", err)
		}
		for _, tk := range resp.GetTickets() {
			seen = append(seen, tk.GetTimeStart().AsTime())
		}
		token = resp.GetNextPageToken()
		if token == "" {
			if pages != 2 {
				t.Fatalf("pages = %d, want 3", pages+1)
			}
			break
		}
	}
	if len(seen) != 5 {
		t.Fatalf("got %d tickets, want 5", len(seen))
	}
	for i := 1; i < len(seen); i++ {
		if !seen[i].After(seen[i-1]) {
			t.Fatalf("not sorted by time_start: %v", seen)
		}
	}

	_, err := c.ListTickets(ctx, &ticketv1.ListTicketsRequest{DepartureLocation: "Hanoi", Destination: "Saigon", PageToken: "garbage!"})
	wantCode(t, err, codes.InvalidArgument)
	_, err = c.ListTickets(ctx, &ticketv1.ListTicketsRequest{DepartureLocation: "Hanoi"})
	wantCode(t, err, codes.InvalidArgument)
}

func TestStatusTransitions(t *testing.T) {
	c := newClient(t)
	ctx := context.Background()
	created, err := c.CreateTicket(ctx, validCreate(time.Now()))
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	id := created.GetTicket().GetId()
	set := func(s ticketv1.TicketStatus) error {
		_, err := c.UpdateTicketStatus(ctx, &ticketv1.UpdateTicketStatusRequest{Id: id, Status: s})
		return err
	}

	wantCode(t, set(ticketv1.TicketStatus_TICKET_STATUS_SOLD), codes.FailedPrecondition) // must reserve first
	wantCode(t, set(ticketv1.TicketStatus_TICKET_STATUS_RESERVED), codes.OK)
	wantCode(t, set(ticketv1.TicketStatus_TICKET_STATUS_RESERVED), codes.FailedPrecondition) // already reserved
	wantCode(t, set(ticketv1.TicketStatus_TICKET_STATUS_SOLD), codes.OK)
	wantCode(t, set(ticketv1.TicketStatus_TICKET_STATUS_UNSPECIFIED), codes.InvalidArgument)
}

func TestDelete(t *testing.T) {
	c := newClient(t)
	ctx := context.Background()
	created, err := c.CreateTicket(ctx, validCreate(time.Now()))
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	id := created.GetTicket().GetId()
	_, err = c.DeleteTicket(ctx, &ticketv1.DeleteTicketRequest{Id: id})
	wantCode(t, err, codes.OK)
	_, err = c.DeleteTicket(ctx, &ticketv1.DeleteTicketRequest{Id: id})
	wantCode(t, err, codes.NotFound)
}
