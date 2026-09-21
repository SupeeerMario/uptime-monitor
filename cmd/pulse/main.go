package main

import (
	"context"
	"log"
	"net/http"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/supeeermario/uptime-monitor/internal/api"
	"github.com/supeeermario/uptime-monitor/internal/config"
	"github.com/supeeermario/uptime-monitor/internal/prober"
	"github.com/supeeermario/uptime-monitor/internal/scheduler"
	"github.com/supeeermario/uptime-monitor/internal/store"
)

func main() {
	envs := config.Load()

	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)

	pool, err := pgxpool.New(context.Background(), envs.DB_URL)

	if err != nil {
		log.Fatalf("pgxpool error: %v", err)
	}

	poolPingctx, poolPingcancel := context.WithTimeout(context.Background(), 5*time.Second)

	if err := pool.Ping(poolPingctx); err != nil {
		log.Fatalf("error while pinging pgxpool: %v", err)
	}

	defer poolPingcancel()
	r := gin.Default()

	st := store.New(pool)
	handler := api.CreateHandler(st)
	sched := scheduler.New(st)

	r.GET("/healthz", api.Health)
	r.POST("/monitors", handler.CreateMonitor)
	r.GET("/monitors", handler.ListMonitors)
	r.DELETE("/monitors/:id", handler.DeleteMonitor)
	r.GET("/duemonitors", handler.ListDueMonitors)

	server := &http.Server{
		Addr:    ":" + envs.PORT,
		Handler: r,
	}

	var wg sync.WaitGroup

	jobs := make(chan store.DueMonitor, 100)
	results := make(chan prober.Result, 100)

	schedCtx, schedCancel := context.WithCancel(context.Background())
	proberCtx, proberCancel := context.WithCancel(context.Background())
	writerCtx, writerCancel := context.WithCancel(context.Background())
	defer writerCancel()

	schedDone := make(chan int64, 100)

	// after the run is finished a defer initiated
	// from within the func to close channel
	go sched.Run(schedCtx, jobs, schedDone)

	// adding 1 to the wait group and defering done from within the func,
	// before -1 the wg, it waits on the wait() to close the channel
	// so it's an indicator to when all the go routines are done,
	// which means when all responses has returned
	for i := range envs.PROBER_COUNT {
		wg.Add(1)
		go prober.HTTPWorker(proberCtx, jobs, results, &wg, i)
	}

	go func() {
		wg.Wait()
		close(results)
	}()

	// made an empty chan of type struct, no values being passed through it
	// it's only an indicator when all the check write to the db are finished,
	// when all the writes are finished, the channel is closed, which then fires
	// the sig to continue and close the pool
	done := make(chan struct{})

	go func() {

		for check := range results {
			err := st.SaveCheck(writerCtx, check.MonitorId, check.StatusCode, check.TotalLatencyMs.Milliseconds(), check.Error)
			if err != nil {
				log.Printf("error while inserting row: %v, into checks", err)
			}
			select {
			case schedDone <- check.MonitorId:
			case <-schedCtx.Done():
			}
		}
		close(done)
	}()

	go func() {
		log.Printf("Server is starting on port: %v", envs.PORT)

		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("error while starting the server: %v", err)
		}
	}()

	<-sigChan

	// when the kill sig is initiated the cancel fires to
	// prevent sched new tasks
	schedCancel()
	proberCancel()

	log.Println("Server is shutting down")

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)

	defer cancel()

	if err := server.Shutdown(ctx); err != nil {
		log.Printf("shutdown didn't complete due to: %v", err)
	}

	<-done
	pool.Close()

}
