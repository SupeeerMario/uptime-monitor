package scheduler

import (
	"context"
	"log"
	"time"

	"github.com/supeeermario/uptime-monitor/internal/store"
)

type Scheduler struct {
	store *store.Store
}

func New(s *store.Store) *Scheduler {
	return &Scheduler{s}
}

func (sc *Scheduler) Run(ctx context.Context) {
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			due, err := sc.store.ListDueMonitors(ctx)

			if err != nil {
				log.Println(err)
				continue
			}
			log.Println(due)
		case <-ctx.Done():
			log.Println("The Scheduler stopped")
			return

		}
	}

}
