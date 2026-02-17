package cmd

import (
	"context"
	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"

	"github.com/joho/godotenv"
	"github.com/psds-microservice/ticket-service/internal/application"
	"github.com/psds-microservice/ticket-service/internal/config"
	"github.com/spf13/cobra"
)

var apiCmd = &cobra.Command{
	Use:   "api",
	Short: "Run HTTP + gRPC API server",
	RunE:  runAPI,
}

func runAPI(cmd *cobra.Command, args []string) error {
	_ = godotenv.Load(".env")
	_ = godotenv.Load("../.env")
	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("config: %w", err)
	}
	if err := cfg.Validate(); err != nil {
		return fmt.Errorf("config: %w", err)
	}
	app, err := application.NewAPI(cfg)
	if err != nil {
		return err
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	if err := app.Run(ctx); err != nil {
		return err
	}
	log.Println("bye")
	return nil
}
