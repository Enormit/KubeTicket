# syntax=docker/dockerfile:1
# Build context must be services/ so shared pkg module is accessible.

FROM golang:1.27-alpine AS builder
WORKDIR /src
ENV GOWORK=off

RUN apk add --no-cache ca-certificates

COPY pkg/go.mod pkg/go.sum ./pkg/
COPY order-worker/go.mod order-worker/go.sum ./order-worker/

RUN cd pkg && go mod download && cd ../order-worker && go mod download

COPY pkg/ ./pkg/
COPY order-worker/ ./order-worker/

RUN cd order-worker && CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags="-s -w" -o /out/order-worker ./cmd

FROM gcr.io/distroless/static-debian12:nonroot
WORKDIR /app

COPY --from=builder /out/order-worker /app/order-worker

EXPOSE 8080
USER nonroot:nonroot
ENTRYPOINT ["/app/order-worker"]
