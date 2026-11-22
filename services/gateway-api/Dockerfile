# syntax=docker/dockerfile:1
# Build context must be services/ so shared pkg module is accessible.

FROM golang:1.27-alpine AS builder
WORKDIR /src

RUN apk add --no-cache ca-certificates

COPY go.work ./
COPY pkg/go.mod pkg/go.sum ./pkg/
COPY gateway-api/go.mod gateway-api/go.sum ./gateway-api/

RUN cd gateway-api && go mod download

COPY pkg/ ./pkg/
COPY gateway-api/ ./gateway-api/

RUN CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags="-s -w" -o /out/gateway-api ./gateway-api/cmd

FROM gcr.io/distroless/static-debian12:nonroot
WORKDIR /app

COPY --from=builder /out/gateway-api /app/gateway-api

EXPOSE 8080
USER nonroot:nonroot
ENTRYPOINT ["/app/gateway-api"]
