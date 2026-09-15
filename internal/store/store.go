package store

import (
	"context"

	"github.com/jackc/pgx/v5/pgxpool"
)

type Store struct {
	pool *pgxpool.Pool
}

func New(pool *pgxpool.Pool) *Store {
	return &Store{pool}
}

func (s *Store) CreateMonitor(ctx context.Context, url string, intervalSeconds int, expectedStatus int) (int64, error) {
	var id int64
	query := `INSERT INTO monitors(url, interval_seconds, expected_status)
			  VALUES ($1, $2, $3)
			  RETURNING id`

	err := s.pool.QueryRow(ctx, query, url, intervalSeconds, expectedStatus).Scan(&id)

	if err != nil {
		return 0, err
	}

	return id, nil
}
