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

func (sc *Scheduler) Run(ctx context.Context, ch chan<- store.DueMonitor, schedDone <-chan int64) {
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()
	defer close(ch)

	inFlight := make(map[int64]struct{})

	for {
		select {
		case <-ticker.C:

			due, err := sc.store.ListDueMonitors(ctx)

			if err != nil {
				log.Println(err)
				continue
			}

			for _, m := range due {
				if _, exists := inFlight[m.Id]; exists {
					continue
				}

				inFlight[m.Id] = struct{}{}

				select {

				case ch <- m:
				case <-ctx.Done():
					log.Println("The Scheduler stopped mid passing the monitors")
					return

				}
			}

			log.Println(len(due))

		case doneId := <-schedDone:
			delete(inFlight, doneId)

		case <-ctx.Done():
			log.Println("The Scheduler stopped")
			return

		}
	}

}
