package cmd

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/joho/godotenv"
	"github.com/psds-microservice/ticket-service/internal/config"
	"github.com/psds-microservice/ticket-service/internal/database"
	"github.com/psds-microservice/ticket-service/internal/kafka"
	"github.com/psds-microservice/ticket-service/internal/model"
	"github.com/psds-microservice/ticket-service/internal/searchindex"
	"github.com/spf13/cobra"
)

var reindexSearchCmd = &cobra.Command{
	Use:   "reindex-search",
	Short: "Reindex all tickets into search. Prefers Kafka; falls back to HTTP if SEARCH_SERVICE_URL set.",
	RunE:  runReindexSearch,
}

func init() {
	rootCmd.AddCommand(reindexSearchCmd)
}

func runReindexSearch(cmd *cobra.Command, args []string) error {
	_ = godotenv.Load(".env")
	_ = godotenv.Load("../.env")
	_ = godotenv.Load("../../.env") // repo root when running from bin/
	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("config: %w", err)
	}
	if err := cfg.Validate(); err != nil {
		return fmt.Errorf("config: %w", err)
	}
	conn, err := database.Open(cfg.DSN())
	if err != nil {
		return fmt.Errorf("db: %w", err)
	}

	var tickets []model.Ticket
	if err := conn.Find(&tickets).Error; err != nil {
		return fmt.Errorf("list tickets: %w", err)
	}
	log.Printf("reindex-search: found %d tickets", len(tickets))

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	defer cancel()

	// Prefer Kafka, then HTTP
	if len(cfg.KafkaBrokers) > 0 && cfg.KafkaTopicTicket != "" {
		log.Println("reindex-search: using Kafka for reindexing")
		producer := kafka.NewProducer(cfg.KafkaBrokers, cfg.KafkaTopicTicket)
		for i := range tickets {
			t := &tickets[i]
			payload := map[string]interface{}{
				"ticket_id":   int64(t.ID),
				"session_id":  t.SessionID,
				"client_id":   t.ClientID,
				"operator_id": t.OperatorID,
				"subject":     t.Subject,
				"notes":       t.Notes,
				"status":      string(t.Status),
			}
			producer.ProduceTicketEvent(ctx, "ticket.updated", payload)
			if (i+1)%50 == 0 || i == len(tickets)-1 {
				log.Printf("reindex-search: sent %d/%d events to Kafka", i+1, len(tickets))
			}
		}
		log.Printf("reindex-search: done, sent %d events to Kafka (search-service worker will index them)", len(tickets))
		return nil
	}
	if cfg.SearchServiceURL != "" {
		log.Println("reindex-search: using HTTP for reindexing")
		client := searchindex.NewClient(cfg.SearchServiceURL)
		for i := range tickets {
			client.IndexTicket(ctx, &tickets[i])
			if (i+1)%50 == 0 || i == len(tickets)-1 {
				log.Printf("reindex-search: indexed %d/%d", i+1, len(tickets))
			}
		}
		log.Printf("reindex-search: done, indexed %d tickets via HTTP", len(tickets))
		return nil
	}
	log.Println("reindex-search: neither KAFKA_BROKERS nor SEARCH_SERVICE_URL set")
	log.Println("reindex-search: normal indexing is via Kafka (search-service worker)")
	log.Printf("reindex-search: found %d tickets (not reindexed)", len(tickets))
	return nil
}
