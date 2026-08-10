package db

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"
)

type PG struct {
	Conn *pgx.Conn
}

func Connect(ctx context.Context, dsn string) (*PG, error) {
	conn, err := pgx.Connect(ctx, dsn)
	if err != nil {
		return nil, fmt.Errorf("pg connect: %w", err)
	}
	return &PG{Conn: conn}, nil
}

func (p *PG) Close(ctx context.Context) error {
	return p.Conn.Close(ctx)
}
