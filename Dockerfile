FROM golang:1.18-alpine AS builder

WORKDIR /app

RUN apk add --no-cache git

COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -a -installsuffix cgo -o main .

FROM alpine:3.14

WORKDIR /root/

COPY --from=builder /app/main .

COPY --from=builder /app/public ./public

CMD ["./main"]
