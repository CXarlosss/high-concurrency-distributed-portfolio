package storage

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type TripEvent struct {
	EventID   string    `json:"event_id"`
	TripID    string    `json:"trip_id"`
	DriverID  string    `json:"driver_id"`
	State     string    `json:"state"`
	Timestamp time.Time `json:"timestamp"`
}

type DBStore interface {
	SaveEventsBatch(ctx context.Context, events []TripEvent) error
	Close()
}

type PostgresDB struct {
	pool *pgxpool.Pool
}

func NewPostgresDB(ctx context.Context, connString string) (*PostgresDB, error) {
	cfg, err := pgxpool.ParseConfig(connString)
	if err != nil {
		return nil, fmt.Errorf("failed to parse postgres config: %w", err)
	}

	cfg.MaxConns = 50
	cfg.MinConns = 5
	cfg.MaxConnLifetime = 1 * time.Hour
	cfg.MaxConnIdleTime = 30 * time.Minute

	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		return nil, fmt.Errorf("failed to create pgxpool: %w", err)
	}

	return &PostgresDB{pool: pool}, nil
}

func (p *PostgresDB) InitSchema(ctx context.Context) error {
	query := `
	CREATE TABLE IF NOT EXISTS trips_history (
		id BIGSERIAL PRIMARY KEY,
		event_id VARCHAR(64) NOT NULL UNIQUE,
		trip_id VARCHAR(64) NOT NULL,
		driver_id VARCHAR(64) NOT NULL,
		state VARCHAR(32) NOT NULL,
		created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
	);
	CREATE INDEX IF NOT EXISTS idx_trips_history_trip ON trips_history(trip_id);
	`
	_, err := p.pool.Exec(ctx, query)
	return err
}

func (p *PostgresDB) SaveEventsBatch(ctx context.Context, events []TripEvent) error {
	if len(events) == 0 {
		return nil
	}

	batch := &pgx.Batch{}
	query := `
		INSERT INTO trips_history (event_id, trip_id, driver_id, state, created_at)
		VALUES ($1, $2, $3, $4, $5)
		ON CONFLICT (event_id) DO NOTHING;
	`

	for _, e := range events {
		batch.Queue(query, e.EventID, e.TripID, e.DriverID, e.State, e.Timestamp)
	}

	br := p.pool.SendBatch(ctx, batch)
	defer br.Close()

	for range events {
		_, err := br.Exec()
		if err != nil {
			return fmt.Errorf("failed batch execution in postgres: %w", err)
		}
	}

	return nil
}

func (p *PostgresDB) Close() {
	if p.pool != nil {
		p.pool.Close()
	}
}
