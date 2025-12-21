FROM golang:1-alpine AS builder

WORKDIR /app

COPY go.mod ./
COPY main.go ./

RUN CGO_ENABLED=0 GOOS=linux go build -a -installsuffix cgo -o generator .

FROM alpine:3.22

WORKDIR /root/

COPY --from=builder /app/generator .

CMD ["./generator"]
