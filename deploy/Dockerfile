# Multi-stage Dockerfile for Hertz-based Go server
# Builder stage
FROM golang:1.22-bullseye AS builder

WORKDIR /src

# Build statically linked binary for Linux
ENV CGO_ENABLED=0
ENV GOOS=linux

# Cache dependencies
COPY go.mod go.sum ./
RUN go mod download

# Copy the rest of the source code
COPY . .

# Build the server binary
RUN go build -o /out/hertz-track ./cmd/server

# Runtime stage using debian slim with bash support
FROM debian:12-slim

WORKDIR /app

ENV TZ=Asia/Shanghai

RUN apt-get update \
    && DEBIAN_FRONTEND=noninteractive apt-get install -y --no-install-recommends curl procps tzdata \
    && apt-get install -y --no-install-recommends ca-certificates \
    && ln -snf /usr/share/zoneinfo/$TZ /etc/localtime \
    && echo $TZ > /etc/timezone \
    && mkdir -p /var/log/track_server \
    && rm -rf /var/lib/apt/lists/*

COPY --from=builder /out/hertz-track /app/hertz-track

# Default listening port
ENV SERVER_ADDR=:8080
ENV LOG_DIR=/var/log/track_server

EXPOSE 8080 8443

ENTRYPOINT ["/app/hertz-track"]
