#!/usr/bin/env bash
# 从 Devin 桌面版读取会话 token，并写入仓库根目录的 .env。
#
#   make token
#   DEVIN_TOKEN='devin-session-token$...' make token   # 手动指定
#
# macOS 上优先用本机 sqlite3；没有 sqlite3 就退回一次性 alpine 容器。
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
ENV_FILE="$ROOT/.env"
DB="$HOME/Library/Application Support/Devin/User/globalStorage/state.vscdb"
QUERY="SELECT json_extract(value,'\$.apiKey') FROM ItemTable WHERE key='windsurfAuthStatus';"

TOKEN="${DEVIN_TOKEN:-}"

if [ -z "$TOKEN" ]; then
  if [ ! -f "$DB" ]; then
    echo "找不到 Devin 本地状态库：$DB" >&2
    echo "若这台机器没装 Devin 桌面版，请在有桌面版的机器上取一次：" >&2
    echo "  DEVIN_TOKEN='devin-session-token\$...' make token" >&2
    exit 1
  fi

  if command -v sqlite3 >/dev/null 2>&1; then
    TOKEN="$(sqlite3 "$DB" "$QUERY" 2>/dev/null || true)"
  elif command -v docker >/dev/null 2>&1; then
    SQL_FILE="$(mktemp)"
    printf '%s\n' "$QUERY" > "$SQL_FILE"
    TOKEN="$(docker run --rm \
      -v "$DB:/db/state.vscdb:ro" \
      -v "$SQL_FILE:/db/q.sql:ro" \
      --entrypoint sh alpine:3.22 \
      -c 'apk add --no-cache sqlite >/dev/null 2>&1; sqlite3 /db/state.vscdb < /db/q.sql' 2>/dev/null | tr -d '\r\n' || true)"
    rm -f "$SQL_FILE"
  fi
fi

if [ -z "$TOKEN" ]; then
  echo "没有读到 token（Devin 可能未登录）。用 DEVIN_TOKEN=... make token 手动传入。" >&2
  exit 1
fi

case "$TOKEN" in
  devin-session-token*) ;;
  *) echo "警告：token 不以 devin-session-token 开头，可能取错了" >&2 ;;
esac

# .env 里用单引号包住，防止 Compose 展开 $ 开头的片段
TMP="$(mktemp)"
if [ -f "$ENV_FILE" ]; then
  grep -v '^DEVIN_TOKEN=' "$ENV_FILE" > "$TMP" || true
fi
[ -s "$TMP" ] || {
  echo "PORT=18089" >> "$TMP"
  echo "AUTH_API_KEY=" >> "$TMP"
}
printf "DEVIN_TOKEN='%s'\n" "$TOKEN" >> "$TMP"
mv "$TMP" "$ENV_FILE"
chmod 600 "$ENV_FILE"

echo "==> 已写入 $ENV_FILE"
echo "    token: ${TOKEN:0:22}…（长度 ${#TOKEN}）"
echo "    下一步: make up"
