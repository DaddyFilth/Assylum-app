FROM golang:1.23-alpine AS builder

WORKDIR /app

COPY backend/ ./backend/
COPY frontend/ ./frontend/

WORKDIR /app/backend
RUN go mod tidy
RUN CGO_ENABLED=0 go build -o server .

FROM alpine:latest

RUN apk --no-cache add ca-certificates

WORKDIR /app

COPY --from=builder /app/backend/server ./server
COPY --from=builder /app/frontend ./frontend

RUN mkdir -p /app/data

EXPOSE 8080

CMD ["./server"]
