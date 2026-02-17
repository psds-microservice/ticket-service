package application

import (
	"context"
	"fmt"
	"log"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"github.com/grpc-ecosystem/grpc-gateway/v2/runtime"
	"github.com/psds-microservice/helpy/paths"
	"github.com/psds-microservice/ticket-service/internal/config"
	"github.com/psds-microservice/ticket-service/internal/database"
	grpcserver "github.com/psds-microservice/ticket-service/internal/grpc"
	"github.com/psds-microservice/ticket-service/internal/handler"
	"github.com/psds-microservice/ticket-service/internal/searchindex"
	"github.com/psds-microservice/ticket-service/internal/service"
	"github.com/psds-microservice/ticket-service/pkg/gen/ticket_service"
	httpSwagger "github.com/swaggo/http-swagger"
	"google.golang.org/grpc"
	"google.golang.org/grpc/reflection"
	"google.golang.org/protobuf/proto"
)

// serveOpenAPISpec отдаёт api/openapi.json или api/openapi.swagger.json (из proto: make proto-openapi).
func serveOpenAPISpec() http.HandlerFunc {
	return func(w http.ResponseWriter, _ *http.Request) {
		for _, path := range []string{"api/openapi.swagger.json", "api/openapi.json", "openapi.json"} {
			data, err := os.ReadFile(path)
			if err == nil {
				w.Header().Set("Content-Type", "application/json")
				w.Write(data)
				return
			}
		}
		exe, _ := os.Executable()
		if exe != "" {
			dir := filepath.Dir(exe)
			for _, name := range []string{"openapi.swagger.json", "openapi.json"} {
				data, err := os.ReadFile(filepath.Join(dir, "api", name))
				if err == nil {
					w.Header().Set("Content-Type", "application/json")
					w.Write(data)
					return
				}
			}
		}
		http.Error(w, "openapi.json not found. Run: make proto-openapi", http.StatusNotFound)
	}
}

// API приложение: HTTP + gRPC серверы (режим api).
type API struct {
	cfg     *config.Config
	httpSrv *http.Server
	grpcSrv *grpc.Server
	lis     net.Listener
}

// NewAPI создаёт приложение для режима api.
func NewAPI(cfg *config.Config) (*API, error) {
	if err := cfg.Validate(); err != nil {
		return nil, fmt.Errorf("config: %w", err)
	}
	if err := database.MigrateUp(cfg.DatabaseURL()); err != nil {
		return nil, fmt.Errorf("migrate: %w", err)
	}
	db, err := database.Open(cfg.DSN())
	if err != nil {
		return nil, fmt.Errorf("database: %w", err)
	}

	ticketSvc := service.NewTicketService(db)
	searchClient := searchindex.NewClient(cfg.SearchServiceURL)

	grpcAddr := cfg.AppHost + ":" + cfg.GRPCPort
	lis, err := net.Listen("tcp", grpcAddr)
	if err != nil {
		return nil, fmt.Errorf("grpc listen %s: %w (порт занят — остановите другой процесс или задайте GRPC_PORT в .env)", grpcAddr, err)
	}
	grpcSrv := grpc.NewServer()
	grpcImpl := grpcserver.NewServer(grpcserver.Deps{
		Ticket: ticketSvc,
		Search: searchClient,
	})
	ticket_service.RegisterTicketServiceServer(grpcSrv, grpcImpl)
	reflection.Register(grpcSrv)

	// Настройка grpc-gateway: возвращаем 201 Created для POST запросов на создание тикетов
	gatewayMux := runtime.NewServeMux(
		runtime.WithForwardResponseOption(func(ctx context.Context, w http.ResponseWriter, resp proto.Message) error {
			// Проверяем путь запроса из контекста и тип ответа
			pattern, ok := runtime.HTTPPathPattern(ctx)
			if ok && pattern == "/api/v1/tickets" {
				// Если ответ - это Ticket (результат CreateTicket), это POST запрос на создание
				if _, isTicket := resp.(*ticket_service.Ticket); isTicket {
					// Проверяем, что это не GET запрос (GetTicket тоже возвращает Ticket)
					// GetTicket имеет путь "/api/v1/tickets/{id}", а CreateTicket - "/api/v1/tickets"
					w.WriteHeader(http.StatusCreated)
				}
			}
			return nil
		}),
	)
	if err := ticket_service.RegisterTicketServiceHandlerServer(context.Background(), gatewayMux, grpcImpl); err != nil {
		return nil, fmt.Errorf("register grpc-gateway: %w", err)
	}

	mux := http.NewServeMux()
	mux.HandleFunc(paths.PathHealth, handler.Health)
	mux.HandleFunc(paths.PathReady, handler.Ready)
	mux.HandleFunc(paths.PathSwagger+"/openapi.json", serveOpenAPISpec())
	mux.Handle(paths.PathSwagger+"/", httpSwagger.Handler(
		httpSwagger.URL("openapi.json"),
		httpSwagger.DeepLinking(true),
		httpSwagger.DocExpansion("list"),
	))
	mux.Handle("/", gatewayMux)

	httpAddr := cfg.AppHost + ":" + cfg.HTTPPort
	httpSrv := &http.Server{
		Addr:              httpAddr,
		Handler:           mux,
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       15 * time.Second,
		WriteTimeout:      30 * time.Second,
		IdleTimeout:       60 * time.Second,
	}

	return &API{
		cfg:     cfg,
		httpSrv: httpSrv,
		grpcSrv: grpcSrv,
		lis:     lis,
	}, nil
}

// Run запускает HTTP и gRPC серверы, блокируется до отмены ctx.
func (a *API) Run(ctx context.Context) error {
	httpAddr := a.httpSrv.Addr
	grpcAddr := a.lis.Addr().String()
	host := a.cfg.AppHost
	if host == "0.0.0.0" {
		host = "localhost"
	}
	base := "http://" + host + ":" + a.cfg.HTTPPort
	log.Printf("HTTP server listening on %s", httpAddr)
	log.Printf("  Swagger UI:    %s/swagger", base)
	log.Printf("  Swagger spec:  %s/swagger/openapi.json", base)
	log.Printf("  Health:        %s/health", base)
	log.Printf("  Ready:         %s/ready", base)
	log.Printf("  API v1:        %s/api/v1/", base)
	log.Printf("gRPC server listening on %s", grpcAddr)
	log.Printf("  gRPC endpoint: %s (reflection enabled)", grpcAddr)

	go func() {
		if err := a.httpSrv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Printf("http: %v", err)
		}
	}()

	go func() {
		if err := a.grpcSrv.Serve(a.lis); err != nil {
			log.Printf("grpc: %v", err)
		}
	}()

	<-ctx.Done()
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := a.httpSrv.Shutdown(shutdownCtx); err != nil {
		return fmt.Errorf("http shutdown: %w", err)
	}
	a.grpcSrv.GracefulStop()
	return nil
}
