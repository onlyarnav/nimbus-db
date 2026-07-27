FROM golang:1.24-alpine AS builder
WORKDIR /app

# Copy shared replace modules and gateway service files
COPY services/observability ./services/observability
COPY services/auth-service ./services/auth-service
COPY services/metadata-service ./services/metadata-service
COPY services/gateway ./services/gateway

WORKDIR /app/services/gateway
RUN CGO_ENABLED=0 go build -o /app/gateway-service .

FROM alpine:latest
RUN adduser -D -u 10001 appuser
WORKDIR /home/appuser
COPY --from=builder /app/gateway-service .
USER appuser
EXPOSE 8080
ENTRYPOINT ["./gateway-service"]
