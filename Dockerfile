FROM golang:1.22-alpine AS builder

RUN apk add --no-cache gcc musl-dev

WORKDIR /app

COPY go.mod ./
RUN go mod download

COPY . .

RUN go mod tidy
RUN GOGC=off GOMAXPROCS=1 CGO_ENABLED=1 GOOS=linux go build -p 1 -ldflags="-s -w" -o server ./cmd/server

FROM alpine:3.20

RUN apk add --no-cache ca-certificates tzdata

WORKDIR /app

COPY --from=builder /app/server .

RUN mkdir -p /app/data

EXPOSE 3000

CMD ["./server"]
