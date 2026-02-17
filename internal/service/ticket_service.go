package service

import (
	"context"
	"errors"

	"github.com/psds-microservice/ticket-service/internal/errs"
	"github.com/psds-microservice/ticket-service/internal/model"
	"gorm.io/gorm"
)

// TicketServicer — интерфейс для gRPC Deps (Dependency Inversion).
type TicketServicer interface {
	Create(ctx context.Context, t *model.Ticket) error
	GetByID(ctx context.Context, id uint64) (*model.Ticket, error)
	List(ctx context.Context, filter map[string]interface{}, limit, offset int) ([]model.Ticket, int64, error)
	Update(ctx context.Context, id uint64, changes map[string]interface{}) (*model.Ticket, error)
}

type TicketService struct {
	db *gorm.DB
}

func NewTicketService(db *gorm.DB) *TicketService {
	return &TicketService{db: db}
}

func (s *TicketService) Create(ctx context.Context, t *model.Ticket) error {
	return s.db.WithContext(ctx).Create(t).Error
}

func (s *TicketService) GetByID(ctx context.Context, id uint64) (*model.Ticket, error) {
	var t model.Ticket
	if err := s.db.WithContext(ctx).First(&t, id).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, errs.ErrTicketNotFound
		}
		return nil, err
	}
	return &t, nil
}

func (s *TicketService) List(ctx context.Context, filter map[string]interface{}, limit, offset int) ([]model.Ticket, int64, error) {
	var items []model.Ticket
	var total int64
	tx := s.db.WithContext(ctx).Model(&model.Ticket{})
	for k, v := range filter {
		tx = tx.Where(k, v)
	}
	// Count total before pagination
	if err := tx.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	// Apply pagination
	if limit > 0 {
		tx = tx.Limit(limit)
	}
	if offset > 0 {
		tx = tx.Offset(offset)
	}
	if err := tx.Order("created_at DESC").Find(&items).Error; err != nil {
		return nil, 0, err
	}
	return items, total, nil
}

func (s *TicketService) Update(ctx context.Context, id uint64, changes map[string]interface{}) (*model.Ticket, error) {
	var t model.Ticket
	if err := s.db.WithContext(ctx).First(&t, id).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, errs.ErrTicketNotFound
		}
		return nil, err
	}
	if err := s.db.WithContext(ctx).Model(&t).Updates(changes).Error; err != nil {
		return nil, err
	}
	return &t, nil
}
