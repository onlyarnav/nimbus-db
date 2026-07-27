FROM golang:1.24-alpine AS builder
WORKDIR /app

COPY services/observability ./services/observability
COPY services/auth-service ./services/auth-service
COPY services/capacity-planner ./services/capacity-planner

WORKDIR /app/services/capacity-planner
RUN CGO_ENABLED=0 go build -o /app/capacity-planner-service .

FROM alpine:latest
RUN adduser -D -u 10001 appuser
WORKDIR /home/appuser
COPY --from=builder /app/capacity-planner-service .
USER appuser
EXPOSE 50059 8089
ENTRYPOINT ["./capacity-planner-service"]
