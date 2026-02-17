package handler

import (
	"errors"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
	"github.com/psds-microservice/ticket-service/internal/errs"
	"github.com/psds-microservice/ticket-service/internal/model"
	"github.com/psds-microservice/ticket-service/internal/searchindex"
	"github.com/psds-microservice/ticket-service/internal/service"
)

type TicketHandler struct {
	svc    *service.TicketService
	search *searchindex.Client
}

func NewTicketHandler(svc *service.TicketService, search *searchindex.Client) *TicketHandler {
	return &TicketHandler{svc: svc, search: search}
}

type createTicketRequest struct {
	SessionID  string `json:"session_id" binding:"required"`
	ClientID   string `json:"client_id" binding:"required"`
	OperatorID string `json:"operator_id"`
	Status     string `json:"status"`
	Priority   string `json:"priority"`
	Region     string `json:"region"`
	Subject    string `json:"subject"`
	Notes      string `json:"notes"`
}

func (h *TicketHandler) Create(c *gin.Context) {
	var req createTicketRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid body"})
		return
	}
	status := req.Status
	if status == "" {
		status = string(model.TicketStatusOpen)
	}
	ticket := &model.Ticket{
		SessionID:  req.SessionID,
		ClientID:   req.ClientID,
		OperatorID: req.OperatorID,
		Status:     model.TicketStatus(status),
		Priority:   req.Priority,
		Region:     req.Region,
		Subject:    req.Subject,
		Notes:      req.Notes,
	}
	if err := h.svc.Create(c.Request.Context(), ticket); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to create ticket"})
		return
	}
	h.search.IndexTicketAsync(ticket)
	c.JSON(http.StatusCreated, ticket)
}

func (h *TicketHandler) Get(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid id"})
		return
	}
	t, err := h.svc.GetByID(c.Request.Context(), id)
	if err != nil {
		if errors.Is(err, errs.ErrTicketNotFound) {
			c.JSON(http.StatusNotFound, gin.H{"error": "ticket not found"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, t)
}

func (h *TicketHandler) List(c *gin.Context) {
	filter := make(map[string]interface{})
	if v := c.Query("client_id"); v != "" {
		filter["client_id = ?"] = v
	}
	if v := c.Query("operator_id"); v != "" {
		filter["operator_id = ?"] = v
	}
	if v := c.Query("status"); v != "" {
		filter["status = ?"] = v
	}
	if v := c.Query("region"); v != "" {
		filter["region = ?"] = v
	}

	// Parse limit and offset
	limit := 0
	offset := 0
	if v := c.Query("limit"); v != "" {
		if parsed, err := strconv.Atoi(v); err == nil && parsed > 0 {
			limit = parsed
		}
	}
	if v := c.Query("offset"); v != "" {
		if parsed, err := strconv.Atoi(v); err == nil && parsed >= 0 {
			offset = parsed
		}
	}

	items, total, err := h.svc.List(c.Request.Context(), filter, limit, offset)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to list tickets"})
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"tickets": items,
		"total":   total,
	})
}

type updateTicketRequest struct {
	Status   *string `json:"status,omitempty"`
	Priority *string `json:"priority,omitempty"`
	Region   *string `json:"region,omitempty"`
	Subject  *string `json:"subject,omitempty"`
	Notes    *string `json:"notes,omitempty"`
}

func (h *TicketHandler) Update(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid id"})
		return
	}
	var req updateTicketRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid body"})
		return
	}
	changes := make(map[string]interface{})
	if req.Status != nil {
		changes["status"] = *req.Status
	}
	if req.Priority != nil {
		changes["priority"] = *req.Priority
	}
	if req.Region != nil {
		changes["region"] = *req.Region
	}
	if req.Subject != nil {
		changes["subject"] = *req.Subject
	}
	if req.Notes != nil {
		changes["notes"] = *req.Notes
	}
	if len(changes) == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "no changes"})
		return
	}
	t, err := h.svc.Update(c.Request.Context(), id, changes)
	if err != nil {
		if errors.Is(err, errs.ErrTicketNotFound) {
			c.JSON(http.StatusNotFound, gin.H{"error": "ticket not found"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	// Re-fetch for full entity to index (GORM Updates doesn't refresh all fields)
	if full, _ := h.svc.GetByID(c.Request.Context(), id); full != nil {
		h.search.IndexTicketAsync(full)
	}
	c.JSON(http.StatusOK, t)
}
