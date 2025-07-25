# Stage 1: Build
FROM golang:1.23.4-alpine as builder
WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download
COPY cmd/outbox-worker/main.go ./main.go
COPY internal/ ./internal/
COPY pkg/ ./pkg/
RUN go build -o outbox-worker main.go

# Stage 2: Runtime
FROM alpine:latest
WORKDIR /app
COPY --from=builder /app/outbox-worker ./outbox-worker
CMD ["./outbox-worker"]
