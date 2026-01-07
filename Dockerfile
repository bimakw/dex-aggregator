# Build stage
FROM golang:1.23-alpine AS builder

WORKDIR /app

# Install build dependencies
RUN apk add --no-cache git

# Copy go mod files
COPY go.mod go.sum ./
RUN go mod download

# Copy source code
COPY . .

# Build binary
RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-s -w" -o /app/bin/api ./cmd/api

# Runtime stage
FROM alpine:3.19

WORKDIR /app

# Install ca-certificates for HTTPS requests
RUN apk add --no-cache ca-certificates

# Copy binary from builder
COPY --from=builder /app/bin/api /app/api

# Expose port
EXPOSE 8080

# Set environment variables
ENV PORT=8080

# Run the binary
ENTRYPOINT ["/app/api"]
