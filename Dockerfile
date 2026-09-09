FROM golang:1.22-alpine AS builder

WORKDIR /src
COPY . .
RUN CGO_ENABLED=0 go build -ldflags="-s -w" -o /bin/aimilivpn ./cmd/aimilivpn

FROM alpine:3.20

RUN apk add --no-cache openvpn ca-certificates tzdata iptables

WORKDIR /app
COPY --from=builder /bin/aimilivpn /app/aimilivpn

ENV DATA_DIR=/app/data \
    UI_HOST=:: \
    UI_PORT=8787 \
    LOCAL_PROXY_HOST=:: \
    LOCAL_PROXY_PORT=7928

EXPOSE 8787 7928

ENTRYPOINT ["/app/aimilivpn"]
