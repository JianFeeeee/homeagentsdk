#!/usr/bin/env bash
# 同步 Lua SDK mock 的单一事实源到各副本。
#
# 事实源：sdk/lua/sdk.lua（本仓）
# 副本：
#   - tools/hmapdev/assets/sdk.lua   工具链内嵌回退（hmapdev init --lua 无 SDK 时用）
#   - example/luademo/sdk.lua        示例插件的离线测试副本
#   - <core>/internal/lua/sdk/sdk.lua 内核内嵌副本（本仓被 vendored 到
#     <core>/third_party/homeagent-sdk 时自动识别；独立 clone 时跳过）
#
# 为什么要有它：三份 sdk.lua 曾各自漂移，出现「mock 有、内核没有」的静默失配。
# 改 mock 只改事实源，然后跑这个脚本；内核仓另有契约测试比对。
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
SRC="$ROOT/sdk/lua/sdk.lua"

[ -f "$SRC" ] || { echo "error: canonical sdk.lua not found: $SRC" >&2; exit 1; }

copy() {
  local dst="$1"
  mkdir -p "$(dirname "$dst")"
  cp "$SRC" "$dst"
  echo "  synced -> $dst"
}

copy "$ROOT/tools/hmapdev/assets/sdk.lua"
copy "$ROOT/example/luademo/sdk.lua"

# 被内核仓 vendored 时（本仓位于 <core>/third_party/homeagent-sdk）同步内核副本。
CORE_COPY="$ROOT/../../internal/lua/sdk/sdk.lua"
if [ -d "$ROOT/../../internal" ]; then
  copy "$(cd "$(dirname "$CORE_COPY")" && pwd)/sdk.lua"
else
  echo "  note: core repo not vendored next to this checkout, skipping core copy"
fi

echo "Lua SDK mock synced."
