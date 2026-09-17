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


FROM scratch

ENV TZ=America/Sao_Paulo

COPY --from=builder /etc/ssl/certs/ca-certificates.crt \
    /etc/ssl/certs/ca-certificates.crt

COPY --from=builder /usr/share/zoneinfo \
    /usr/share/zoneinfo

COPY --from=builder /out/service /app/service

ENTRYPOINT ["/app/service"]