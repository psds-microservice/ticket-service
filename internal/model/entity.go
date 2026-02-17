package model

import "time"

type TicketStatus string

const (
	TicketStatusOpen       TicketStatus = "open"
	TicketStatusInProgress TicketStatus = "in_progress"
	TicketStatusClosed     TicketStatus = "closed"
)

type Ticket struct {
	ID         uint64       `gorm:"primaryKey" json:"id"`
	SessionID  string       `gorm:"index;not null" json:"session_id"`
	ClientID   string       `gorm:"index;not null" json:"client_id"`
	OperatorID string       `gorm:"index" json:"operator_id,omitempty"`
	Status     TicketStatus `gorm:"type:varchar(32);index;not null" json:"status"`
	Priority   string       `gorm:"type:varchar(32);index" json:"priority,omitempty"`
	Region     string       `gorm:"type:varchar(64);index" json:"region,omitempty"`
	Subject    string       `gorm:"type:varchar(255)" json:"subject,omitempty"`
	Notes      string       `gorm:"type:text" json:"notes,omitempty"`

	CreatedAt time.Time  `json:"created_at"`
	UpdatedAt time.Time  `json:"updated_at"`
	ClosedAt  *time.Time `json:"closed_at,omitempty"`
}
