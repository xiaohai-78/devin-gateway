SHELL := /bin/bash

# 端口优先取命令行 PORT=，其次 .env 里的 PORT，最后 18089
ENV_PORT := $(shell sed -n 's/^PORT=//p' .env 2>/dev/null | tail -1 | tr -d '\r')
PORT ?= $(if $(ENV_PORT),$(ENV_PORT),18089)
BASE_URL ?= http://127.0.0.1:$(PORT)
GO_IMAGE := $(shell sed -n 's/^FROM --platform=[^ ]* \(golang:[^ ]*\) .*/\1/p' Dockerfile | head -1)
# 脚本清单在宿主机展开，再作为参数传给 shellcheck 容器
SHELL_SCRIPTS := $(addprefix /mnt/,$(notdir $(wildcard scripts/*.sh)))

# 默认拉预构建镜像；dev 叠加层改为本地构建
COMPOSE ?= docker compose -f docker-compose.yml
COMPOSE_DEV := $(COMPOSE) -f docker-compose.dev.yml
# 本地构建的版本标识：tag 或短哈希（含 -dirty）
GIT_VERSION := $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
GIT_COMMIT := $(shell git rev-parse --short HEAD 2>/dev/null || echo '')

.PHONY: help token up up-image down logs ps smoke check lint test ci clean

help:
	@echo "make token     macOS: 从 Devin 桌面版读 token 并写入 .env"
	@echo "make up        本地构建并启动（开发用，不需要先发布镜像）"
	@echo "make up-image  拉 GHCR 预构建镜像并启动（用户路径）"
	@echo "make down      停止"
	@echo "make logs      跟踪日志"
	@echo "make smoke     六项冒烟测试（需要真实 token）"
	@echo "make check     校验 token 与上游连通性（容器内跑 -check）"
	@echo "make lint      shellcheck 检查脚本"
	@echo "make test      Go 单元测试（容器内运行，不需要本机装 Go）"
	@echo "make clean     停容器并删除本地镜像"

token:
	@bash scripts/token-macos.sh

up:
	@test -f .env || { echo "缺少 .env：先运行 make token，或 cp .env.example .env 后填 DEVIN_TOKEN"; exit 1; }
	GATEWAY_VERSION="$(GIT_VERSION)" GATEWAY_COMMIT="$(GIT_COMMIT)" $(COMPOSE_DEV) up -d --build
	@echo
	@echo "网关地址：$(BASE_URL)/v1"
	@echo "自检：  make smoke"

up-image:
	@test -f .env || { echo "缺少 .env：先运行 make token，或 cp .env.example .env 后填 DEVIN_TOKEN"; exit 1; }
	$(COMPOSE) pull
	$(COMPOSE) up -d
	@echo "网关地址：$(BASE_URL)/v1"

down:
	$(COMPOSE_DEV) down

logs:
	$(COMPOSE_DEV) logs -f --tail=50

ps:
	$(COMPOSE_DEV) ps

smoke:
	@bash scripts/smoke.sh "$(BASE_URL)"

# token 失效时会打印上游的 401，比空模型列表好排查
check:
	$(COMPOSE_DEV) exec -T gateway /app/devin-gateway -check

lint:
	docker run --rm -v "$$PWD/scripts:/mnt:ro" koalaman/shellcheck:stable -x $(SHELL_SCRIPTS)

test:
	docker run --rm -v "$$PWD":/src -w /src -e GOFLAGS=-buildvcs=false $(GO_IMAGE) \
	  sh -c 'go vet ./... && go test ./...'

# CI 用：不启动容器，只验证可构建
ci:
	$(COMPOSE) config -q
	$(COMPOSE_DEV) config -q
	$(COMPOSE_DEV) build

clean:
	$(COMPOSE_DEV) down --rmi local -v 2>/dev/null || true
