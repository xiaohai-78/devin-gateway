#!/usr/bin/env bash
# 六项冒烟测试：健康、模型目录、额度、普通对话、SSE 流式、工具调用
#
#   scripts/smoke.sh                       # http://127.0.0.1:18089
#   scripts/smoke.sh http://10.0.0.5:18089
#   MODEL=<model-id> scripts/smoke.sh       # 跳过自动选模型
#
# 不需要 python3：有 jq 用 jq，没有就用 python3，都没有就退化为文本匹配。
set -uo pipefail

BASE="${1:-http://127.0.0.1:18089}"
KEY="${DEVIN2API_API_KEY:-local}"
MODEL="${MODEL:-}"

pass=0; fail=0
chk() {
  if [ "$1" = 0 ]; then printf '  \033[32mOK\033[0m   %s\n' "$2"; pass=$((pass+1))
  else printf '  \033[31mFAIL\033[0m %s\n' "$2"; fail=$((fail+1)); fi
}

have() { command -v "$1" >/dev/null 2>&1; }

# 列出不需要 reasoning_effort 的模型 id（每行一个）；没有 JSON 解析器时输出空。
list_models() {
  local body="$1"
  if have jq; then
    printf '%s' "$body" | jq -r '.data[] | select(.requires_reasoning_effort != true) | .id' 2>/dev/null
  elif have python3; then
    printf '%s' "$body" | python3 -c 'import json,sys
for m in json.load(sys.stdin).get("data", []):
    if not m.get("requires_reasoning_effort"):
        print(m["id"])' 2>/dev/null
  fi
}

# 选模型：优先已知便宜/稳定的候选，其次第一个可用的，避免挑到 -fast/-priority 变体。
pick_model() {
  local ids="$1" candidate
  for candidate in claude-sonnet-4-6-1m glm-5-2-1m swe-1-7 claude-opus-4-6-1m; do
    if printf '%s\n' "$ids" | grep -qx "$candidate"; then
      printf '%s' "$candidate"
      return
    fi
  done
  printf '%s\n' "$ids" | sed -n '1p'
}

# 统计模型数量；没有 JSON 解析器时退回粗略计数。
count_models() {
  local body="$1"
  if have jq; then
    printf '%s' "$body" | jq '.data | length' 2>/dev/null
  elif have python3; then
    printf '%s' "$body" | python3 -c 'import json,sys;print(len(json.load(sys.stdin).get("data", [])))' 2>/dev/null
  else
    printf '%s' "$body" | grep -o '"id"' | wc -l | tr -d ' '
  fi
}

echo "== target: $BASE"

curl -fsS -m 5 "$BASE/healthz" >/dev/null 2>&1
chk $? "healthz"

MODELS_BODY="$(curl -fsS -m 30 "$BASE/v1/models" 2>/dev/null)" || MODELS_BODY=""
if [ -n "$MODELS_BODY" ]; then
  N="$(count_models "$MODELS_BODY")"
  if [ "${N:-0}" -gt 0 ]; then chk 0 "/v1/models（${N} 个）"; else chk 1 "/v1/models（0 个）"; fi
else
  chk 1 "/v1/models（请求失败，检查 token）"
fi

[ -n "$MODEL" ] || MODEL="$(pick_model "$(list_models "$MODELS_BODY")")"
[ -n "$MODEL" ] || MODEL="claude-sonnet-4-6-1m"
echo "== model: $MODEL"

curl -fsS -m 15 "$BASE/v1/quota" >/dev/null 2>&1
chk $? "/v1/quota"

CHAT_BODY="$(curl -fsS -m 60 "$BASE/v1/chat/completions" \
  -H "Content-Type: application/json" -H "Authorization: Bearer $KEY" \
  -d "{\"model\":\"$MODEL\",\"messages\":[{\"role\":\"user\",\"content\":\"回复两个字:通\"}],\"stream\":false}" 2>/dev/null)" || CHAT_BODY=""
if [ -n "$CHAT_BODY" ]; then chk 0 "chat/completions"; else chk 1 "chat/completions"; fi

TMP_OUT="$(mktemp)"
curl -sS -m 60 -N "$BASE/v1/chat/completions" \
  -H "Content-Type: application/json" -H "Authorization: Bearer $KEY" \
  -d "{\"model\":\"$MODEL\",\"messages\":[{\"role\":\"user\",\"content\":\"数到三\"}],\"stream\":true}" \
  -o "$TMP_OUT" 2>/dev/null || true
grep -q '^data: ' "$TMP_OUT"; chk $? "SSE 流式"
rm -f "$TMP_OUT"

TOOL_BODY="$(curl -fsS -m 60 "$BASE/v1/chat/completions" \
  -H "Content-Type: application/json" -H "Authorization: Bearer $KEY" \
  -d "{\"model\":\"$MODEL\",\"messages\":[{\"role\":\"user\",\"content\":\"北京天气?\"}],\"stream\":false,\"tools\":[{\"type\":\"function\",\"function\":{\"name\":\"get_weather\",\"parameters\":{\"type\":\"object\",\"properties\":{\"city\":{\"type\":\"string\"}},\"required\":[\"city\"]}}}]}" 2>/dev/null)" || TOOL_BODY=""
case "$TOOL_BODY" in
  *'"tool_calls"'*) chk 0 "工具调用" ;;
  *) chk 1 "工具调用（模型未返回 tool_calls）" ;;
esac

echo "== $pass passed, $fail failed"
[ "$fail" = 0 ]
