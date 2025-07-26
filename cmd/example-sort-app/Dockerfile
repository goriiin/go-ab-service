# Stage 1: Build
FROM golang:1.24.5-alpine as builder
WORKDIR /app

COPY go.mod go.sum ./
COPY . /app/go-ab-service
WORKDIR /app/go-ab-service
RUN go mod download

COPY ./cmd/example-sort-app /app/cmd/example-sort-app
WORKDIR /app/cmd/example-sort-app
RUN CGO_ENABLED=0 GOOS=linux go build -o /app/example-sort-app .

# Stage 2: Runtime
FROM alpine:latest
WORKDIR /app
COPY --from=builder /app/example-sort-app .
CMD ["./example-sort-app"]