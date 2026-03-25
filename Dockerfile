# Если в логе: "connecting to 127.0.0.1:... refused" — Docker использует мёртвый HTTP(S)_PROXY.
# Уберите прокси в среде / Docker Desktop → Proxies, либо задайте NO_PROXY=registry-1.docker.io
#
# При ошибке TLS / timeout до registry-1.docker.io задайте зеркало, например:
#   docker compose build --build-arg GOLANG_IMAGE=docker.m.daocloud.io/library/golang:1.22-alpine --build-arg ALPINE_IMAGE=docker.m.daocloud.io/library/alpine:3.20
# Или настройте mirror в Docker Desktop / daemon.json (registry-mirrors).
# ARG для всех FROM должны быть до первого FROM (иначе второй FROM не видит ALPINE_IMAGE).
ARG GOLANG_IMAGE=golang:1.22-alpine
ARG ALPINE_IMAGE=alpine:3.20
FROM ${GOLANG_IMAGE} AS builder
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

FROM ${ALPINE_IMAGE} AS runtime
RUN apk add --no-cache docker-cli && adduser -D appuser
WORKDIR /app
COPY --from=builder /out/gateway /usr/local/bin/gateway
COPY --from=builder /out/room /usr/local/bin/room
COPY --from=builder /out/room-manager /usr/local/bin/room-manager
COPY ["map/corridor5.json", "/assets/maps/corridor5.json"]
# Раскомментируйте после копирования shareware DOOM.WAD в map/DOOM.WAD:
# COPY ["map/DOOM.WAD", "/assets/DOOM.WAD"]
USER appuser

# Runtime command is selected in docker-compose per service.
CMD ["/usr/local/bin/gateway"]
