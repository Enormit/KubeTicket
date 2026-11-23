# syntax=docker/dockerfile:1
# Build context must be services/ so shared pkg module is accessible.

FROM golang:1.27-alpine AS builder
WORKDIR /src
ENV GOWORK=off

RUN apk add --no-cache ca-certificates

COPY pkg/go.mod pkg/go.sum ./pkg/
COPY flashsale-engine/go.mod flashsale-engine/go.sum ./flashsale-engine/

RUN cd pkg && go mod download && cd ../flashsale-engine && go mod download

COPY pkg/ ./pkg/
COPY flashsale-engine/ ./flashsale-engine/

RUN cd flashsale-engine && CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags="-s -w" -o /out/flashsale-engine ./cmd

FROM gcr.io/distroless/static-debian12:nonroot
WORKDIR /app

COPY --from=builder /out/flashsale-engine /app/flashsale-engine

EXPOSE 8080
USER nonroot:nonroot
ENTRYPOINT ["/app/flashsale-engine"]
