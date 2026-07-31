FROM golang:1.25-alpine AS builder
WORKDIR /app

COPY services/observability ./services/observability
COPY services/auth-service ./services/auth-service
COPY services/metadata-service ./services/metadata-service
COPY services/scheduler ./services/scheduler

WORKDIR /app/services/scheduler
RUN CGO_ENABLED=0 go build -o /app/scheduler-service .

FROM alpine:latest
RUN adduser -D -u 10001 appuser
WORKDIR /home/appuser
COPY --from=builder /app/scheduler-service .
USER appuser
EXPOSE 50052 9091
ENTRYPOINT ["./scheduler-service"]
