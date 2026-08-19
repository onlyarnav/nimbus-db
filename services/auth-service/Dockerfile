FROM golang:1.25-alpine AS builder
WORKDIR /app

COPY services/observability ./services/observability
COPY services/auth-service ./services/auth-service

WORKDIR /app/services/auth-service
RUN go mod tidy && CGO_ENABLED=0 go build -o /app/auth-service .

FROM alpine:latest
RUN adduser -D -u 10001 appuser
WORKDIR /home/appuser
COPY --from=builder /app/auth-service .
USER appuser
EXPOSE 8087
ENTRYPOINT ["./auth-service"]
