package grpcapi

import (
	"context"

	"github.com/google/uuid"

	"github.com/HuyDoan-2112/handle-massive-requests/internal/ticket"
)

type ticketService interface {
	Create(ctx context.Context, t ticket.Ticket) (ticket.Ticket, error)
	Get(ctx context.Context, id uuid.UUID) (ticket.Ticket, error)
	List(ctx context.Context, f ticket.ListFilter) ([]ticket.Ticket, error)
	UpdateStatus(ctx context.Context, id uuid.UUID, to ticket.Status) (ticket.Ticket, error)
	Delete(ctx context.Context, id uuid.UUID) error
}
