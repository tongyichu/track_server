# Multi-stage Dockerfile for Hertz-based Go server
FROM node:22-bookworm-slim AS admin-frontend
WORKDIR /src/internal/admin/frontend
COPY internal/admin/frontend/package*.json ./
RUN npm ci --no-audit --no-fund
COPY internal/admin/frontend/ ./
RUN npm run build

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
COPY --from=admin-frontend /src/internal/admin/static/workbench ./internal/admin/static/workbench

# Build the server binary
RUN go build -o /out/hertz-track ./cmd/server

# Runtime stage using debian slim with bash support
FROM debian:12-slim

WORKDIR /app

ENV TZ=Asia/Shanghai

RUN apt-get update \
    && DEBIAN_FRONTEND=noninteractive apt-get install -y --no-install-recommends curl procps tzdata webp \
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
