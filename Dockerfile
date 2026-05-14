FROM golang:1.26.2-bookworm
WORKDIR /app

RUN apt-get update && \
    apt-get install -y git docker.io curl && \
    rm -rf /var/lib/apt/lists/*

RUN curl -sSL https://railpack.com/install.sh | RAILPACK_VERSION=0.23.0 sh -s -- --yes

COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN go build -o server ./cmd/server

EXPOSE 8080