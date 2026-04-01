FROM golang:1.22-alpine AS builder
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build \
    -ldflags="-s -w -X main.version=$(git describe --tags --always 2>/dev/null || echo dev)" \
    -o /bin/corral ./cmd/corral/

FROM alpine:3.19
RUN apk add --no-cache ca-certificates tzdata curl

COPY --from=builder /bin/corral /usr/local/bin/corral

# Environment variables — override at runtime
# DATA_DIR should be backed by a persistent volume in production
ENV PORT="8760" \
    DATA_DIR="/data" \
    RETENTION_DAYS="30" \
    CORRAL_LICENSE_KEY=""

EXPOSE 8760

# Healthcheck — adjust interval for your use case
HEALTHCHECK --interval=30s --timeout=5s --start-period=10s --retries=3 \
    CMD curl -sf http://localhost:8760/health || exit 1

ENTRYPOINT ["corral"]
