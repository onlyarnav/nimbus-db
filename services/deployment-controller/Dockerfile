FROM golang:1.25-alpine AS builder
WORKDIR /app

COPY services/observability ./services/observability
COPY services/auth-service ./services/auth-service
COPY services/metadata-service ./services/metadata-service
COPY services/worker-node ./services/worker-node
COPY services/deployment-controller ./services/deployment-controller

WORKDIR /app/services/deployment-controller
RUN go mod tidy && CGO_ENABLED=0 go build -o /app/deployment-controller-service .

FROM alpine:latest
RUN adduser -D -u 10001 appuser
WORKDIR /home/appuser
COPY --from=builder /app/deployment-controller-service .
USER appuser
EXPOSE 50058 8088
ENTRYPOINT ["./deployment-controller-service"]
