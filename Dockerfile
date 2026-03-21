FROM golang:1.22-alpine AS builder
WORKDIR /src

# sum.golang.org часто даёт TLS-ошибки за прокси/антивирусом/Docker Desktop (lookup при verify).
# Зеркало модулей можно сменить: docker compose build --build-arg GOPROXY=https://goproxy.io,direct
ARG GOPROXY=https://proxy.golang.org,direct
ENV GOPROXY=${GOPROXY}
ENV GOSUMDB=off

COPY . .
RUN go mod tidy && \
    CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o /out/gateway ./cmd/gateway && \
    CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o /out/room ./cmd/room && \
    CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o /out/room-manager ./cmd/room-manager

FROM alpine:3.20 AS runtime
RUN apk add --no-cache docker-cli && adduser -D appuser
WORKDIR /app
COPY --from=builder /out/gateway /usr/local/bin/gateway
COPY --from=builder /out/room /usr/local/bin/room
COPY --from=builder /out/room-manager /usr/local/bin/room-manager
COPY ["doom wed/DOOM.WAD", "/assets/DOOM.WAD"]
COPY ["doom wed/map1.json", "/assets/maps/map1.json"]
COPY ["doom wed/map2.json", "/assets/maps/map2.json"]
USER appuser

# Runtime command is selected in docker-compose per service.
CMD ["/usr/local/bin/gateway"]
