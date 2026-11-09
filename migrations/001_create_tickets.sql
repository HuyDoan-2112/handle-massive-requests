-- 001_create_tickets.sql
-- Runs automatically on the FIRST start of the postgres container (empty volume only).
-- Columns mirror internal/ticket/ticket.go.

CREATE TABLE IF NOT EXISTS tickets (
    id                 uuid          PRIMARY KEY DEFAULT gen_random_uuid(),
    name               text          NOT NULL,
    status             text          NOT NULL DEFAULT 'available'
                                     CHECK (status IN ('available', 'reserved', 'sold', 'cancelled')),
    departure_location text          NOT NULL,
    destination        text          NOT NULL,
    price              numeric(12,2) NOT NULL CHECK (price >= 0),
    ticket_type        text          NOT NULL
                                     CHECK (ticket_type IN ('BasicEconomy', 'Economy', 'PremiumEconomy', 'Business', 'First')),
    time_start         timestamptz   NOT NULL,
    time_end           timestamptz   NOT NULL,
    created_at         timestamptz   NOT NULL DEFAULT now(),
    updated_at         timestamptz   NOT NULL DEFAULT now(),

    CHECK (time_end > time_start)
);

-- "Find tickets from A to B leaving around time T" is the main search query.
CREATE INDEX IF NOT EXISTS idx_tickets_route_time
    ON tickets (departure_location, destination, time_start);
