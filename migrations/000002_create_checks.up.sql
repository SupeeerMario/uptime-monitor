CREATE TABLE checks (
    id BIGSERIAL PRIMARY KEY,
    monitor_id BIGINT NOT NULL,
    status_code INT CHECK(status_code BETWEEN 100 AND 599) NULL,
    ttfb INTERVAL NULL,
    total_latency INTERVAL NULL,
    error TEXT NULL,
    redirect_chain TEXT NULL,
    tls TIMESTAMPTZ NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),


    CONSTRAINT fk_checks_monitor
        FOREIGN KEY (monitor_id)
        REFERENCES monitors(id)
        ON DELETE CASCADE
);

CREATE INDEX IF NOT EXISTS checks_monitor ON checks(monitor_id, created_at DESC);