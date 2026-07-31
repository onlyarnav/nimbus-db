FROM golang:1.25-alpine AS builder
WORKDIR /app

COPY services/observability ./services/observability
COPY services/auth-service ./services/auth-service
COPY services/metadata-service ./services/metadata-service

WORKDIR /app/services/metadata-service
RUN CGO_ENABLED=0 go build -o /app/metadata-service .

FROM alpine:latest
RUN adduser -D -u 10001 appuser
WORKDIR /home/appuser
COPY --from=builder /app/metadata-service .
COPY --from=builder /app/services/metadata-service/db/migrations ./db/migrations
USER appuser
EXPOSE 8080 50051
ENTRYPOINT ["./metadata-service"]
