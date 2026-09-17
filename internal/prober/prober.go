package prober

import (
	"context"
	"io"
	"log"
	"net/http"
	"time"

	"github.com/supeeermario/uptime-monitor/internal/store"
)

type Result struct {
	MonitorId      int64
	StatusCode     *int
	TotalLatencyMs time.Duration
	Error          *string
}

func HTTPWorker(ctx context.Context, ch <-chan store.DueMonitor, writeCh chan<- Result) {
	client := http.Client{Timeout: 10 * time.Second}

	for m := range ch {
		var res Result

		log.Println(m)

		req, err := http.NewRequestWithContext(ctx, "GET", m.Url, nil)

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

		log.Println(res.MonitorId, *res.StatusCode, res.TotalLatencyMs, res.Error)

		writeCh <- res

		io.Copy(io.Discard, clientRes.Body)
		clientRes.Body.Close()
	}

}
