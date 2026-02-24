# syntax=docker/dockerfile:1

# Use the official Golang image to create a build artifact.
FROM golang:1.25-alpine AS builder

# Instruct Docker to use this directory as the default destination for all subsequent commands
# Set destination for COPY and ADD commands
WORKDIR /app


# Copy the go.mod and go.sum file into the project directory /app which, 
# owing to my use of WORKDIR
COPY go.mod go.sum ./

# Download Go modules including dependencies into a directory inside the image
RUN go mod download

# Copy the source code. This copies all files in the current directory on the host machine
# to the working directory inside the image.
COPY *.go ./

# Build the Go application
# CGO_ENABLED=0 disables CGO, ensuring the binary is statically linked
# GOOS=linux ensures the binary is built for Linux OS
# The output binary is named "productdocker" and placed in the app directory of the image
RUN CGO_ENABLED=0 GOOS=linux go build -o productdocker

# Phase 2: Create a smaller image to run the application
FROM alpine:latest
WORKDIR /app

COPY --from=builder /app/productdocker .

EXPOSE 8080

# Tell Docker to run the compiled binary when the container starts with CMD command
CMD ["./productdocker"]