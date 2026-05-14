FROM golang:1.26.2-bookworm
WORKDIR /app

RUN apt-get update && \
    apt-get install -y git docker.io curl && \
    rm -rf /var/lib/apt/lists/*

RUN curl -sSL https://railpack.com/install.sh | RAILPACK_VERSION=0.23.0 sh -s -- --yes

COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN GOOS=linux GOARCH=amd64 go build -ldflags='-s' -o=./bin/linux_amd64/api ./cmd/api

EXPOSE 8080