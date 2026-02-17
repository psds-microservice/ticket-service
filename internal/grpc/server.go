package grpc

import (
	"context"
	"errors"
	"log"

	"github.com/psds-microservice/ticket-service/internal/errs"
	"github.com/psds-microservice/ticket-service/internal/model"
	"github.com/psds-microservice/ticket-service/internal/searchindex"
	"github.com/psds-microservice/ticket-service/internal/service"
	"github.com/psds-microservice/ticket-service/pkg/gen/ticket_service"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/timestamppb"
	"gorm.io/gorm"
)

// Deps — зависимости gRPC-сервера
type Deps struct {
	Ticket *service.TicketService
	Search *searchindex.Client
}

// Server implements ticket_service.TicketServiceServer
type Server struct {
	ticket_service.UnimplementedTicketServiceServer
	Deps
}

// NewServer создаёт gRPC-сервер с внедрёнными сервисами
func NewServer(deps Deps) *Server {
	return &Server{Deps: deps}
}

func (s *Server) mapError(err error) error {
	if err == nil {
		return nil
	}
	// Обработка известных ошибок домена
	if errors.Is(err, errs.ErrTicketNotFound) {
		return status.Error(codes.NotFound, err.Error())
	}
	// Обработка ошибок GORM
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return status.Error(codes.NotFound, "record not found")
	}
	if errors.Is(err, gorm.ErrDuplicatedKey) {
		return status.Error(codes.AlreadyExists, "duplicate key violation")
	}
	// Проверяем, является ли это ошибкой GORM (для других типов ошибок GORM)
	// Ошибки валидации и ограничений БД обычно имеют специфичные коды
	// Для остальных ошибок логируем и возвращаем Internal с сообщением
	log.Printf("grpc: unhandled error: %v", err)
	return status.Error(codes.Internal, err.Error())
}

func toProtoTicket(t *model.Ticket) *ticket_service.Ticket {
	if t == nil {
		return nil
	}
	out := &ticket_service.Ticket{
		Id:         int64(t.ID),
		SessionId:  t.SessionID,
		ClientId:   t.ClientID,
		OperatorId: t.OperatorID,
		Status:     string(t.Status),
		Priority:   t.Priority,
		Region:     t.Region,
		Subject:    t.Subject,
		Notes:      t.Notes,
	}
	if !t.CreatedAt.IsZero() {
		out.CreatedAt = timestamppb.New(t.CreatedAt)
	}
	if !t.UpdatedAt.IsZero() {
		out.UpdatedAt = timestamppb.New(t.UpdatedAt)
	}
	if t.ClosedAt != nil {
		out.ClosedAt = timestamppb.New(*t.ClosedAt)
	}
	return out
}

func (s *Server) CreateTicket(ctx context.Context, req *ticket_service.CreateTicketRequest) (*ticket_service.Ticket, error) {
	// Валидация обязательных полей
	if req.GetSessionId() == "" {
		return nil, status.Error(codes.InvalidArgument, "session_id is required")
	}
	if req.GetClientId() == "" {
		return nil, status.Error(codes.InvalidArgument, "client_id is required")
	}
	// Валидация статуса
	statusStr := req.GetStatus()
	if statusStr == "" {
		statusStr = string(model.TicketStatusOpen)
	} else {
		validStatus := model.TicketStatus(statusStr)
		if validStatus != model.TicketStatusOpen && validStatus != model.TicketStatusInProgress && validStatus != model.TicketStatusClosed {
			return nil, status.Error(codes.InvalidArgument, "invalid status: must be 'open', 'in_progress', or 'closed'")
		}
	}
	ticket := &model.Ticket{
		SessionID:  req.GetSessionId(),
		ClientID:   req.GetClientId(),
		OperatorID: req.GetOperatorId(),
		Status:     model.TicketStatus(statusStr),
		Priority:   req.GetPriority(),
		Region:     req.GetRegion(),
		Subject:    req.GetSubject(),
		Notes:      req.GetNotes(),
	}
	if err := s.Ticket.Create(ctx, ticket); err != nil {
		return nil, s.mapError(err)
	}
	if s.Search != nil {
		s.Search.IndexTicketAsync(ticket)
	}
	return toProtoTicket(ticket), nil
}

func (s *Server) GetTicket(ctx context.Context, req *ticket_service.GetTicketRequest) (*ticket_service.Ticket, error) {
	ticket, err := s.Ticket.GetByID(ctx, uint64(req.GetId()))
	if err != nil {
		return nil, s.mapError(err)
	}
	return toProtoTicket(ticket), nil
}

func (s *Server) ListTickets(ctx context.Context, req *ticket_service.ListTicketsRequest) (*ticket_service.ListTicketsResponse, error) {
	filter := make(map[string]interface{})
	if req.GetClientId() != "" {
		filter["client_id = ?"] = req.GetClientId()
	}
	if req.GetOperatorId() != "" {
		filter["operator_id = ?"] = req.GetOperatorId()
	}
	if req.GetStatus() != "" {
		filter["status = ?"] = req.GetStatus()
	}
	if req.GetRegion() != "" {
		filter["region = ?"] = req.GetRegion()
	}

	limit := int(req.GetLimit())
	offset := int(req.GetOffset())

	tickets, total, err := s.Ticket.List(ctx, filter, limit, offset)
	if err != nil {
		return nil, s.mapError(err)
	}

	protoTickets := make([]*ticket_service.Ticket, len(tickets))
	for i, t := range tickets {
		protoTickets[i] = toProtoTicket(&t)
	}

	return &ticket_service.ListTicketsResponse{
		Tickets: protoTickets,
		Total:   int32(total),
	}, nil
}

func (s *Server) UpdateTicket(ctx context.Context, req *ticket_service.UpdateTicketRequest) (*ticket_service.Ticket, error) {
	// Валидация ID
	if req.GetId() <= 0 {
		return nil, status.Error(codes.InvalidArgument, "id must be greater than 0")
	}
	changes := make(map[string]interface{})
	if req.GetSubject() != "" {
		changes["subject"] = req.GetSubject()
	}
	if req.GetNotes() != "" {
		changes["notes"] = req.GetNotes()
	}
	if req.GetStatus() != "" {
		// Валидация статуса
		validStatus := model.TicketStatus(req.GetStatus())
		if validStatus != model.TicketStatusOpen && validStatus != model.TicketStatusInProgress && validStatus != model.TicketStatusClosed {
			return nil, status.Error(codes.InvalidArgument, "invalid status: must be 'open', 'in_progress', or 'closed'")
		}
		changes["status"] = req.GetStatus()
	}
	if req.GetPriority() != "" {
		changes["priority"] = req.GetPriority()
	}
	if req.GetRegion() != "" {
		changes["region"] = req.GetRegion()
	}

	if len(changes) == 0 {
		return nil, status.Error(codes.InvalidArgument, "no changes provided")
	}

	ticket, err := s.Ticket.Update(ctx, uint64(req.GetId()), changes)
	if err != nil {
		return nil, s.mapError(err)
	}

	if s.Search != nil {
		// Re-fetch for full entity to index
		if full, _ := s.Ticket.GetByID(ctx, uint64(req.GetId())); full != nil {
			s.Search.IndexTicketAsync(full)
		}
	}

	return toProtoTicket(ticket), nil
}
