FROM golang:1.22-alpine AS builder
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -ldflags="-s -w -X main.version=$(git describe --tags --always 2>/dev/null || echo dev)" \
    -o /bin/corral ./cmd/corral/

FROM alpine:3.19
RUN apk add --no-cache ca-certificates tzdata
COPY --from=builder /bin/corral /usr/local/bin/corral
ENV PORT=8760 \
    DATA_DIR=/data \
    RETENTION_DAYS=30
EXPOSE 8760
ENTRYPOINT ["corral"]
