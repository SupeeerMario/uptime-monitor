package api

import (
	"log"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
	"github.com/supeeermario/uptime-monitor/internal/store"
)

type Handler struct {
	store *store.Store
}

type Request struct {
	URL             string `json:"url" binding:"required,url"`
	IntervalSeconds int    `json:"interval_seconds" binding:"required,gt=0"`
	ExpectedStatus  int    `json:"expected_status" binding:"required,gte=100,lte=599"`
}

func CreateHandler(s *store.Store) *Handler {
	if s == nil {
		log.Fatalf("Error while creating the handler")
	}
	return &Handler{s}
}

func (h *Handler) CreateMonitor(ctx *gin.Context) {
	var req Request

	if err := ctx.ShouldBindJSON(&req); err != nil {
		ctx.JSON(http.StatusUnprocessableEntity, gin.H{"error": err.Error()})
		return
	}

	id, err := h.store.CreateMonitor(
		ctx.Request.Context(),
		req.URL,
		req.IntervalSeconds,
		req.ExpectedStatus,
	)

	if err != nil {
		log.Printf("error while saving monitor to postgres: %v", err)
		ctx.JSON(http.StatusInternalServerError, "internal server error")
		return
	}
	ctx.Header("Location", "/monitors/"+strconv.FormatInt(id, 10))
	ctx.JSON(http.StatusCreated, gin.H{
		"id":               id,
		"url":              req.URL,
		"interval_seconds": req.IntervalSeconds,
		"expected_status":  req.ExpectedStatus,
	})
}

func (h *Handler) ListMonitors(ctx *gin.Context) {
	res, err := h.store.ListMonitors(ctx.Request.Context())

	if err != nil {
		log.Printf("error while getting monitors from postgres: %v", err)
		ctx.JSON(http.StatusInternalServerError, "internal server error")
		return
	}

	ctx.JSON(http.StatusOK, res)
}
