package ticket

import (
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
)

type Type string

const (
	TypeBasicEconomy   Type = "BasicEconomy"
	TypeEconomy        Type = "Economy"
	TypePremiumEconomy Type = "PremiumEconomy"
	TypeBusiness       Type = "Business"
	TypeFirst          Type = "First"
)

type Status string

const (
	StatusAvailable Status = "available"
	StatusReserved  Status = "reserved"
	StatusSold      Status = "sold"
	StatusCancelled Status = "cancelled"
)

var (
	ErrNotFound          = errors.New("ticket not found")
	ErrInvalid           = errors.New("invalid ticket")
	ErrInvalidTransition = errors.New("invalid status transition")
)

type Ticket struct {
	ID                uuid.UUID
	Name              string
	Status            Status
	DepartureLocation string
	Destination       string
	PriceCents        int64
	Type              Type
	TimeStart         time.Time
	TimeEnd           time.Time
	CreatedAt         time.Time
	UpdatedAt         time.Time
}

// Validate checks the fields a caller supplies on create.
func (t Ticket) Validate() error {
	switch {
	case t.Name == "":
		return fmt.Errorf("%w: name is required", ErrInvalid)
	case t.DepartureLocation == "":
		return fmt.Errorf("%w: departure_location is required", ErrInvalid)
	case t.Destination == "":
		return fmt.Errorf("%w: destination is required", ErrInvalid)
	case t.PriceCents < 0:
		return fmt.Errorf("%w: price_cents must be >= 0", ErrInvalid)
	case t.Type == "":
		return fmt.Errorf("%w: ticket_type is required", ErrInvalid)
	case t.TimeStart.IsZero() || t.TimeEnd.IsZero():
		return fmt.Errorf("%w: time_start and time_end are required", ErrInvalid)
	case !t.TimeEnd.After(t.TimeStart):
		return fmt.Errorf("%w: time_end must be after time_start", ErrInvalid)
	}
	return nil
}

// AllowedFrom returns the statuses a ticket may move to `to` from.
func AllowedFrom(to Status) []Status {
	switch to {
	case StatusReserved:
		return []Status{StatusAvailable}
	case StatusSold:
		return []Status{StatusReserved}
	case StatusAvailable:
		return []Status{StatusReserved} // reservation released
	case StatusCancelled:
		return []Status{StatusAvailable, StatusReserved}
	}
	return nil
}

// ListFilter is a route search. Cursor fields are set from a page token.
type ListFilter struct {
	DepartureLocation string
	Destination       string
	TimeStartFrom     time.Time // zero = no lower bound
	TimeStartTo       time.Time // zero = no upper bound
	Limit             int
	AfterTimeStart    time.Time // keyset cursor
	AfterID           uuid.UUID
}
