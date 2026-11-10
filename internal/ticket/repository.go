package ticket

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

const columns = `id, name, status, departure_location, destination, price_cents,
	ticket_type, time_start, time_end, created_at, updated_at`

type Repository struct {
	pool *pgxpool.Pool
}

func NewRepository(pool *pgxpool.Pool) *Repository {
	return &Repository{pool: pool}
}

func (r *Repository) Create(ctx context.Context, t Ticket) (Ticket, error) {
	row := r.pool.QueryRow(ctx, `
		INSERT INTO tickets (name, departure_location, destination, price_cents, ticket_type, time_start, time_end)
		VALUES ($1, $2, $3, $4, $5, $6, $7)
		RETURNING `+columns,
		t.Name, t.DepartureLocation, t.Destination, t.PriceCents, string(t.Type), t.TimeStart, t.TimeEnd)
	created, err := scan(row)
	if err != nil {
		return Ticket{}, fmt.Errorf("create ticket: %w", err)
	}
	return created, nil
}

func (r *Repository) Get(ctx context.Context, id uuid.UUID) (Ticket, error) {
	row := r.pool.QueryRow(ctx, `SELECT `+columns+` FROM tickets WHERE id = $1`, id)
	t, err := scan(row)
	if err != nil {
		return Ticket{}, fmt.Errorf("get ticket: %w", err)
	}
	return t, nil
}

func (r *Repository) List(ctx context.Context, f ListFilter) ([]Ticket, error) {
	// Keyset pagination on (time_start, id): stable and fast at any depth,
	// unlike OFFSET which rescans every skipped row.
	rows, err := r.pool.Query(ctx, `
		SELECT `+columns+` FROM tickets
		WHERE departure_location = $1
		  AND destination = $2
		  AND ($3::timestamptz IS NULL OR time_start >= $3)
		  AND ($4::timestamptz IS NULL OR time_start <  $4)
		  AND ($5::timestamptz IS NULL OR (time_start, id) > ($5, $6))
		ORDER BY time_start, id
		LIMIT $7`,
		f.DepartureLocation, f.Destination,
		nullTime(f.TimeStartFrom), nullTime(f.TimeStartTo),
		nullTime(f.AfterTimeStart), f.AfterID, f.Limit)
	if err != nil {
		return nil, fmt.Errorf("list tickets: %w", err)
	}
	defer rows.Close()

	var out []Ticket
	for rows.Next() {
		t, err := scan(rows)
		if err != nil {
			return nil, fmt.Errorf("list tickets: %w", err)
		}
		out = append(out, t)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("list tickets: %w", err)
	}
	return out, nil
}

// UpdateStatus changes status only if the current status is allowed.
// The check and the write are one statement, so two concurrent
// "reserve" calls cannot both succeed.
func (r *Repository) UpdateStatus(ctx context.Context, id uuid.UUID, to Status) (Ticket, error) {
	row := r.pool.QueryRow(ctx, `
		UPDATE tickets SET status = $2, updated_at = now()
		WHERE id = $1 AND status = ANY($3)
		RETURNING `+columns,
		id, string(to), statusStrings(AllowedFrom(to)))
	t, err := scan(row)
	if errors.Is(err, ErrNotFound) {
		// Distinguish "no such ticket" from "wrong current status".
		_, getErr := r.Get(ctx, id)
		if getErr == nil {
			return Ticket{}, ErrInvalidTransition
		}
		if !errors.Is(getErr, ErrNotFound) {
			return Ticket{}, fmt.Errorf("check ticket after status update: %w", getErr)
		}
	}
	if err != nil {
		return Ticket{}, fmt.Errorf("update ticket status: %w", err)
	}
	return t, nil
}

func (r *Repository) Delete(ctx context.Context, id uuid.UUID) error {
	tag, err := r.pool.Exec(ctx, `DELETE FROM tickets WHERE id = $1`, id)
	if err != nil {
		return fmt.Errorf("delete ticket: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

func scan(row pgx.Row) (Ticket, error) {
	var t Ticket
	err := row.Scan(&t.ID, &t.Name, &t.Status, &t.DepartureLocation, &t.Destination,
		&t.PriceCents, &t.Type, &t.TimeStart, &t.TimeEnd, &t.CreatedAt, &t.UpdatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return Ticket{}, ErrNotFound
	}
	if err != nil {
		return Ticket{}, fmt.Errorf("scan ticket: %w", err)
	}
	return t, nil
}

func statusStrings(ss []Status) []string {
	out := make([]string, len(ss))
	for i, s := range ss {
		out[i] = string(s)
	}
	return out
}

func nullTime(t time.Time) any {
	if t.IsZero() {
		return nil
	}
	return t
}
