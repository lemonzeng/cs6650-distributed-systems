# Multi-stage build:
# Stage 1: compile the Go binary
# Stage 2: run with a minimal image to reduce image size
# Smaller images pull faster on ECS Fargate, which shortens cold-start time

FROM golang:1.25 AS builder

WORKDIR /app

# Copy go.mod and go.sum first to leverage Docker layer caching
# Dependencies are only re-downloaded when these files change
COPY go.mod go.sum ./
RUN go mod download

# Copy source and build
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -o product-search .

# Stage 2: minimal runtime image
FROM alpine:latest

WORKDIR /app

COPY --from=builder /app/product-search .

EXPOSE 8080

CMD ["./product-search"]