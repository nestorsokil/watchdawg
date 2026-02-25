FROM golang:1.25-alpine AS builder

WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN go build -o bin/watchdawg ./cmd/watchdawg

FROM alpine:3.21

WORKDIR /app
COPY --from=builder /app/bin/watchdawg ./watchdawg

ENTRYPOINT ["./watchdawg"]
