FROM golang:1.25-alpine AS builder
WORKDIR /app
COPY services/observability/ ./services/observability/
WORKDIR /app/services/observability/webhook
RUN go build -o webhook_receiver .

FROM alpine:latest
WORKDIR /root/
COPY --from=builder /app/services/observability/webhook/webhook_receiver .
EXPOSE 9099
CMD ["./webhook_receiver"]
