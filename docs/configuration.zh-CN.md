# 配置参考

[返回 README](../README.zh-CN.md) · [English](configuration.md)

网关在启动时读取工作目录下的 `config.yaml`（或 `-config` 指定的文件），只读一次；文件不存在时使用内置默认值。
仓库里不提交 `config.yaml`，只有 `config.example.yaml`。未知 YAML 字段会被拒绝。容器 / systemd 场景可用环境变量覆盖
YAML；`devin.reasoning_tiers` 这类嵌套 map 只能写在 YAML 里。

## 最小配置

```yaml
devin:
  base_url: "https://server.codeium.com"
  token: "devin-session-token$..."
  model: "glm-5-2"
```

`server`、`catalog`、`debug`、`auth` 都是可选的。完整起点见
[`config.example.yaml`](../config.example.yaml)（镜像和二进制已内置同样的默认值；要用时复制为 `config.yaml`）。

## 字段

| 字段 | 默认值 | 说明 |
| --- | --- | --- |
| `server.listen` | `127.0.0.1:18089` | HTTP 监听地址。默认只走回环；容器镜像里设为 `SERVER_LISTEN=:18089`。放宽前先设 `auth.api_key` |
| `server.max_concurrency` | `1024` | `/v1/*` 并发上限；`0` 使用默认值 |
| `devin.base_url` | — | Devin Connect 地址，如 `https://server.codeium.com` |
| `devin.token` | — | 会话 token；未设置时 `/v1/*` 不可用 |
| `devin.model` | — | 启动必填。供 `-check` 使用，上游目录没列出时补进 `/v1/models`。请求仍须带 `model` |
| `devin.proxy` | — | 可选 HTTP/HTTPS 代理；留空直连或走系统代理 |
| `devin.force_http1` | `true` | 规避上游 HTTP/2 多路复用瓶颈 |
| `devin.stream_idle_timeout` | `5m` | 上游超时无帧则返回 504；`0` 表示不限制 |
| `devin.reasoning_tiers` | — | 把基础模型 + 档位映射为实际 uid（见下文） |
| `catalog.snapshot` | — | 模型目录快照路径，相对 `config.yaml`；留空不写快照 |
| `catalog.refresh` | `5m` | 目录缓存有效期；`0` 使用默认值 |
| `catalog.model_mode` | `raw` | `raw`、`openai` 或 `both`（见下文） |
| `catalog.auto_tiers` | `false` | 从 family metadata 自动生成基础模型/档位 |
| `debug.enabled` | `false` | 在 `logs/` 下记录每请求日志 |
| `auth.api_key` | — | 为空则不校验 |

## 环境变量覆盖

| 环境变量 | 字段 |
| --- | --- |
| `SERVER_LISTEN` | `server.listen` |
| `SERVER_MAX_CONCURRENCY` | `server.max_concurrency` |
| `DEVIN_BASE_URL` | `devin.base_url` |
| `DEVIN_TOKEN` | `devin.token` |
| `DEVIN_MODEL` | `devin.model` |
| `DEVIN_PROXY` | `devin.proxy` |
| `DEVIN_FORCE_HTTP1` | `devin.force_http1` |
| `DEVIN_STREAM_IDLE_TIMEOUT` | `devin.stream_idle_timeout` |
| `CATALOG_SNAPSHOT` | `catalog.snapshot` |
| `CATALOG_REFRESH` | `catalog.refresh` |
| `CATALOG_MODEL_MODE` | `catalog.model_mode` |
| `CATALOG_AUTO_TIERS` | `catalog.auto_tiers` |
| `DEBUG_ENABLED` | `debug.enabled` |
| `AUTH_API_KEY` | `auth.api_key` |

`devin.reasoning_tiers` 是嵌套 map，没有环境变量，只能写在 YAML 中。

## 推理档位映射

Devin 的部分模型把推理档位写在 uid 里（`swe-2-medium`、`swe-2-high`）。把它们映射到稳定的基础 id：

```yaml
devin:
  reasoning_tiers:
    swe-2:
      medium: swe-2-medium
      high: swe-2-high
      max: swe-2-max
```

```bash
curl -s localhost:18089/v1/chat/completions \
  -H 'Content-Type: application/json' \
  -d '{"model":"swe-2","reasoning_effort":"high","messages":[{"role":"user","content":"hi"}]}'
# 实际发给上游的 chat_model_uid 是 swe-2-high
```

- Responses 使用 `reasoning: {"effort": "high"}`；
- Chat 也接受 `thinking: true` 作为 `high` 的简写，`thinking: {"type": "enabled"}` 同样可用；
- 合成的基础 id（本身不是上游模型）不带档位时返回 `400`，列出支持的档位，并给出一个可直接调用的档位 id；
  本身就是上游真实模型的基础 id 不带档位时原样透传；
- 显式档位 id（如 `swe-2-high`）原样透传；
- 网关不会自行拼后缀，只使用显式配置或自动生成的映射。

`catalog.auto_tiers: true` 时，建议来自上游 family metadata；显式的
`devin.reasoning_tiers` 优先级更高。

## 模型目录

### 快照

```yaml
catalog:
  snapshot: "state/models.json"
  refresh: "5m"
```

文件原子写入（`0600`），不是配置文件的替代品。上游失败时使用上次快照，并标记 `stale`。
`GET /v1/model-manifest` 返回 `schema_version`、`generated_at`、`catalog_hash`、`stale`、
原始模型列表和自动生成的 `families` 建议。

手动刷新：

```bash
go run ./cmd/devin-gateway -config config.yaml -sync-models
```

### 展示模式

| 模式 | `/v1/models` |
| --- | --- |
| `raw` | 上游 id，如 `swe-2-high`（默认） |
| `openai` | 虚拟基础模型，如 `swe-2`，带 `reasoning_efforts` 和 `requires_reasoning_effort`；实际档位 id 隐藏但仍可调用 |
| `both` | 基础模型和实际 id 都显示 |

`openai` 模式下，没有 family metadata 的模型和 fast/lightning 变体保持可见。
如果某个 family 的基础 id 本身就是上游真实模型（如 `swe-1-7`），不传 `reasoning_effort` 也能直接调用：
它会列出可选的 `reasoning_efforts`，但不标 `requires_reasoning_effort`，对应档位 id 也保持可见。
被隐藏的档位 id 仍可用于请求，但 `/v1/models/{id}` 会返回 404。

## 调试与鉴权

- `debug.enabled` 会把请求、中间请求、上游帧和响应记录到 `logs/`；
- 即使关闭 debug，失败也会带请求 ID 输出到 stderr；
- `auth.api_key` 启用 `Authorization: Bearer <key>` 和 `X-Api-Key: <key>`；为空则不校验；`/healthz` 始终不鉴权；
- session token 不会写入调试日志。

## 校验

```bash
go run ./cmd/devin-gateway -config config.yaml -check
```

`-check` 校验配置、token 和上游目录；如果只有 stale 快照会返回非零。
`-sync-models` 要求配置 `catalog.snapshot`，上游无法刷新时返回非零。
