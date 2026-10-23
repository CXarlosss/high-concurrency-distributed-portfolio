FROM golang:1.23-alpine AS builder

WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download

COPY . .

# Compile server binary
RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-s -w" -o uber-server ./cmd/server

FROM alpine:3.20
WORKDIR /app
RUN apk --no-cache add ca-certificates
COPY --from=builder /app/uber-server .
COPY --from=builder /app/web ./web

EXPOSE 8082
CMD ["./uber-server"]
