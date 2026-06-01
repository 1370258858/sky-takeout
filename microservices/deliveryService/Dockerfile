# syntax=docker/dockerfile:1

FROM golang:1.26-alpine AS builder
WORKDIR /src

COPY go.mod go.sum ./
RUN go mod download

COPY deliveryService ./deliveryService
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o /out/app ./deliveryService/cmd/delivery

FROM alpine:3.20
WORKDIR /app
RUN adduser -D -H -u 10001 appuser
COPY --from=builder /out/app /app/app
USER appuser

EXPOSE 18085
ENTRYPOINT ["/app/app"]