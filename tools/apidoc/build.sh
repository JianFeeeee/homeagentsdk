#!/usr/bin/env bash
# 生成并构建插件 SDK 文档站。
#
# 两步：
#   1. apidoc   —— 从 sdk/*.go 提取公开 API 面（签名/注释/分层）→ JSON
#   2. gensite  —— 把 JSON 渲染成 docs/api/*.md + 检索索引
# 然后 mkdocs 构建静态站。
#
# 为什么要脚本而不是手敲：API 参考是**生成物**，必须与源码同步，
# 否则文档会悄悄过时（这是文档站最常见的死法）。
#
# 用法：
#   tools/apidoc/build.sh          # 生成 + 构建
#   tools/apidoc/build.sh serve    # 生成 + 本地预览（热重载）
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
cd "$ROOT"

TMP_API="${TMPDIR:-/tmp}/homeagent-sdk-api.json"

echo "=== 1/3 提取 API 面 ==="
go run ./tools/apidoc -pkgdir ./sdk -out "$TMP_API"

echo "=== 2/3 渲染文档页与检索索引 ==="
go run ./tools/apidoc/gensite -api "$TMP_API" -out ./docs -examples ./example

echo "=== 3/3 构建静态站 ==="
if [ "${1:-}" = "serve" ]; then
  exec mkdocs serve
fi
mkdocs build --strict
echo
echo "完成。产物在 site_build/，本地预览：tools/apidoc/build.sh serve"
