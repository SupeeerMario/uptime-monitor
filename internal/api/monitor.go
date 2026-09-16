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

func (h *Handler) DeleteMonitor(ctx *gin.Context) {
	id, err := strconv.ParseInt(ctx.Param("id"), 10, 64)

	if err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": "invalid id format"})
		return
	}

	res, err := h.store.DeleteMonitor(ctx.Request.Context(), id)

	if err != nil {
		log.Printf("error while deleting monitor row: %v", err)
		ctx.JSON(http.StatusInternalServerError, gin.H{"error": "internal server error"})
		return
	}

	if res == 0 {
		ctx.JSON(http.StatusNotFound, gin.H{"error": "monitor not found"})
		return
	}

	ctx.Status(http.StatusNoContent)

}

func (h *Handler) ListDueMonitors(ctx *gin.Context) {

	res, err := h.store.ListDueMonitors(ctx.Request.Context())

	if err != nil {
		log.Printf("error while returning due monitors: %v", err)
		ctx.JSON(http.StatusInternalServerError, gin.H{"error": "internal server error"})
		return
	}

	ctx.JSON(http.StatusOK, res)
}
