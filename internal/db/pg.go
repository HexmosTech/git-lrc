package db

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5/pgxpool"
)

type PG struct {
	Conn *pgxpool.Pool
}

func Connect(ctx context.Context, dsn string) (*PG, error) {
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		return nil, fmt.Errorf("pg connect: %w", err)
	}
	return &PG{Conn: pool}, nil
}

func ConnectWithMaxConns(ctx context.Context, dsn string, maxConns int32) (*PG, error) {
	cfg, err := pgxpool.ParseConfig(dsn)
	if err != nil {
		return nil, fmt.Errorf("pg parse config: %w", err)
	}
	if maxConns > 0 {
		cfg.MaxConns = maxConns
	}
	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		return nil, fmt.Errorf("pg connect: %w", err)
	}
	return &PG{Conn: pool}, nil
}

func (p *PG) Close(_ context.Context) error {
	p.Conn.Close()
	return nil
}
