package ticket

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
)

type stubStore struct {
	create       func(context.Context, Ticket) (Ticket, error)
	get          func(context.Context, uuid.UUID) (Ticket, error)
	list         func(context.Context, ListFilter) ([]Ticket, error)
	updateStatus func(context.Context, uuid.UUID, Status) (Ticket, error)
	delete       func(context.Context, uuid.UUID) error
}

func (s stubStore) Create(ctx context.Context, t Ticket) (Ticket, error) {
	return s.create(ctx, t)
}
func (s stubStore) Get(ctx context.Context, id uuid.UUID) (Ticket, error) {
	return s.get(ctx, id)
}
func (s stubStore) List(ctx context.Context, f ListFilter) ([]Ticket, error) {
	return s.list(ctx, f)
}
func (s stubStore) UpdateStatus(ctx context.Context, id uuid.UUID, to Status) (Ticket, error) {
	return s.updateStatus(ctx, id, to)
}
func (s stubStore) Delete(ctx context.Context, id uuid.UUID) error {
	return s.delete(ctx, id)
}

func validTicket() Ticket {
	start := time.Date(2026, 12, 1, 8, 0, 0, 0, time.UTC)
	return Ticket{
		Name: "SE1", DepartureLocation: "Hanoi", Destination: "Saigon",
		PriceCents: 125000, Type: TypeEconomy,
		TimeStart: start, TimeEnd: start.Add(30 * time.Hour),
	}
}

func TestServiceCreateRejectsInvalidTickets(t *testing.T) {
	cases := map[string]func(*Ticket){
		"missing name":        func(t *Ticket) { t.Name = "" },
		"missing departure":   func(t *Ticket) { t.DepartureLocation = "" },
		"missing destination": func(t *Ticket) { t.Destination = "" },
		"negative price":      func(t *Ticket) { t.PriceCents = -1 },
		"missing type":        func(t *Ticket) { t.Type = "" },
		"unknown type":        func(t *Ticket) { t.Type = "Luxury" },
		"missing start":       func(t *Ticket) { t.TimeStart = time.Time{} },
		"missing end":         func(t *Ticket) { t.TimeEnd = time.Time{} },
		"end before start":    func(t *Ticket) { t.TimeEnd = t.TimeStart.Add(-time.Hour) },
		"equal times":         func(t *Ticket) { t.TimeEnd = t.TimeStart },
	}
	for name, mutate := range cases {
		t.Run(name, func(t *testing.T) {
			store := stubStore{create: func(context.Context, Ticket) (Ticket, error) {
				t.Fatal("invalid ticket reached storage")
				return Ticket{}, nil
			}}
			in := validTicket()
			mutate(&in)
			if _, err := NewService(store).Create(context.Background(), in); !errors.Is(err, ErrInvalid) {
				t.Fatalf("error = %v, want ErrInvalid", err)
			}
		})
	}
}

func TestServiceCreate(t *testing.T) {
	for _, typ := range []Type{TypeBasicEconomy, TypeEconomy, TypePremiumEconomy, TypeBusiness, TypeFirst} {
		t.Run(string(typ), func(t *testing.T) {
			in := validTicket()
			in.Type, in.PriceCents = typ, 0
			want := in
			want.ID, want.Status = uuid.New(), StatusAvailable
			store := stubStore{create: func(_ context.Context, got Ticket) (Ticket, error) {
				if got != in {
					t.Fatalf("stored ticket = %+v, want %+v", got, in)
				}
				return want, nil
			}}
			got, err := NewService(store).Create(context.Background(), in)
			if err != nil || got != want {
				t.Fatalf("Create = %+v, %v; want %+v, nil", got, err, want)
			}
		})
	}
}

func TestServiceListRejectsInvalidFilters(t *testing.T) {
	for _, filter := range []ListFilter{
		{Destination: "Saigon", Limit: 10},
		{DepartureLocation: "Hanoi", Limit: 10},
		{DepartureLocation: "Hanoi", Destination: "Saigon", Limit: 0},
		{DepartureLocation: "Hanoi", Destination: "Saigon", Limit: -1},
	} {
		store := stubStore{list: func(context.Context, ListFilter) ([]Ticket, error) {
			t.Fatal("invalid filter reached storage")
			return nil, nil
		}}
		if _, err := NewService(store).List(context.Background(), filter); !errors.Is(err, ErrInvalid) {
			t.Fatalf("List(%+v) error = %v, want ErrInvalid", filter, err)
		}
	}
}

func TestServiceRejectsUnknownStatus(t *testing.T) {
	for _, to := range []Status{"", "unknown"} {
		store := stubStore{updateStatus: func(context.Context, uuid.UUID, Status) (Ticket, error) {
			t.Fatal("invalid status reached storage")
			return Ticket{}, nil
		}}
		if _, err := NewService(store).UpdateStatus(context.Background(), uuid.New(), to); !errors.Is(err, ErrInvalid) {
			t.Fatalf("UpdateStatus(%q) error = %v, want ErrInvalid", to, err)
		}
	}
}

func TestServicePreservesStorageErrors(t *testing.T) {
	ctx := context.Background()
	id := uuid.New()
	for _, want := range []error{ErrNotFound, ErrInvalidTransition, context.Canceled, errors.New("database unavailable")} {
		store := stubStore{
			create:       func(context.Context, Ticket) (Ticket, error) { return Ticket{}, want },
			get:          func(context.Context, uuid.UUID) (Ticket, error) { return Ticket{}, want },
			list:         func(context.Context, ListFilter) ([]Ticket, error) { return nil, want },
			updateStatus: func(context.Context, uuid.UUID, Status) (Ticket, error) { return Ticket{}, want },
			delete:       func(context.Context, uuid.UUID) error { return want },
		}
		svc := NewService(store)
		_, createErr := svc.Create(ctx, validTicket())
		_, getErr := svc.Get(ctx, id)
		_, listErr := svc.List(ctx, ListFilter{DepartureLocation: "Hanoi", Destination: "Saigon", Limit: 10})
		_, updateErr := svc.UpdateStatus(ctx, id, StatusReserved)
		deleteErr := svc.Delete(ctx, id)
		for name, err := range map[string]error{"create": createErr, "get": getErr, "list": listErr, "update": updateErr, "delete": deleteErr} {
			if !errors.Is(err, want) {
				t.Fatalf("%s error = %v, want %v", name, err, want)
			}
		}
	}
}
