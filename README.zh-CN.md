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
两个脚本都不需要 Docker：macOS 用系统自带的 `sqlite3`；Windows 依次尝试 `sqlite3.exe`、
Python 3、Docker，都没有时会提示该装什么。
其他系统从装了 Devin 桌面版的机器上复制 `devin-session-token$...` 值。

### 启动 / 停止

```bash
docker compose up -d       # 启动（之后随 Docker 自动重启）
docker compose logs -f     # 跟踪日志
docker compose up -d       # 改了 .env 之后用它：按新值重建容器
docker compose restart     # 单纯重启，不会重新读取 .env
docker compose down        # 停止并删除容器
```

### 本机开发

```bash
make token    # macOS 自动读 Devin 桌面版 token
make up       # 本地构建并启动
make smoke    # 六项自检
```

### 不用 Docker

从 [Releases](https://github.com/xiaohai-78/devin-gateway/releases/latest) 下载对应平台的压缩包：

macOS / Linux（`.tar.gz`）：

```bash
tar -xzf devin-gateway_*_darwin_arm64.tar.gz     # 换成你的平台
cd devin-gateway_*_darwin_arm64                   # 压缩包会解压出一个同名目录
DEVIN_TOKEN='devin-session-token$...' ./devin-gateway   # 用单引号：token 里有 $
```

Windows（`.zip`）：辅助脚本在本仓库里，所以先 clone 仓库（或下载源码 zip），再把 release
压缩包里的 `devin-gateway.exe` 放到**仓库根目录**：

```powershell
.\scripts\token-windows.ps1     # 只需一次：从 Devin 桌面版读 token 写入 .env
.\scripts\start-windows.ps1     # 只给本进程加载 .env，然后启动 exe
```

也可以自己编译：`go build -o devin-gateway ./cmd/devin-gateway`。

二进制同样**内置默认配置**，只需要 `DEVIN_TOKEN`。它在前台运行，按 `Ctrl+C` 停止。
默认**只监听 `127.0.0.1:18089`**；改监听地址用 `SERVER_LISTEN`（`PORT` 只对 Docker Compose
生效，对二进制无效）。

在 PowerShell 里手动设置 token 时请用**单引号**：双引号会展开 `$...`，而 token 里一定有 `$`，
会被悄悄改坏。

**Windows 常驻运行。** 优先用 Docker（会自动重启），或用 [NSSM](https://nssm.cc/) 把
`scripts\start-windows.ps1` 注册成服务。token 留在 `.env` 里（只有你能读），不要用 `setx`
写进用户或系统的永久环境变量：之后你运行的每个程序都能读到它。

如果用任务计划程序：任务默认带 **72 小时执行时限**，到点会被系统静默杀掉。
请在仓库根目录用 PowerShell 创建任务并关掉这个时限：

```powershell
$script   = Join-Path $PWD 'scripts\start-windows.ps1'
$action   = New-ScheduledTaskAction -Execute 'powershell.exe' `
              -Argument "-NoProfile -ExecutionPolicy Bypass -File `"$script`"" -WorkingDirectory $PWD
$trigger  = New-ScheduledTaskTrigger -AtLogOn
$settings = New-ScheduledTaskSettingsSet -ExecutionTimeLimit ([TimeSpan]::Zero) -RestartCount 3 `
              -RestartInterval (New-TimeSpan -Minutes 1)
Register-ScheduledTask devin-gateway -Action $action -Trigger $trigger -Settings $settings
```

`schtasks /create` 改不了这个时限，不要用它跑常驻网关。

## 客户端配置

| 项 | 值 |
|---|---|
| Base URL | `http://127.0.0.1:18089/v1` |
| API Key | 任意非空（设了 `AUTH_API_KEY` 则填它） |
| Model | `GET /v1/models` 里的任意 id |

**每个请求都必须带 `model`。** 可用 id 从 `curl 127.0.0.1:18089/v1/models` 查：

- 标了 `"requires_reasoning_effort": true` 的（如 `claude-sonnet-5-5`）是合成的基础 id：
  要么传 `reasoning_effort`（取 `reasoning_efforts` 里的值），要么直接用具体档位 id。
  报错信息里会给出一个可以照抄的 id。
- 其他 id 都能直接调用。如果它也列了 `reasoning_efforts`（如 `swe-1-7`），传 `reasoning_effort`
  会路由到对应档位；不传就用该模型自身的默认行为。
- 目录由上游下发，可能包含你的套餐用不了的模型，调用会报 `403 permission_denied`，换一个即可。

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
| `AUTH_API_KEY` | 给 `/v1/*` 加鉴权；留空则免鉴权。**监听范围超出回环前必须设置** |
| `SERVER_LISTEN` | 监听地址。二进制默认 `127.0.0.1:18089`；镜像里设为 `:18089` |
| `PORT` | **仅 Docker Compose**：绑定在 `127.0.0.1` 上的宿主机端口，默认 18089 |
| `DEVIN_MODEL` | `-check` 报告的模型；上游目录没列出时会补进 `/v1/models`（默认 `glm-5-2`）。**不是**兜底模型，请求仍须带 `model` |

需要改其他项：`cp config.example.yaml config.yaml` 后修改，二进制在同目录运行即可（或用 `-config` 指定），
Docker 则挂载：`-v "$PWD/config.yaml:/app/config.yaml:ro"`。
完整字段见 [docs/configuration.zh-CN.md](docs/configuration.zh-CN.md)。

### 开放给局域网

所有方式默认都只监听本机。要让其他机器访问，**先设置 `AUTH_API_KEY`**（一串足够长的随机字符串，
客户端把它填成 API Key），然后：

- Docker：把 `docker-compose.yml` 的端口行改成 `"18089:18089"`，再 `docker compose up -d`。
- 二进制：设置 `SERVER_LISTEN=0.0.0.0:18089`。

更稳妥的做法是保持只监听本机，通过 Tailscale 或 SSH 隧道访问。

## 排障

| 现象 | 处理 |
|---|---|
| 模型列表空 / 401 | token 无效或过期：重新 `make token`，或检查 `.env` 里是否用**单引号**包住 token |
| 日志出现 `config not found` | 正常：说明在用内置默认配置（可用环境变量覆盖） |
| 端口被占 | `lsof -iTCP:18089`；Docker 改 `.env` 里的 `PORT`，二进制设 `SERVER_LISTEN` |
| `requires reasoning_effort` | 传 `reasoning_effort`，或改用报错里给出的档位 id |
| `permission_denied` / 对你的账号不可用 | 套餐不含这个模型，换一个 |
| 局域网连不上（二进制） | 默认只听回环：`SERVER_LISTEN=0.0.0.0:18089`，并设置 `AUTH_API_KEY` |
| Windows 上跑了约 3 天自己停了 | 任务计划程序 72 小时时限，见「Windows 常驻运行」 |
| 其他 | `make logs`，或见 [docs/protocol.md](docs/protocol.md) |

## 仓库

```
cmd/ internal/         Go 源码（网关本体 + 内置默认配置）
docker-compose.yml     拉预构建镜像
docker-compose.dev.yml 本地构建叠加层（make up）
config.example.yaml    带注释的示例；复制为 config.yaml（已 gitignore）后使用
Dockerfile             两阶段、非 root、内置默认配置
Makefile               token / up / up-image / smoke / test
scripts/               token 读取、Windows 启动脚本、冒烟测试
docs/                  协议与配置参考
```

## 许可与免责

本仓库 MIT（[LICENSE](LICENSE)）。上游署名见 [NOTICE](NOTICE)：
[devin-2api-lite](https://github.com/Green-hats/devin-2api-lite) © Green-hats、
[devin-2api](https://github.com/leookun/devin-2api) © leookun。

**非官方**：Devin 协议私有且无文档，本项目与 Devin / Windsurf / Codeium / Cognition 无关联。
使用自己的账号 token，风险自负，并遵守上游条款；使用可能导致账号受限。
