.PHONY: help build run run-dev clean tidy vet fmt health-check migrate proto proto-build proto-generate proto-generate-local proto-generate-docker proto-openapi install-deps update docker-build docker-compose-up docker-compose-down

APP_NAME = ticket-service
CMD_PATH = ./cmd/ticket-service
BIN_DIR = bin
PORT = 8097
PROTOC_IMAGE = local/protoc-go:latest
PROTO_ROOT = pkg/ticket_service
PROTO_FILE = ticket.proto
GEN_DIR = pkg/gen/ticket_service
GO_MODULE = github.com/psds-microservice/ticket-service
OPENAPI_OUT = api
.DEFAULT_GOAL := help

help:
	@echo "ticket-service"
	@echo "  make build run run-dev clean tidy vet fmt health-check docker-build docker-compose-up"
	@echo "  make migrate  - Run database migrations"
	@echo "  make proto / proto-generate / proto-openapi  - as in user-service"
	@echo "  make install-deps / update"
	@echo "  HTTP Port: $(PORT)  gRPC Port: 9097"
	@echo "  Health: http://localhost:$(PORT)/health  Swagger: http://localhost:$(PORT)/swagger"

build:
	@mkdir -p $(BIN_DIR)
	go build -o $(BIN_DIR)/$(APP_NAME) $(CMD_PATH)
	@echo "OK: $(BIN_DIR)/$(APP_NAME)"

run: build
	@cd $(BIN_DIR) && ./$(APP_NAME) api

run-dev:
	go run $(CMD_PATH) api

health-check:
	@curl -sf http://localhost:$(PORT)/health && echo " OK" || echo " FAIL"

migrate:
	@go run $(CMD_PATH) migrate up

vet:
	go vet ./...

fmt:
	go fmt ./...

clean:
	rm -rf $(BIN_DIR)
	go clean

tidy:
	go mod tidy

install-deps:
	@echo "Installing dependencies..."
	go mod download
	go install github.com/grpc-ecosystem/grpc-gateway/v2/protoc-gen-grpc-gateway@latest
	go install github.com/grpc-ecosystem/grpc-gateway/v2/protoc-gen-openapiv2@latest
	@echo "Dependencies installed"

update:
	@echo "Updating dependencies..."
	go get -u ./... github.com/psds-microservice/helpy \
		github.com/psds-microservice/infra \
		github.com/psds-microservice/search-service
	go mod tidy
	go mod vendor
	$(MAKE) proto
	@$(MAKE) proto-openapi 2>/dev/null || true
	@echo "Dependencies updated"

INFRA_VENDOR := vendor/github.com/psds-microservice/infra
proto: proto-build proto-generate
proto-build:
	@echo "Building protoc-go image (from $(INFRA_VENDOR))..."
	@docker build -t $(PROTOC_IMAGE) -f $(INFRA_VENDOR)/protoc-go.Dockerfile $(INFRA_VENDOR)
proto-generate:
	@PATH="$$(go env GOPATH 2>/dev/null)/bin:$$PATH"; if command -v protoc >/dev/null 2>&1 && command -v protoc-gen-go >/dev/null 2>&1 && command -v protoc-gen-go-grpc >/dev/null 2>&1; then $(MAKE) proto-generate-local; else $(MAKE) proto-generate-docker; fi
proto-generate-local:
	@mkdir -p $(GEN_DIR) $(OPENAPI_OUT); command -v protoc-gen-grpc-gateway >/dev/null 2>&1 || (echo "Install protoc-gen-grpc-gateway" && exit 1); PATH="$$(go env GOPATH)/bin:$$PATH"; protoc -I $(PROTO_ROOT) -I third_party --go_out=. --go_opt=module=$(GO_MODULE) --go-grpc_out=. --go-grpc_opt=module=$(GO_MODULE) --grpc-gateway_out=. --grpc-gateway_opt=module=$(GO_MODULE) $(PROTO_ROOT)/$(PROTO_FILE); echo "OK: $(GEN_DIR)"
proto-generate-docker:
	@mkdir -p $(GEN_DIR); docker run --rm -v "$(CURDIR):/workspace" -w /workspace --entrypoint sh $(PROTOC_IMAGE) -c "protoc -I $(PROTO_ROOT) -I third_party -I /include --go_out=. --go_opt=module=$(GO_MODULE) --go-grpc_out=. --go-grpc_opt=module=$(GO_MODULE) --grpc-gateway_out=. --grpc-gateway_opt=module=$(GO_MODULE) $(PROTO_ROOT)/$(PROTO_FILE)" || (echo "Run make proto-build or install protoc+plugins" && exit 1)
proto-openapi:
	@command -v protoc >/dev/null 2>&1 || (echo "Install protoc" && exit 1); command -v protoc-gen-openapiv2 >/dev/null 2>&1 || (echo "Install protoc-gen-openapiv2" && exit 1); mkdir -p $(OPENAPI_OUT); PATH="$$(go env GOPATH)/bin:$$PATH"; protoc -I $(PROTO_ROOT) -I third_party --openapiv2_out=$(OPENAPI_OUT) --openapiv2_opt=logtostderr=true --openapiv2_opt=allow_merge=true --openapiv2_opt=merge_file_name=openapi $(PROTO_ROOT)/$(PROTO_FILE); if [ -f $(OPENAPI_OUT)/openapi.swagger.json ]; then cp $(OPENAPI_OUT)/openapi.swagger.json $(OPENAPI_OUT)/openapi.json; fi

docker-build:
	docker build -f deployments/Dockerfile -t $(APP_NAME):latest .

docker-compose-up:
	docker compose -f deployments/docker-compose.yml up -d

docker-compose-down:
	docker compose -f deployments/docker-compose.yml down
