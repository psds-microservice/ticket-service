# No replace, no go.work: inject helpy and infra into GOMODCACHE.
FROM golang:1.26-alpine AS builder
ENV GOMODCACHE=/go/pkg/mod
WORKDIR /app
COPY helpy /go/pkg/mod/github.com/psds-microservice/helpy@v0.1.0
COPY infra /go/pkg/mod/github.com/psds-microservice/infra@v0.0.3
COPY ticket-service ./ticket-service
WORKDIR /app/ticket-service
RUN go mod download
RUN CGO_ENABLED=0 go build -ldflags="-s -w" -o /ticket-service ./cmd/ticket-service

FROM alpine:3.19
RUN apk --no-cache add ca-certificates
WORKDIR /app
COPY --from=builder /ticket-service .
COPY --from=builder /app/ticket-service/database ./database
EXPOSE 8095
CMD ["./ticket-service", "api"]
