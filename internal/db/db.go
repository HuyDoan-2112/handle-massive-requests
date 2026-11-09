package db

import (
	"context"
	"github.com/jackc/pgx/v5"
)

func NewDB(dsn string) (*pgx.Conn, error) {
	db, err := pgx.Connect(context.Background(), dsn)
	if err != nil {
		return nil, err
	}
	return db, nil
}

func CloseDB(db *pgx.Conn) error {
	return db.Close(context.Background())
}
