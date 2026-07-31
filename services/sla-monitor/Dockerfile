FROM golang:1.25-alpine AS builder
WORKDIR /app

COPY services/observability ./services/observability
COPY services/auth-service ./services/auth-service
COPY services/sla-monitor ./services/sla-monitor

WORKDIR /app/services/sla-monitor
RUN CGO_ENABLED=0 go build -o /app/sla-monitor-service .

FROM alpine:latest
RUN adduser -D -u 10001 appuser
WORKDIR /home/appuser
COPY --from=builder /app/sla-monitor-service .
USER appuser
EXPOSE 50060 8090
ENTRYPOINT ["./sla-monitor-service"]
