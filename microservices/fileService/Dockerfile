# syntax=docker/dockerfile:1

FROM golang:1.26-alpine AS builder
WORKDIR /src

COPY go.mod go.sum ./
RUN go mod download

COPY fileService ./fileService
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o /out/app ./fileService/cmd/file

FROM alpine:3.20
WORKDIR /app
RUN adduser -D -H -u 10001 appuser
COPY --from=builder /out/app /app/app
USER appuser

EXPOSE 18086
ENTRYPOINT ["/app/app"]