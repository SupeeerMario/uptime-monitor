package scheduler

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/supeeermario/uptime-monitor/internal/prober"
	"github.com/supeeermario/uptime-monitor/internal/store"
)

type fake struct {
	list []store.DueMonitor
}

func (l fake) ListDueMonitors(ctx context.Context) ([]store.DueMonitor, error) {
	return l.list, nil
}

func TestSlowMonitorDoesNotBlockOthers(t *testing.T) {
	var m sync.Mutex
	var counter int

	stuck := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {

		time.Sleep(2 * time.Second)
	}))

	defer stuck.Close()

	healthy := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {

		m.Lock()
		counter++
		m.Unlock()

	}))

	defer healthy.Close()

	f := fake{
		list: []store.DueMonitor{
			{Id: 1, Url: stuck.URL, ExpectedStatus: 200},
			{Id: 2, Url: healthy.URL, ExpectedStatus: 200},
			{Id: 3, Url: healthy.URL, ExpectedStatus: 200},
			{Id: 4, Url: healthy.URL, ExpectedStatus: 200},
		},
	}

	sched := New(f)

	cancellationCtx, cancellationCancel := context.WithCancel(context.Background())
	defer cancellationCancel()

	jobs := make(chan store.DueMonitor, 10)
	results := make(chan prober.Result, 100)
	schedDone := make(chan int64, 100)

	go sched.Run(cancellationCtx, 50*time.Millisecond, jobs, schedDone)
	var wg sync.WaitGroup

	for i := range 3 {
		wg.Add(1)
		go prober.HTTPWorker(cancellationCtx, jobs, results, &wg, i)
	}

	go func() {
		for check := range results {
			select {
			case schedDone <- check.MonitorId:
			case <-cancellationCtx.Done():
			}
		}

	}()
	time.Sleep(1 * time.Second)
	cancellationCancel()

	m.Lock()
	hits := counter
	m.Unlock()
	if hits < 10 {
		t.Fatalf("counter is less than 10, counter: %v", hits)
	}
}
