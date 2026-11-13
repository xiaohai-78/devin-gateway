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
Neither needs Docker: the macOS script uses the system `sqlite3`; the Windows script tries
`sqlite3.exe`, then Python 3, then Docker, and tells you what to install if none is present.
Anywhere else, copy a `devin-session-token$...` value from a machine that has the Devin app.

### Start / stop

```bash
docker compose up -d       # start (restarts automatically with Docker)
docker compose logs -f     # follow logs
docker compose up -d       # after editing .env: recreates the container with the new values
docker compose restart     # plain restart; does NOT reload .env
docker compose down        # stop and remove the container
```

### Local development

```bash
make token    # macOS: read the token from the Devin desktop app
make up       # build locally and start
make smoke    # six checks
```

### No Docker

Download the binary for your platform from
[Releases](https://github.com/xiaohai-78/devin-gateway/releases/latest), then:

macOS / Linux (`.tar.gz`):

```bash
tar -xzf devin-gateway_*_darwin_arm64.tar.gz     # use your platform's file
cd devin-gateway_*_darwin_arm64                   # the archive unpacks into a folder
DEVIN_TOKEN='devin-session-token$...' ./devin-gateway   # single quotes: the token contains $
```

Windows (`.zip`) — the helper scripts live in this repo, so clone it (or download the source zip)
first, then put `devin-gateway.exe` from the release zip in the **repo root**:

```powershell
.\scripts\token-windows.ps1     # once: reads the token from the Devin app into .env
.\scripts\start-windows.ps1     # loads .env for this process only and starts the exe
```

Or build from source: `go build -o devin-gateway ./cmd/devin-gateway`.

The binary ships a **built-in default config** — only `DEVIN_TOKEN` is required. It runs in the
foreground; stop it with `Ctrl+C`. By default it listens on **`127.0.0.1:18089` only**; change
the address with `SERVER_LISTEN` (`PORT` is a Docker Compose setting and has no effect here).

If you set the token by hand in PowerShell, use **single quotes**: in double quotes PowerShell
expands `$...`, and since every token contains `$` it would silently be corrupted.

**Keeping it running on Windows.** Prefer the Docker path (restarts on its own), or register
`scripts\start-windows.ps1` as a service with [NSSM](https://nssm.cc/). The token stays in `.env`
(readable only by you); don't copy it into persistent user/machine environment variables with
`setx` — every program you run could read it.

If you use Task Scheduler, tasks get a default **72-hour execution time limit** and are killed
when it expires. Create the task with PowerShell and disable the limit (run from the repo root):

```powershell
$script   = Join-Path $PWD 'scripts\start-windows.ps1'
$action   = New-ScheduledTaskAction -Execute 'powershell.exe' `
              -Argument "-NoProfile -ExecutionPolicy Bypass -File `"$script`"" -WorkingDirectory $PWD
$trigger  = New-ScheduledTaskTrigger -AtLogOn
$settings = New-ScheduledTaskSettingsSet -ExecutionTimeLimit ([TimeSpan]::Zero) -RestartCount 3 `
              -RestartInterval (New-TimeSpan -Minutes 1)
Register-ScheduledTask devin-gateway -Action $action -Trigger $trigger -Settings $settings
```

`schtasks /create` cannot change this limit; don't use it for a long-running gateway.

## Client setup

| Field | Value |
|---|---|
| Base URL | `http://127.0.0.1:18089/v1` |
| API Key | any non-empty value (or `AUTH_API_KEY`) |
| Model | any id from `GET /v1/models` |

**`model` is required in every request.** Model ids come from `curl 127.0.0.1:18089/v1/models`:

- Entries with `"requires_reasoning_effort": true` (e.g. `claude-sonnet-5-5`) are synthetic base
  ids: send `reasoning_effort` (one of `reasoning_efforts`), or pick a concrete tier id instead.
  The error message names one you can copy.
- Every other id can be called as-is. If it also lists `reasoning_efforts` (e.g. `swe-1-7`),
  sending `reasoning_effort` routes to that tier; omitting it uses the model's own default.
- The catalog comes from upstream and can include models your plan can't use; those fail with
  `403 permission_denied` — pick another id.

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
| `AUTH_API_KEY` | protect `/v1/*`; empty = no auth. **Set it before listening beyond loopback** |
| `SERVER_LISTEN` | listen address. Binary default `127.0.0.1:18089`; the image sets `:18089` |
| `PORT` | **Docker Compose only**: host port bound on `127.0.0.1`, default 18089 |
| `DEVIN_MODEL` | model reported by `-check`, and added to `/v1/models` if upstream doesn't list it (default `glm-5-2`). **Not** a fallback: requests must still send `model` |

To change anything else: `cp config.example.yaml config.yaml`, edit it, then run the binary next to it
(or pass `-config`), or mount it into the container: `-v "$PWD/config.yaml:/app/config.yaml:ro"`.
Full reference: [docs/configuration.md](docs/configuration.md).

### Exposing it to your LAN

Loopback-only is the default everywhere. To let other machines in, **set `AUTH_API_KEY` first**
(a long random string; clients send it as the API key), then:

- Docker: in `docker-compose.yml` change the port line to `"18089:18089"`, then `docker compose up -d`.
- Binary: `SERVER_LISTEN=0.0.0.0:18089`.

A safer alternative is to keep loopback and reach it over Tailscale or an SSH tunnel.

## Troubleshooting

| Symptom | Fix |
|---|---|
| empty model list / 401 | token invalid or expired: rerun `make token`; keep it **single-quoted** in `.env` |
| `config not found` log line | expected — built-in defaults in use, override with env vars |
| port in use | `lsof -iTCP:18089`; Docker: change `PORT` in `.env`; binary: set `SERVER_LISTEN` |
| `requires reasoning_effort` | send `reasoning_effort`, or use the tier id from the error message |
| `permission_denied` / not available for your account | your plan can't use that model; pick another |
| LAN clients can't connect (binary) | it listens on loopback by default: `SERVER_LISTEN=0.0.0.0:18089` + `AUTH_API_KEY` |
| Windows service dies after ~3 days | Task Scheduler 72 h limit; see "Keeping it running on Windows" |
| anything else | `make logs`, or [docs/protocol.md](docs/protocol.md) |

## Layout

```
cmd/ internal/         Go source (gateway + built-in default config)
docker-compose.yml     pull the prebuilt image
docker-compose.dev.yml local build overlay (make up)
config.example.yaml    commented example; copy to config.yaml (gitignored) to use
Dockerfile             two-stage, non-root, built-in defaults
Makefile               token / up / up-image / smoke / test
scripts/               token extraction, Windows launcher, smoke test
docs/                  protocol and configuration reference
```

## License & disclaimer

MIT for this repository ([LICENSE](LICENSE)). Upstream credits in [NOTICE](NOTICE):
[devin-2api-lite](https://github.com/Green-hats/devin-2api-lite) © Green-hats,
[devin-2api](https://github.com/leookun/devin-2api) © leookun.

**Unofficial.** The Devin protocol is proprietary and undocumented; this project is not affiliated
with Devin, Windsurf, Codeium or Cognition. Use your own account token at your own risk and follow
the upstream terms — usage may get your account restricted.
