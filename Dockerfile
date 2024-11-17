FROM golang:1.23-alpine AS builder

WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -a -installsuffix cgo -o main .

FROM alpine:3.20

WORKDIR /root/

COPY --from=builder /app/main .

COPY --from=builder /app/public ./public

CMD ["./main"]
