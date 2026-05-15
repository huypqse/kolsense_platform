# ─── Build stage ───────────────────────────────────────────────────────────────
FROM golang:1.23-alpine AS builder

RUN apk add --no-cache git ca-certificates

WORKDIR /app

COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-s -w" -o /app/bin/api ./cmd/api

# ─── Runtime stage ─────────────────────────────────────────────────────────────
FROM gcr.io/distroless/static:nonroot

WORKDIR /app

COPY --from=builder /app/bin/api /app/api
COPY --from=builder /app/internal/prompt/templates /app/internal/prompt/templates

EXPOSE 8080

ENTRYPOINT ["/app/api"]
