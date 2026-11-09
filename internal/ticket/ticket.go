package ticket

import (
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


type Ticket struct {
	ID                uuid.UUID `json:"id"`
	Name              string    `json:"name"`
	Status            string    `json:"status"`
	DepartureLocation string    `json:"departure_location"`
	Destination       string    `json:"destination"`
	Price             float64   `json:"price"`
	Type              Type      `json:"ticket_type"`
	TimeStart         time.Time `json:"time_start"`
	TimeEnd           time.Time `json:"time_end"`
	CreatedAt         time.Time `json:"created_at"`
	UpdatedAt         time.Time `json:"updated_at"`
}
