package searchindex

import (
	"bytes"
	"context"
	"encoding/json"
	"log"
	"net/http"
	"time"

	"github.com/psds-microservice/ticket-service/internal/model"
)

// Client отправляет тикеты в search-service для индексации (best-effort, не блокирует API).
type Client struct {
	baseURL    string
	httpClient *http.Client
}

// NewClient возвращает клиент. Если baseURL пустой, вызовы IndexTicket — no-op.
func NewClient(baseURL string) *Client {
	return &Client{
		baseURL: baseURL,
		httpClient: &http.Client{
			Timeout: 5 * time.Second,
		},
	}
}

// IndexTicketPayload — тело POST /search/index/ticket.
type IndexTicketPayload struct {
	TicketID   int64  `json:"ticket_id"`
	SessionID  string `json:"session_id"`
	ClientID   string `json:"client_id"`
	OperatorID string `json:"operator_id"`
	Subject    string `json:"subject"`
	Notes      string `json:"notes"`
	Status     string `json:"status"`
}

// IndexTicket отправляет тикет в search-service. Вызывать в goroutine после Create/Update.
func (c *Client) IndexTicket(ctx context.Context, t *model.Ticket) {
	if c.baseURL == "" {
		return
	}
	payload := IndexTicketPayload{
		TicketID:   int64(t.ID),
		SessionID:  t.SessionID,
		ClientID:   t.ClientID,
		OperatorID: t.OperatorID,
		Subject:    t.Subject,
		Notes:      t.Notes,
		Status:     string(t.Status),
	}
	body, err := json.Marshal(payload)
	if err != nil {
		log.Printf("searchindex: marshal: %v", err)
		return
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/search/index/ticket", bytes.NewReader(body))
	if err != nil {
		log.Printf("searchindex: new request: %v", err)
		return
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := c.httpClient.Do(req)
	if err != nil {
		log.Printf("searchindex: request: %v", err)
		return
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		log.Printf("searchindex: status %d for ticket %d", resp.StatusCode, t.ID)
		return
	}
}

// IndexTicketAsync вызывает IndexTicket в отдельной горутине (не блокирует ответ API).
func (c *Client) IndexTicketAsync(t *model.Ticket) {
	if c.baseURL == "" {
		return
	}
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		c.IndexTicket(ctx, t)
	}()
}
