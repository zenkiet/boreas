# syntax=docker/dockerfile:1

# Build a static Linux binary. Boreas has no cgo dependencies.
FROM golang:1.26-bookworm AS go-builder

WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build \
    -trimpath -ldflags="-s -w" \
    -o /out/boreas ./cmd/boreas

# Boreas runs as root so it can use the mounted Docker daemon socket. Keep the
# runtime otherwise minimal while retaining trusted CA certificates.
FROM alpine:3.22

RUN apk add --no-cache ca-certificates
WORKDIR /app

COPY --from=go-builder /out/boreas /app/boreas

EXPOSE 8080

ENTRYPOINT ["/app/boreas"]
