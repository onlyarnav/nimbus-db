FROM golang:1.25-alpine AS builder
WORKDIR /app

COPY services/observability ./services/observability
COPY services/auth-service ./services/auth-service
COPY services/metadata-service ./services/metadata-service
COPY services/control-plane ./services/control-plane

WORKDIR /app/services/control-plane
RUN go mod tidy && CGO_ENABLED=0 go build -o /app/control-plane-service .

FROM alpine:latest
RUN adduser -D -u 10001 appuser
WORKDIR /home/appuser
COPY --from=builder /app/control-plane-service .
USER appuser
EXPOSE 8081
ENTRYPOINT ["./control-plane-service"]
