# Stage 1: Build
FROM golang:1.24-alpine AS builder

WORKDIR /app

# Установка необходимых пакетов для сборки
RUN apk add --no-cache ca-certificates

COPY go.mod go.sum ./
RUN go mod download

COPY . .
ARG TARGETARCH
RUN CGO_ENABLED=0 GOOS=linux GOARCH=${TARGETARCH:-amd64} go build \
    -ldflags="-s -w" \
    -trimpath \
    -o service ./cmd/bot

# Stage 2: Production runtime
FROM gcr.io/distroless/static-debian12:nonroot AS final

WORKDIR /app

# Копируем бинарник
COPY --from=builder /app/service .
COPY --from=builder /app/api/ ./api/

# Копируем конфигурацию (опционально, только если нужно)
COPY --from=builder /app/config/ ./config/

EXPOSE 8080
CMD ["/app/service"]
