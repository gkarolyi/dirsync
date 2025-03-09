FROM golang:1.21-alpine AS builder

# Install build dependencies
RUN apk add --no-cache git

WORKDIR /build

# Copy source files
COPY src/*.go ./
COPY static/ ./static/

# Create a sample config.json if it doesn't exist
RUN echo '{"sync_interval": 60, "sync_pairs": ["/app/data/source:/app/data/destination"], "port": ":8080"}' > config.json

# Initialize Go module and build the application
RUN go mod init dirsync && \
    go mod tidy && \
    go build -o dirsync .

# Use a smaller image for the final application
FROM alpine:latest

# Install rsync for future use
RUN apk add --no-cache rsync ca-certificates tzdata

# Set up working directory
WORKDIR /app

# Copy the binary from the builder stage
COPY --from=builder /build/dirsync /app/
COPY --from=builder /build/static/ /app/static/
COPY --from=builder /build/config.json /app/

# Create directories for sync
RUN mkdir -p /app/data/source /app/data/destination

# Add some test files to the source directory
RUN echo "Test file 1" > /app/data/source/file1.txt && \
    echo "Test file 2" > /app/data/source/file2.txt && \
    mkdir -p /app/data/source/subdir && \
    echo "Test file in subdirectory" > /app/data/source/subdir/file3.txt

# Expose the port the app runs on
EXPOSE 8080

# Set environment variables
ENV TZ=UTC

# Command to run the executable
CMD ["./dirsync"]