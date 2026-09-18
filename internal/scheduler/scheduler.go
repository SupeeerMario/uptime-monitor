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

func (sc *Scheduler) Run(ctx context.Context, ch chan<- store.DueMonitor) {
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()
	defer close(ch)

	for {
		select {
		case <-ticker.C:
			due, err := sc.store.ListDueMonitors(ctx)

			if err != nil {
				log.Println(err)
				continue
			}

			for _, m := range due {
				select {
				case ch <- m:
				case <-ctx.Done():
					log.Println("The Scheduler stopped mid passing the monitors")
					return

				}
			}

			log.Println(len(due))
		case <-ctx.Done():
			log.Println("The Scheduler stopped")
			return

		}
	}

}
