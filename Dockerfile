# 两阶段构建：纯 Go、CGO 关闭，产物是单个静态二进制；运行时镜像只带二进制与许可文件。
FROM --platform=$BUILDPLATFORM golang:1.26.3-alpine AS builder

ARG TARGETARCH
# 构建信息：release-image.yml 传入 tag 与提交短哈希，本地产物默认 dev
ARG VERSION=dev
ARG COMMIT=

WORKDIR /src

COPY go.mod go.sum ./
RUN go mod download

COPY cmd ./cmd
COPY internal ./internal

RUN CGO_ENABLED=0 GOOS=linux GOARCH=$TARGETARCH go build -trimpath \
    -ldflags="-s -w -X main.version=${VERSION} -X main.commit=${COMMIT}" \
    -o /out/devin-gateway ./cmd/devin-gateway

FROM alpine:3.22

RUN apk add --no-cache ca-certificates \
    && adduser -D -H -u 10001 app \
    && mkdir -p /app/logs \
    && chown -R app:app /app

WORKDIR /app
COPY --from=builder /out/devin-gateway /app/devin-gateway
# MIT 许可与上游署名随镜像分发
COPY LICENSE NOTICE THIRD_PARTY_LICENSES /app/

# 以非 root 运行；debug.enabled=true 时日志写入 /app/logs。
USER 10001

EXPOSE 18089

HEALTHCHECK --interval=30s --timeout=3s --start-period=5s \
  CMD wget -qO- http://127.0.0.1:18089/healthz >/dev/null || exit 1

ENTRYPOINT ["/app/devin-gateway"]
