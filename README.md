# devin-gateway

Turn the models in your **Devin subscription** into an **OpenAI-compatible API** for any client
(Cline, Cursor, Cherry Studio, Codex CLI, scripts).

Self-contained: the Go source lives in this repo; images and multi-platform binaries are published by CI.

**English** | [中文](README.zh-CN.md)

## Quick start

### Docker (recommended, no build)

```bash
git clone https://github.com/xiaohai-78/devin-gateway.git && cd devin-gateway
cp .env.example .env      # fill in DEVIN_TOKEN
docker compose up -d      # pulls the prebuilt image
curl 127.0.0.1:18089/healthz
```

Where does the token come from? On macOS run `bash scripts/token-macos.sh` (it writes `.env` for you);
on Windows run `powershell -ExecutionPolicy Bypass -File .\scripts\token-windows.ps1`.
Anywhere else, copy a `devin-session-token$...` value from a machine that has the Devin app.


### Local development

```bash
make token    # macOS: read the token from the Devin desktop app
make up       # build locally and start
make smoke    # six checks
```

### No Docker

Download the binary for your platform from
[Releases](https://github.com/xiaohai-78/devin-gateway/releases/latest), then:

```bash
# macOS/Linux: .tar.gz archive; Windows: .zip archive
tar -xzf devin-gateway_0.1.2_darwin_arm64.tar.gz   # use your platform's file
chmod +x devin-gateway
DEVIN_TOKEN='devin-session-token$...' ./devin-gateway
# Windows (PowerShell): expand-archive the .zip, then
# $env:DEVIN_TOKEN='...'; .\devin-gateway.exe
```

Or build from source: `go build -o devin-gateway ./cmd/devin-gateway`.

The binary ships a **built-in default config** — only `DEVIN_TOKEN` is required.

## Client setup

| Field | Value |
|---|---|
| Base URL | `http://127.0.0.1:18089/v1` |
| API Key | any non-empty value (or `AUTH_API_KEY`) |
| Model | any id from `GET /v1/models` |

Some models **require `reasoning_effort`**, which GUI clients usually don't send. Check
`requires_reasoning_effort` in `curl 127.0.0.1:18089/v1/models` and pick the `false` ones,
or use an SDK that can send the field.

```python
from openai import OpenAI
c = OpenAI(base_url="http://127.0.0.1:18089/v1", api_key="local")
print(c.chat.completions.create(
    model="claude-sonnet-4-6-1m",
    messages=[{"role": "user", "content": "hi"}],
).choices[0].message.content)
```

## Endpoints

| Endpoint | Notes |
|---|---|
| `POST /v1/chat/completions` | streaming/non-streaming, `tools` |
| `POST /v1/responses` | OpenAI Responses protocol (Codex CLI) |
| `GET /v1/models` | catalog with context, vision, reasoning tiers |
| `GET /v1/quota` | plan and remaining daily/weekly quota |
| `GET /healthz` | unauthenticated, no concurrency slot |

## Configuration

Environment wins over the config file:

| Variable | Meaning |
|---|---|
| `DEVIN_TOKEN` | **required** Devin session token |
| `DEVIN_MODEL` | default model uid (built-in default `glm-5-2`) |
| `AUTH_API_KEY` | protect `/v1/*`; empty = no auth |
| `PORT` | host port, default 18089 |

To change anything else, mount a config file: `-v "$PWD/config.yaml:/app/config.yaml:ro"`.
Full reference: [docs/configuration.md](docs/configuration.md).

## Troubleshooting

| Symptom | Fix |
|---|---|
| empty model list / 401 | token invalid or expired: rerun `make token`; keep it **single-quoted** in `.env` |
| `config not found` log line | expected — built-in defaults in use, override with env vars |
| port in use | `lsof -iTCP:18089`, or change `PORT` in `.env` |
| `requires reasoning_effort` | see "Client setup" |
| anything else | `make logs`, or [docs/protocol.md](docs/protocol.md) |

## Layout

```
cmd/ internal/         Go source (gateway + built-in default config)
docker-compose.yml     pull the prebuilt image
docker-compose.dev.yml local build overlay (make up)
config.yaml            optional override (not mounted by default)
Dockerfile             two-stage, non-root, built-in defaults
Makefile               token / up / up-image / smoke / test
scripts/               token extraction, smoke test
docs/                  protocol and configuration reference
```

## License & disclaimer

MIT for this repository ([LICENSE](LICENSE)). Upstream credits in [NOTICE](NOTICE):
[devin-2api-lite](https://github.com/Green-hats/devin-2api-lite) © Green-hats,
[devin-2api](https://github.com/leookun/devin-2api) © leookun.

**Unofficial.** The Devin protocol is proprietary and undocumented; this project is not affiliated
with Devin, Windsurf, Codeium or Cognition. Use your own account token at your own risk and follow
the upstream terms — usage may get your account restricted.
