FROM golang:1.25-alpine AS builder
WORKDIR /app
COPY . .
RUN go build -o /deployment-controller ./services/deployment-controller

FROM alpine:latest
WORKDIR /root/
COPY --from=builder /deployment-controller .
EXPOSE 50058 8088
CMD ["./deployment-controller"]
