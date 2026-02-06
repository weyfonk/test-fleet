# --- Build Stage ---
FROM golang:1.24-alpine AS builder

WORKDIR /app

# Copy module files (go.mod and go.sum) and download dependencies
COPY go.* ./
RUN go mod download

# Copy source code and build
COPY *.go ./
RUN CGO_ENABLED=0 GOOS=linux go build -o /fake-slow-registry

# --- Final Stage ---
FROM alpine:latest

WORKDIR /

# Copy binary from builder
COPY --from=builder /fake-slow-registry /fake-slow-registry

# Expose the port used by the application
EXPOSE 4242

# Run the binary
ENTRYPOINT ["/fake-slow-registry", "--simulate-index-duration=10s"]
