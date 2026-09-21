package prober

import (
	"context"
	"io"
	"log"
	"net/http"
	"sync"
	"time"

	"github.com/supeeermario/uptime-monitor/internal/store"
)

type Result struct {
	MonitorId      int64
	StatusCode     *int
	TotalLatencyMs time.Duration
	Error          *string
}

func HTTPWorker(ctx context.Context, ch <-chan store.DueMonitor, writeCh chan<- Result, wg *sync.WaitGroup, proberID int) {
	defer wg.Done()
	client := http.Client{Timeout: 10 * time.Second}

	for {
		select {
		case m, ok := <-ch:
			if ok == true {
				var res Result

				log.Printf("Prober-Id: %d, Url: %v", proberID, m.Url)

				req, err := http.NewRequestWithContext(context.Background(), "GET", m.Url, nil)

				if err != nil {
					s := err.Error()
					res.Error = &s
					res.MonitorId = m.Id

					writeCh <- res

					continue
				}

				start := time.Now()
				clientRes, err := client.Do(req)
				res.TotalLatencyMs = time.Since(start)
				res.MonitorId = m.Id

				if err != nil {
					s := err.Error()
					res.Error = &s

					writeCh <- res

					continue
				}

				statusCode := int(clientRes.StatusCode)
				res.StatusCode = &statusCode

				log.Printf("proberID: %d, MonitorId: %d, *StatusCode: %d, TotalLatencyMs: %v, Error: %v", proberID, res.MonitorId, *res.StatusCode, res.TotalLatencyMs, res.Error)

				writeCh <- res

				io.Copy(io.Discard, clientRes.Body)
				clientRes.Body.Close()
			} else {
				return
			}
		case <-ctx.Done():
			return
		}
	}

}
