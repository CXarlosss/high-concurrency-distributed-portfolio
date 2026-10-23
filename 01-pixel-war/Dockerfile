# Stage 1: Build binary
FROM golang:1.23-alpine AS builder

WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-s -w" -o pixelwar-server ./cmd/server

# Stage 2: Minimal runtime image (<18 MB)
FROM alpine:3.20

WORKDIR /app
RUN apk --no-cache add ca-certificates

COPY --from=builder /app/pixelwar-server .
COPY --from=builder /app/web ./web

EXPOSE 8080

CMD ["./pixelwar-server"]
