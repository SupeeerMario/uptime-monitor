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

func (h *Handler) ListChecks(ctx *gin.Context) {
	id, err := strconv.ParseInt(ctx.Param("id"), 10, 64)
	limitStr := ctx.DefaultQuery("limit", "50")
	offsetStr := ctx.DefaultQuery("offset", "0")
	var limitValue int
	var offsetValue int

	if err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": "invalid id format"})
		return
	}

	if val, err := strconv.Atoi(limitStr); err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": "invalid limit"})
		return
	} else {

		limitValue = val
	}
	if val, err := strconv.Atoi(offsetStr); err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": "invalid offset"})
		return
	} else {

		offsetValue = val
	}

	if limitValue < 1 {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": "invalid limit"})
		return
	}

	if offsetValue < 0 {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": "invalid offset"})
		return
	}

	if limitValue > 200 {
		limitValue = 200
	}
	res, err := h.store.ListChecks(ctx.Request.Context(), id, limitValue, offsetValue)

	if err != nil {
		log.Printf("error while listing checks row: %v", err)
		ctx.JSON(http.StatusInternalServerError, gin.H{"error": "internal server error"})
		return
	}

	ctx.JSON(http.StatusOK, res)
}
