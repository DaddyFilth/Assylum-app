# Build stage
FROM golang:1.21-alpine AS builder

WORKDIR /app

# Copy go module files first for better caching
COPY backend/go.mod backend/go.sum ./backend/

WORKDIR /app/backend
RUN go mod download

# Copy the rest of the source
WORKDIR /app
COPY backend/ ./backend/
COPY frontend/ ./frontend/

# Build the binary
WORKDIR /app/backend
RUN CGO_ENABLED=0 go build -o server .

# Runtime stage
FROM alpine:latest

RUN apk --no-cache add ca-certificates

WORKDIR /app

COPY --from=builder /app/backend/server ./server
COPY --from=builder /app/frontend ./frontend

RUN mkdir -p /app/data

EXPOSE 8080

CMD ["./server"]
