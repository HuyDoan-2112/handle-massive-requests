package ticket

import (
	"errors"
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
	Name              string `validate:"required"`
	Status            Status
	DepartureLocation string    `validate:"required"`
	Destination       string    `validate:"required"`
	PriceCents        int64     `validate:"gte=0"`
	Type              Type      `validate:"required,oneof=BasicEconomy Economy PremiumEconomy Business First"`
	TimeStart         time.Time `validate:"required"`
	TimeEnd           time.Time `validate:"required,gtfield=TimeStart"`
	CreatedAt         time.Time
	UpdatedAt         time.Time
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
	DepartureLocation string    `validate:"required"`
	Destination       string    `validate:"required"`
	TimeStartFrom     time.Time // zero = no lower bound
	TimeStartTo       time.Time // zero = no upper bound
	Limit             int       `validate:"gt=0"`
	AfterTimeStart    time.Time // keyset cursor
	AfterID           uuid.UUID
}
