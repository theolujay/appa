FROM golang:1.26-alpine AS builder

WORKDIR /src

COPY go.mod go.sum ./

RUN go mod download

COPY . .

RUN CGO_ENABLED=0 go build -ldflags='-w -s' -o=/usr/bin/appa ./cmd/api

FROM debian:bookworm-slim

ARG RAILPACK_VERSION
# Make available at runtime if ran alone (outside compose.yml)
ENV RAILPACK_VERSION=${RAILPACK_VERSION}

RUN apt-get update && \
    apt-get install -y --no-install-recommends git docker.io curl tar ca-certificates && \
    rm -rf /var/lib/apt/lists/*

RUN curl -fsSL https://github.com/moby/buildkit/releases/download/v0.30.0/buildkit-v0.30.0.linux-amd64.tar.gz \
    | tar xz -C /usr/bin --strip-components=1 bin/buildctl

RUN curl -sSL https://railpack.com/install.sh | RAILPACK_VERSION=$RAILPACK_VERSION sh -s -- -y

COPY --from=builder /usr/bin/appa /usr/bin/appa

COPY migrations /migrations

RUN curl -fsSL https://github.com/golang-migrate/migrate/releases/download/v4.19.1/migrate.linux-amd64.tar.gz \
    | tar xz -C /usr/bin

COPY scripts/entrypoint.sh /usr/bin/entrypoint.sh
RUN chmod +x /usr/bin/entrypoint.sh

ENTRYPOINT ["/usr/bin/entrypoint.sh"]
CMD ["/usr/bin/appa"]