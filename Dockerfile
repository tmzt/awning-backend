FROM golang:1.24.1-alpine AS builder

WORKDIR /app

COPY go.mod go.sum* ./

RUN go mod download

COPY . .

RUN go build -o api-server .

FROM alpine:latest

RUN apk --no-cache add ca-certificates

RUN mkdir -p /app/public /app/data

WORKDIR /app

COPY --from=builder /app/api-server .
# COPY --from=builder /app/data/system.txt ./data/
COPY ./data/system.txt ./data/

VOLUME /app/public

EXPOSE 3000

CMD ["./api-server"]