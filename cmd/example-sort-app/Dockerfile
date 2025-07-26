# Stage 1: Build
FROM golang:1.24.5-alpine as builder
WORKDIR /app

# 1. Копируем файлы модуля для кеширования зависимостей
COPY go.mod go.sum ./
RUN go mod download

COPY . .

RUN CGO_ENABLED=0 GOOS=linux go build -o /app/example-sort-app ./cmd/example-sort-app

# Stage 2: Runtime
FROM alpine:latest
WORKDIR /app
COPY --from=builder /app/example-sort-app .
CMD ["./example-sort-app"]