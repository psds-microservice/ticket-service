package kafka

import (
	"context"
	"encoding/json"
	"log"
	"strings"
	"time"

	"github.com/segmentio/kafka-go"
)

// TicketEventProducer — интерфейс для отправки событий тикета в Kafka (для подмены моком в тестах).
type TicketEventProducer interface {
	ProduceTicketEvent(ctx context.Context, event string, payload map[string]interface{})
}

// Producer пишет события тикетов в топик Kafka (best-effort, не блокирует API).
type Producer struct {
	writer *kafka.Writer
	topic  string
}

// NewProducer создаёт продюсер. Если brokers пустой или topic пустой — методы no-op.
func NewProducer(brokers []string, topic string) *Producer {
	if len(brokers) == 0 || topic == "" {
		return &Producer{}
	}
	return &Producer{
		topic: topic,
		writer: &kafka.Writer{
			Addr:         kafka.TCP(brokers...),
			Topic:        topic,
			Balancer:     &kafka.LeastBytes{},
			BatchTimeout: 10 * time.Millisecond,
		},
	}
}

// ProduceTicketEvent отправляет событие тикета в топик. payload: ticket_id, session_id, client_id, operator_id, subject, notes, status.
func (p *Producer) ProduceTicketEvent(ctx context.Context, event string, payload map[string]interface{}) {
	if p.writer == nil {
		return
	}
	msg := map[string]interface{}{"event": event}
	for k, v := range payload {
		msg[k] = v
	}
	body, err := json.Marshal(msg)
	if err != nil {
		log.Printf("kafka: marshal ticket event: %v", err)
		return
	}
	if err := p.writer.WriteMessages(ctx, kafka.Message{Value: body}); err != nil {
		log.Printf("kafka: write ticket event: %v", err)
	}
}

// Close закрывает writer.
func (p *Producer) Close() error {
	if p.writer == nil {
		return nil
	}
	return p.writer.Close()
}

// ParseBrokers разбивает строку брокеров "host1:9092,host2:9092" на слайс.
func ParseBrokers(s string) []string {
	var out []string
	for _, t := range strings.Split(s, ",") {
		if t = strings.TrimSpace(t); t != "" {
			out = append(out, t)
		}
	}
	return out
}
