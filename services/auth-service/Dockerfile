FROM golang:1.25-alpine AS builder
WORKDIR /app
COPY . .
RUN go build -o /auth-service ./services/auth-service

FROM alpine:latest
WORKDIR /root/
COPY --from=builder /auth-service .
EXPOSE 8087
CMD ["./auth-service"]
