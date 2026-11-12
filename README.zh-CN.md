# devin-gateway

把 **Devin 订阅里的模型** 变成 **OpenAI 兼容 API**，给任何客户端用（Cline、Cursor、Cherry Studio、Codex CLI、脚本）。

自包含：Go 源码在仓库里，镜像与多平台二进制由 CI 发布。

[English](README.md) | **中文**

## 快速开始

### Docker（推荐，不用编译）

```bash
git clone https://github.com/xiaohai-78/devin-gateway.git && cd devin-gateway
cp .env.example .env      # 填 DEVIN_TOKEN
docker compose up -d      # 自动拉预构建镜像
curl 127.0.0.1:18089/healthz
```

token 从哪来：macOS 跑 `bash scripts/token-macos.sh`（会自动写 `.env`）；
Windows 跑 `powershell -ExecutionPolicy Bypass -File .\scripts\token-windows.ps1`；
其他系统从装了 Devin 桌面版的机器上复制 `devin-session-token$...` 值。

> 镜像发布之前 `docker compose up -d` 没有可拉取的镜像，改用 `make up`（本地构建）。

### 本机开发

```bash
make token    # macOS 自动读 Devin 桌面版 token
make up       # 本地构建并启动
make smoke    # 六项自检
```

### 不用 Docker

从 [Releases](https://github.com/xiaohai-78/devin-gateway/releases/latest) 下载对应平台的压缩包：

```bash
tar -xzf devin-gateway_*_darwin_arm64.tar.gz   # 换成你的平台
chmod +x devin-gateway
DEVIN_TOKEN='devin-session-token$...' ./devin-gateway
# Windows：解压后 $env:DEVIN_TOKEN='...'; .\devin-gateway.exe
```

也可以自己编译：`go build -o devin-gateway ./cmd/devin-gateway`。

二进制同样**内置默认配置**，只需要 `DEVIN_TOKEN`。

## 客户端配置

| 项 | 值 |
|---|---|
| Base URL | `http://127.0.0.1:18089/v1` |
| API Key | 任意非空（设了 `AUTH_API_KEY` 则填它） |
| Model | `GET /v1/models` 里的任意 id |

模型选择：部分模型**要求 `reasoning_effort`**，图形客户端通常不会发。用
`curl 127.0.0.1:18089/v1/models` 查 `requires_reasoning_effort`，选 `false` 的即可；
SDK/curl 能自己发这个字段时就没有限制。

```python
from openai import OpenAI
c = OpenAI(base_url="http://127.0.0.1:18089/v1", api_key="local")
print(c.chat.completions.create(
    model="claude-sonnet-4-6-1m",
    messages=[{"role": "user", "content": "hi"}],
).choices[0].message.content)
```

## 端点

| 端点 | 说明 |
|---|---|
| `POST /v1/chat/completions` | 流式/非流式，支持 `tools` |
| `POST /v1/responses` | OpenAI Responses 协议（Codex CLI） |
| `GET /v1/models` | 目录：上下文、图片能力、推理档位 |
| `GET /v1/quota` | 套餐与日/周剩余额度 |
| `GET /healthz` | 不鉴权、不占并发 |

## 配置

环境变量优先于配置文件：

| 变量 | 说明 |
|---|---|
| `DEVIN_TOKEN` | **必填**，Devin 会话 token |
| `DEVIN_MODEL` | 默认模型 uid（内置默认 `glm-5-2`） |
| `AUTH_API_KEY` | 给 `/v1/*` 加鉴权；留空则免鉴权 |
| `PORT` | 宿主机端口，默认 18089 |

需要改其他项时挂载配置文件：`-v "$PWD/config.yaml:/app/config.yaml:ro"`。
完整字段见 [docs/configuration.zh-CN.md](docs/configuration.zh-CN.md)。

## 排障

| 现象 | 处理 |
|---|---|
| 模型列表空 / 401 | token 无效或过期：重新 `make token`，或检查 `.env` 里是否用**单引号**包住 token |
| 日志出现 `config not found` | 正常：说明在用内置默认配置（可用环境变量覆盖） |
| 端口被占 | `lsof -iTCP:18089`，或改 `.env` 里的 `PORT` |
| `requires reasoning_effort` | 见上面「客户端配置」 |
| 其他 | `make logs`，或见 [docs/protocol.md](docs/protocol.md) |

## 仓库

```
cmd/ internal/         Go 源码（网关本体 + 内置默认配置）
docker-compose.yml     拉预构建镜像
docker-compose.dev.yml 本地构建叠加层（make up）
config.yaml            可选覆盖配置（默认不挂载）
Dockerfile             两阶段、非 root、内置默认配置
Makefile               token / up / up-image / smoke / test
scripts/               token 读取、冒烟测试
docs/                  协议与配置参考
```

## 许可与免责

本仓库 MIT（[LICENSE](LICENSE)）。上游署名见 [NOTICE](NOTICE)：
[devin-2api-lite](https://github.com/Green-hats/devin-2api-lite) © Green-hats、
[devin-2api](https://github.com/leookun/devin-2api) © leookun。

**非官方**：Devin 协议私有且无文档，本项目与 Devin / Windsurf / Codeium / Cognition 无关联。
使用自己的账号 token，风险自负，并遵守上游条款；使用可能导致账号受限。
