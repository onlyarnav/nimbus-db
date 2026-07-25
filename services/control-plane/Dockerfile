FROM golang:1.25-alpine AS builder
WORKDIR /app
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -o control-plane .

FROM alpine:latest
WORKDIR /app
COPY --from=builder /app/control-plane .
EXPOSE 8082
CMD ["./control-plane"]
