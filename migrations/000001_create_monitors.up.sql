CREATE TABLE monitors(
    id BIGSERIAL PRIMARY KEY,
    url TEXT NOT NULL,
    interval_seconds INT CHECK(interval_seconds > 0) NOT NULL,
    expected_status INT CHECK(expected_status BETWEEN 100 AND 599) NOT NULL,
    last_checked_at TIMESTAMPTZ NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);