FROM golang:1.21 AS builder

WORKDIR /app

COPY backend/go.mod backend/go.sum ./backend/
COPY frontend/ ./frontend/

WORKDIR /app/backend
RUN go mod download

COPY backend/ ./

ENV GOPROXY=https://proxy.golang.org,direct
RUN CGO_ENABLED=0 go build -o server .

FROM debian:bookworm-slim

RUN apt-get update && apt-get install -y ca-certificates && rm -rf /var/lib/apt/lists/*

WORKDIR /app

COPY --from=builder /app/backend/server ./server
COPY --from=builder /app/frontend ./frontend

RUN mkdir -p /app/data

EXPOSE 8080

CMD ["./server"]
