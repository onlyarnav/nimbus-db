FROM golang:1.25-alpine AS builder
WORKDIR /app

COPY services/observability ./services/observability
COPY services/auth-service ./services/auth-service
COPY services/worker-node ./services/worker-node

WORKDIR /app/services/worker-node
RUN CGO_ENABLED=0 go build -o /app/worker-node-service .

FROM alpine:latest
RUN adduser -D -u 10001 appuser
WORKDIR /home/appuser
COPY --from=builder /app/worker-node-service .
USER appuser
EXPOSE 50053 8085
ENTRYPOINT ["./worker-node-service"]
