package store

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

type Store struct {
	pool *pgxpool.Pool
}

type Monitor struct {
	Id              int64      `json:"id"`
	Url             string     `json:"url"`
	IntervalSeconds int        `json:"interval_seconds"`
	ExpectedStatus  int        `json:"expected_status"`
	LastCheckedAt   *time.Time `json:"last_checked_at"`
	CreatedAt       time.Time  `json:"created_at"`
	UptimePercent   *float64   `json:"uptime_percent"`
}

type DueMonitor struct {
	Id             int64  `json:"id"`
	Url            string `json:"url"`
	ExpectedStatus int    `json:"expected_status"`
}

func New(pool *pgxpool.Pool) *Store {
	return &Store{pool}
}

func (s *Store) CreateMonitor(ctx context.Context, url string, intervalSeconds int, expectedStatus int) (int64, error) {
	var id int64
	query := `INSERT INTO monitors(url, interval_seconds, expected_status)
			  VALUES ($1, $2, $3)
			  RETURNING id;`

	err := s.pool.QueryRow(ctx, query, url, intervalSeconds, expectedStatus).Scan(&id)

	if err != nil {
		return 0, err
	}

	return id, nil
}

func (s *Store) ListMonitors(ctx context.Context) ([]Monitor, error) {
	list := []Monitor{}

	query := `SELECT monitors.id, url, interval_seconds, expected_status,
			  last_checked_at, monitors.created_at,
			  (100.0 * count(*) FILTER (WHERE error IS NULL 
			  AND status_code = monitors.expected_status) / NULLIF(count(checks.id), 0))::float8 AS uptime_percent
			  FROM monitors LEFT JOIN checks ON checks.monitor_id = monitors.id
			  GROUP BY monitors.id ORDER BY monitors.id;`

	rows, err := s.pool.Query(ctx, query)
	if err != nil {
		return nil, err
	}

	defer rows.Close()

	for rows.Next() {
		var m Monitor

		err := rows.Scan(
			&m.Id,
			&m.Url,
			&m.IntervalSeconds,
			&m.ExpectedStatus,
			&m.LastCheckedAt,
			&m.CreatedAt,
			&m.UptimePercent,
		)

		if err != nil {
			return nil, err
		}

		list = append(list, m)
	}

	if err := rows.Err(); err != nil {
		return nil, err
	}

	return list, nil

}

func (s *Store) DeleteMonitor(ctx context.Context, id int64) (int64, error) {

	query := `DELETE FROM monitors
			  WHERE id = $1;`

	res, err := s.pool.Exec(ctx, query, id)

	if err != nil {
		return 0, err
	}

	rowsAffected := res.RowsAffected()

	return rowsAffected, nil
}

func (s *Store) ListDueMonitors(ctx context.Context) ([]DueMonitor, error) {
	list := []DueMonitor{}

	query := `SELECT id, url, expected_status FROM monitors
			  WHERE last_checked_at IS NULL OR 
			  last_checked_at + make_interval(secs => interval_seconds) <= now();`

	rows, err := s.pool.Query(ctx, query)

	if err != nil {
		return nil, err
	}

	defer rows.Close()

	for rows.Next() {
		var d DueMonitor

		err := rows.Scan(
			&d.Id,
			&d.Url,
			&d.ExpectedStatus,
		)

		if err != nil {
			return nil, err
		}

		list = append(list, d)
	}

	if err := rows.Err(); err != nil {
		return nil, err
	}

	return list, nil
}

func (s *Store) SaveCheck(ctx context.Context, monitorId int64, statusCode *int, totalLatencyMs int64, errorMsg *string) error {

	query := `WITH ins AS (INSERT INTO checks 
			  (monitor_id, status_code, total_latency_ms, error)
			  VALUES($1, $2, $3, $4))

			  UPDATE monitors SET last_checked_at = now() 
			  WHERE id = $1;
			  `

	_, err := s.pool.Exec(ctx, query, monitorId, statusCode, totalLatencyMs, errorMsg)

	if err != nil {
		return err
	}
	return nil
}
