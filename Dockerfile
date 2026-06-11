# Use official Go image (or Node, Python, whatever you use)
FROM golang:1.24-alpine

# Set working directory
WORKDIR /app

# Copy go.mod & go.sum first to leverage caching
COPY go.mod go.sum ./
RUN go mod download

# Copy all source files
COPY . .

# Build the app
RUN go build -o server .

# Expose the port Cloud Run expects
EXPOSE 8080

# Run the server
CMD ["./server"]
