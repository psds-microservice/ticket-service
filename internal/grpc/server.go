package grpc

import (
	"context"
	"errors"
	"log"
	"strings"
	"time"

	"github.com/psds-microservice/ticket-service/internal/errs"
	"github.com/psds-microservice/ticket-service/internal/kafka"
	"github.com/psds-microservice/ticket-service/internal/model"
	"github.com/psds-microservice/ticket-service/internal/service"
	"github.com/psds-microservice/ticket-service/pkg/gen/ticket_service"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/timestamppb"
	"gorm.io/gorm"
)

// Deps — зависимости gRPC-сервера (D: зависимость от абстракций).
type Deps struct {
	Ticket   service.TicketServicer
	Producer kafka.TicketEventProducer
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

// getMetadata returns the first value for key from incoming gRPC metadata (case-insensitive key).
func getMetadata(ctx context.Context, key string) string {
	md, ok := metadata.FromIncomingContext(ctx)
	if !ok {
		return ""
	}
	keyLower := strings.ToLower(key)
	if v := md.Get(keyLower); len(v) > 0 {
		return strings.TrimSpace(v[0])
	}
	return ""
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

func ticketEventPayload(t *model.Ticket) map[string]interface{} {
	if t == nil {
		return nil
	}
	return map[string]interface{}{
		"ticket_id":   int64(t.ID),
		"session_id":  t.SessionID,
		"client_id":   t.ClientID,
		"operator_id": t.OperatorID,
		"subject":     t.Subject,
		"notes":       t.Notes,
		"status":      string(t.Status),
	}
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
	// Fire-and-forget: событие должно уйти даже при отмене запроса, но с таймаутом
	if s.Producer != nil {
		payload := ticketEventPayload(ticket)
		eventCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		go s.Producer.ProduceTicketEvent(eventCtx, "ticket.created", payload)
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
	// Permission check: caller must be the ticket's client or assigned operator.
	ticket, err := s.Ticket.GetByID(ctx, uint64(req.GetId()))
	if err != nil {
		return nil, s.mapError(err)
	}
	callerID := getMetadata(ctx, "x-caller-id")
	if callerID == "" {
		return nil, status.Error(codes.PermissionDenied, "caller identity required (x-caller-id)")
	}
	if ticket.ClientID != callerID && ticket.OperatorID != callerID {
		return nil, status.Error(codes.PermissionDenied, "caller is not the ticket client or assigned operator")
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

	ticket, err = s.Ticket.Update(ctx, uint64(req.GetId()), changes)
	if err != nil {
		return nil, s.mapError(err)
	}
	// Fire-and-forget: событие должно уйти даже при отмене запроса, но с таймаутом
	if s.Producer != nil {
		if full, _ := s.Ticket.GetByID(ctx, uint64(req.GetId())); full != nil {
			eventCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			go s.Producer.ProduceTicketEvent(eventCtx, "ticket.updated", ticketEventPayload(full))
		}
	}
	return toProtoTicket(ticket), nil
}
