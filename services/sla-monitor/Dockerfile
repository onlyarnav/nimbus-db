FROM golang:1.25-alpine AS builder
WORKDIR /app
COPY . .
RUN go build -o /sla-monitor ./services/sla-monitor

FROM alpine:latest
WORKDIR /root/
COPY --from=builder /sla-monitor .
EXPOSE 50060 8090
CMD ["./sla-monitor"]
