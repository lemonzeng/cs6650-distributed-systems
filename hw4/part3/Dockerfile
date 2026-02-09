# Use the official Go image to build the binary
FROM golang:1.25.5 as builder
WORKDIR /app

# Copy the manifest files first for better caching
COPY go.mod go.sum ./
RUN go mod download

# Copy the rest of the code and build it
# CGO_ENABLED=0 disables CGO, ensuring the binary is statically linked
# GOOS=linux ensures the binary is built for Linux OS
# The output binary is named "mapreducerdocker"
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -o /mapreducerdocker

# Use a tiny image to actually run the app
FROM alpine:latest
WORKDIR /root/
# Copy the compiled binary from the builder stage
COPY --from=builder /mapreducerdocker /mapreducerdocker

# Document that the application listens on port 8080 by default
EXPOSE 8080

# Tell Docker to run the compiled binary when the container starts
CMD ["/mapreducerdocker"]