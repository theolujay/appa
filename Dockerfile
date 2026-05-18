FROM golang:1.26-alpine AS builder

WORKDIR /src

COPY go.mod go.sum ./

RUN go mod download

COPY . .

RUN CGO_ENABLED=0 go build -ldflags='-w -s' -o=/usr/bin/appa ./cmd/api

FROM debian:bookworm-slim

RUN apt-get update && \
    apt-get install -y --no-install-recommends git docker.io curl tar ca-certificates && \
    rm -rf /var/lib/apt/lists/*

RUN curl -sSL https://railpack.com/install.sh | RAILPACK_VERSION=0.23.0 sh -s -- -y

COPY --from=builder /usr/bin/appa /usr/bin/appa