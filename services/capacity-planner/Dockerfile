FROM golang:1.25-alpine AS builder
WORKDIR /app
COPY . .
RUN go build -o /capacity-planner ./services/capacity-planner

FROM alpine:latest
WORKDIR /root/
COPY --from=builder /capacity-planner .
EXPOSE 50059 8089
CMD ["./capacity-planner"]
