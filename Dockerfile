FROM golang:1.22-alpine AS builder

# Install build dependencies
RUN apk add --no-cache git

# Set working directory
WORKDIR /app

# Copy go mod files
COPY go.mod go.sum ./

# Download dependencies
RUN go mod download

# Copy source code
COPY . .

# Build the application
RUN CGO_ENABLED=0 GOOS=linux go build -o reservoir ./cmd/server

# Final stage
FROM alpine:3.19

# Install CA certificates for HTTPS requests
RUN apk --no-cache add ca-certificates tzdata

# Create non-root user
RUN addgroup -g 1000 reservoir && \
    adduser -D -u 1000 -G reservoir reservoir

# Set working directory
WORKDIR /home/reservoir

# Copy binary from builder
COPY --from=builder /app/reservoir .

# Change ownership
RUN chown -R reservoir:reservoir /home/reservoir

# Switch to non-root user
USER reservoir

# Expose port
EXPOSE 8080

# Health check
HEALTHCHECK --interval=30s --timeout=5s --start-period=5s --retries=3 \
    CMD wget --no-verbose --tries=1 --spider http://localhost:8080/health || exit 1

# Run the application
CMD ["./reservoir"]
