# syntax=docker/dockerfile:1.7

FROM golang:1.26-alpine AS builder
WORKDIR /src
ENV CGO_ENABLED=0 GOOS=linux GOARCH=amd64

COPY go.mod go.sum ./
RUN --mount=type=cache,target=/go/pkg/mod \
	--mount=type=cache,target=/root/.cache/go-build \
	go mod download

COPY fileService ./fileService
RUN --mount=type=cache,target=/go/pkg/mod \
	--mount=type=cache,target=/root/.cache/go-build \
	go build -trimpath -ldflags "-s -w" -o /out/app ./fileService/cmd/file

FROM alpine:3.20
WORKDIR /app
RUN adduser -D -H -u 10001 appuser
COPY --from=builder /out/app /app/app
USER appuser

EXPOSE 18086
ENTRYPOINT ["/app/app"]