package main

import (
	"context"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/supeeermario/uptime-monitor/internal/api"
	"github.com/supeeermario/uptime-monitor/internal/config"
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

	store := store.New(pool)
	handler := api.CreateHandler(store)

	r.GET("/healthz", api.Health)
	r.POST("/monitors", handler.CreateMonitor)
	r.GET("/monitors", handler.ListMonitors)
	r.DELETE("/monitors/:id", handler.DeleteMonitor)
	r.GET("/duemonitors", handler.ListDueMonitors)

	server := &http.Server{
		Addr:    ":" + envs.PORT,
		Handler: r,
	}

	go func() {
		log.Printf("Server is starting on port: %v", envs.PORT)

		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("error while starting the server: %v", err)
		}
	}()

	<-sigChan

	log.Println("Server is shutting down")

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)

	defer cancel()

	if err := server.Shutdown(ctx); err != nil {
		log.Printf("shutdown didn't complete due to: %v", err)
	}

	pool.Close()

}
