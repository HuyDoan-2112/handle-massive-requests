package ticket

import (
	"context"
	"fmt"

	"github.com/go-playground/validator/v10"
	"github.com/google/uuid"
)

var validate = validator.New(validator.WithRequiredStructEnabled())

// Store is the persistence required by Service. Repository implements it.
type Store interface {
	Create(context.Context, Ticket) (Ticket, error)
	Get(context.Context, uuid.UUID) (Ticket, error)
	List(context.Context, ListFilter) ([]Ticket, error)
	UpdateStatus(context.Context, uuid.UUID, Status) (Ticket, error)
	Delete(context.Context, uuid.UUID) error
}

// Service coordinates ticket operations independently of the transport and database.
type Service struct {
	store Store
}

func NewService(store Store) *Service {
	return &Service{store: store}
}

func (s *Service) Create(ctx context.Context, t Ticket) (Ticket, error) {
	if err := validate.Struct(t); err != nil {
		return Ticket{}, fmt.Errorf("%w: %v", ErrInvalid, err)
	}
	return s.store.Create(ctx, t)
}

func (s *Service) Get(ctx context.Context, id uuid.UUID) (Ticket, error) {
	return s.store.Get(ctx, id)
}

func (s *Service) List(ctx context.Context, f ListFilter) ([]Ticket, error) {
	if err := validate.Struct(f); err != nil {
		return nil, fmt.Errorf("%w: %v", ErrInvalid, err)
	}
	return s.store.List(ctx, f)
}

func (s *Service) UpdateStatus(ctx context.Context, id uuid.UUID, to Status) (Ticket, error) {
	if len(AllowedFrom(to)) == 0 {
		return Ticket{}, fmt.Errorf("%w: unsupported ticket status %q", ErrInvalid, to)
	}
	// Storage checks the current status and updates it atomically.
	return s.store.UpdateStatus(ctx, id, to)
}

func (s *Service) Delete(ctx context.Context, id uuid.UUID) error {
	return s.store.Delete(ctx, id)
}
