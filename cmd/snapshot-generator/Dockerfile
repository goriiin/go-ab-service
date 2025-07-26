# Stage 1: Build
FROM golang:1.24.5-alpine as builder
WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download
COPY cmd/snapshot-generator/main.go ./main.go
COPY internal/ ./internal/
COPY pkg/ ./pkg/
RUN go build -o snapshot-generator main.go

# Stage 2: Runtime
FROM alpine:latest
WORKDIR /app
COPY --from=builder /app/snapshot-generator ./snapshot-generator
CMD ["./snapshot-generator"]
