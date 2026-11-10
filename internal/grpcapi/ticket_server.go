package grpcapi

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/google/uuid"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	ticketv1 "github.com/HuyDoan-2112/handle-massive-requests/gen/ticket/v1"
	"github.com/HuyDoan-2112/handle-massive-requests/internal/ticket"
)

const (
	defaultPageSize = 50
	maxPageSize     = 500
)

// ticketService is the application interface consumed by the gRPC handlers.
type ticketService interface {
	Create(ctx context.Context, t ticket.Ticket) (ticket.Ticket, error)
	Get(ctx context.Context, id uuid.UUID) (ticket.Ticket, error)
	List(ctx context.Context, f ticket.ListFilter) ([]ticket.Ticket, error)
	UpdateStatus(ctx context.Context, id uuid.UUID, to ticket.Status) (ticket.Ticket, error)
	Delete(ctx context.Context, id uuid.UUID) error
}

type TicketServer struct {
	ticketv1.UnimplementedTicketServiceServer
	service ticketService
}

func NewTicketServer(service ticketService) *TicketServer {
	return &TicketServer{service: service}
}

func (s *TicketServer) CreateTicket(ctx context.Context, req *ticketv1.CreateTicketRequest) (*ticketv1.CreateTicketResponse, error) {
	typ, ok := typeFromProto[req.GetTicketType()]
	if !ok {
		return nil, status.Error(codes.InvalidArgument, "ticket_type is required")
	}
	in := ticket.Ticket{
		Name:              req.GetName(),
		DepartureLocation: req.GetDepartureLocation(),
		Destination:       req.GetDestination(),
		PriceCents:        req.GetPriceCents(),
		Type:              typ,
		TimeStart:         timeFromProto(req.GetTimeStart()),
		TimeEnd:           timeFromProto(req.GetTimeEnd()),
	}
	t, err := s.service.Create(ctx, in)
	if err != nil {
		return nil, toStatus(err)
	}
	return &ticketv1.CreateTicketResponse{Ticket: toProto(t)}, nil
}

func (s *TicketServer) GetTicket(ctx context.Context, req *ticketv1.GetTicketRequest) (*ticketv1.GetTicketResponse, error) {
	id, err := parseID(req.GetId())
	if err != nil {
		return nil, err
	}
	t, err := s.service.Get(ctx, id)
	if err != nil {
		return nil, toStatus(err)
	}
	return &ticketv1.GetTicketResponse{Ticket: toProto(t)}, nil
}

func (s *TicketServer) ListTickets(ctx context.Context, req *ticketv1.ListTicketsRequest) (*ticketv1.ListTicketsResponse, error) {
	size := int(req.GetPageSize())
	switch {
	case size < 0:
		return nil, status.Error(codes.InvalidArgument, "page_size must be >= 0")
	case size == 0:
		size = defaultPageSize
	case size > maxPageSize:
		size = maxPageSize
	}

	f := ticket.ListFilter{
		DepartureLocation: req.GetDepartureLocation(),
		Destination:       req.GetDestination(),
		TimeStartFrom:     timeFromProto(req.GetTimeStartFrom()),
		TimeStartTo:       timeFromProto(req.GetTimeStartTo()),
		Limit:             size + 1, // fetch one extra to know if another page exists
	}
	if tok := req.GetPageToken(); tok != "" {
		var err error
		if f.AfterTimeStart, f.AfterID, err = decodePageToken(tok); err != nil {
			return nil, status.Error(codes.InvalidArgument, "invalid page_token")
		}
	}

	ts, err := s.service.List(ctx, f)
	if err != nil {
		return nil, toStatus(err)
	}
	resp := &ticketv1.ListTicketsResponse{}
	if len(ts) > size {
		ts = ts[:size]
		last := ts[len(ts)-1]
		resp.NextPageToken = encodePageToken(last.TimeStart, last.ID)
	}
	for _, t := range ts {
		resp.Tickets = append(resp.Tickets, toProto(t))
	}
	return resp, nil
}

func (s *TicketServer) UpdateTicketStatus(ctx context.Context, req *ticketv1.UpdateTicketStatusRequest) (*ticketv1.UpdateTicketStatusResponse, error) {
	id, err := parseID(req.GetId())
	if err != nil {
		return nil, err
	}
	to, ok := statusFromProto[req.GetStatus()]
	if !ok {
		return nil, status.Error(codes.InvalidArgument, "status is required")
	}
	t, err := s.service.UpdateStatus(ctx, id, to)
	if err != nil {
		return nil, toStatus(err)
	}
	return &ticketv1.UpdateTicketStatusResponse{Ticket: toProto(t)}, nil
}

func (s *TicketServer) DeleteTicket(ctx context.Context, req *ticketv1.DeleteTicketRequest) (*ticketv1.DeleteTicketResponse, error) {
	id, err := parseID(req.GetId())
	if err != nil {
		return nil, err
	}
	if err := s.service.Delete(ctx, id); err != nil {
		return nil, toStatus(err)
	}
	return &ticketv1.DeleteTicketResponse{}, nil
}

// toStatus maps domain errors to gRPC codes. Unknown errors are logged
// and returned as Internal so DB details never leak to clients.
func toStatus(err error) error {
	switch {
	case errors.Is(err, ticket.ErrNotFound):
		return status.Error(codes.NotFound, err.Error())
	case errors.Is(err, ticket.ErrInvalid):
		return status.Error(codes.InvalidArgument, err.Error())
	case errors.Is(err, ticket.ErrInvalidTransition):
		return status.Error(codes.FailedPrecondition, err.Error())
	case errors.Is(err, context.Canceled):
		return status.Error(codes.Canceled, err.Error())
	case errors.Is(err, context.DeadlineExceeded):
		return status.Error(codes.DeadlineExceeded, err.Error())
	}
	log.Printf("internal error: %v", err)
	return status.Error(codes.Internal, "internal error")
}

func parseID(s string) (uuid.UUID, error) {
	id, err := uuid.Parse(s)
	if err != nil {
		return uuid.Nil, status.Error(codes.InvalidArgument, "id must be a UUID")
	}
	return id, nil
}

func encodePageToken(t time.Time, id uuid.UUID) string {
	raw := fmt.Sprintf("%d|%s", t.UnixNano(), id)
	return base64.RawURLEncoding.EncodeToString([]byte(raw))
}

func decodePageToken(tok string) (time.Time, uuid.UUID, error) {
	raw, err := base64.RawURLEncoding.DecodeString(tok)
	if err != nil {
		return time.Time{}, uuid.Nil, err
	}
	nanos, idStr, ok := strings.Cut(string(raw), "|")
	if !ok {
		return time.Time{}, uuid.Nil, errors.New("malformed token")
	}
	var n int64
	if _, err := fmt.Sscan(nanos, &n); err != nil {
		return time.Time{}, uuid.Nil, err
	}
	id, err := uuid.Parse(idStr)
	if err != nil {
		return time.Time{}, uuid.Nil, err
	}
	return time.Unix(0, n), id, nil
}
