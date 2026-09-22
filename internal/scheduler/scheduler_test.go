package scheduler

import (
	"context"

	"github.com/supeeermario/uptime-monitor/internal/store"
)

type fake struct {
	list []store.DueMonitor
}

func (l fake) ListDueMonitors(ctx context.Context) ([]store.DueMonitor, error) {
	return l.list, nil
}
