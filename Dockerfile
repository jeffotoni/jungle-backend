FROM golang:1.25-bookworm AS builder

WORKDIR /src

COPY go.mod go.sum ./
RUN go mod download

COPY . .

ARG SERVICE
RUN test -n "$SERVICE" && \
    CGO_ENABLED=0 GOOS=linux go build \
    -trimpath \
    -ldflags="-s -w" \
    -o /out/service \
    ./cmd/$SERVICE


FROM alpine:3.22

RUN apk add --no-cache ca-certificates

ENV TZ=UTC

WORKDIR /app

COPY --from=builder /out/service /app/service

ENTRYPOINT ["/app/service"]