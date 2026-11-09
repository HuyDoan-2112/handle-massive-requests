package grpcapi

import (
	"time"

	"google.golang.org/protobuf/types/known/timestamppb"

	ticketv1 "github.com/HuyDoan-2112/handle-massive-requests/gen/ticket/v1"
	"github.com/HuyDoan-2112/handle-massive-requests/internal/ticket"
)

var typeFromProto = map[ticketv1.TicketType]ticket.Type{
	ticketv1.TicketType_TICKET_TYPE_BASIC_ECONOMY:   ticket.TypeBasicEconomy,
	ticketv1.TicketType_TICKET_TYPE_ECONOMY:         ticket.TypeEconomy,
	ticketv1.TicketType_TICKET_TYPE_PREMIUM_ECONOMY: ticket.TypePremiumEconomy,
	ticketv1.TicketType_TICKET_TYPE_BUSINESS:        ticket.TypeBusiness,
	ticketv1.TicketType_TICKET_TYPE_FIRST:           ticket.TypeFirst,
}

var statusFromProto = map[ticketv1.TicketStatus]ticket.Status{
	ticketv1.TicketStatus_TICKET_STATUS_AVAILABLE: ticket.StatusAvailable,
	ticketv1.TicketStatus_TICKET_STATUS_RESERVED:  ticket.StatusReserved,
	ticketv1.TicketStatus_TICKET_STATUS_SOLD:      ticket.StatusSold,
	ticketv1.TicketStatus_TICKET_STATUS_CANCELLED: ticket.StatusCancelled,
}

var (
	typeToProto   = invert(typeFromProto)
	statusToProto = invert(statusFromProto)
)

func toProto(t ticket.Ticket) *ticketv1.Ticket {
	return &ticketv1.Ticket{
		Id:                t.ID.String(),
		Name:              t.Name,
		Status:            statusToProto[t.Status],
		DepartureLocation: t.DepartureLocation,
		Destination:       t.Destination,
		PriceCents:        t.PriceCents,
		TicketType:        typeToProto[t.Type],
		TimeStart:         timestamppb.New(t.TimeStart),
		TimeEnd:           timestamppb.New(t.TimeEnd),
		CreateTime:        timestamppb.New(t.CreatedAt),
		UpdateTime:        timestamppb.New(t.UpdatedAt),
	}
}

// timeFromProto returns the zero time for an unset timestamp.
func timeFromProto(ts *timestamppb.Timestamp) time.Time {
	if ts == nil {
		return time.Time{}
	}
	return ts.AsTime()
}

func invert[K, V comparable](m map[K]V) map[V]K {
	out := make(map[V]K, len(m))
	for k, v := range m {
		out[v] = k
	}
	return out
}
