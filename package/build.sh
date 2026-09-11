#!/usr/bin/env bash
set -euo pipefail

PROJECT_ROOT="$(cd "$(dirname "$0")/.." && pwd)"
BUILD_DIR="${PROJECT_ROOT}/build"
VERSION="${VERSION:-$(git -C "$PROJECT_ROOT" describe --tags --dirty 2>/dev/null || echo "0.7.1")}"
GO="${GO:-$(command -v go 2>/dev/null || echo "/home/jianf/go1.26.5/go/bin/go")}"
GOCACHE="${GOCACHE:-}"
GOPATH="${GOPATH:-}"

TARGET="${1:-native}"
COMPONENT="${2:-all}"

case "$TARGET" in
  native)   GOOS="" GOARCH="" ;;
  linux/amd64)  GOOS=linux   GOARCH=amd64 ;;
  linux/arm64)  GOOS=linux   GOARCH=arm64 ;;
  darwin/amd64) GOOS=darwin  GOARCH=amd64 ;;
  darwin/arm64) GOOS=darwin  GOARCH=arm64 ;;
  windows/amd64) GOOS=windows GOARCH=amd64 ;;
  all)
    "$0" linux/amd64   "$COMPONENT"
    "$0" linux/arm64   "$COMPONENT"
    "$0" darwin/amd64  "$COMPONENT"
    "$0" darwin/arm64  "$COMPONENT"
    "$0" windows/amd64 "$COMPONENT"
    exit 0
    ;;
  *)
    echo "Unknown target: $TARGET"
    echo "Usage: $0 [native|linux/amd64|linux/arm64|darwin/amd64|darwin/arm64|windows/amd64|all] [all|plugindev|examples]"
    exit 1
esac

if [ -n "${GOOS:-}" ]; then
  SUFFIX="${GOOS}_${GOARCH}"
  export GOOS GOARCH
fi
export CGO_ENABLED=0
[ -n "$GOCACHE" ] && export GOCACHE
[ -n "$GOPATH" ] && export GOPATH

mkdir -p "$BUILD_DIR"

build_plugindev() {
  local src="tools/plugindev"
  local out="$BUILD_DIR/plugindev${SUFFIX:+_$SUFFIX}"
  if [ "$GOOS" = "windows" ]; then out="${out}.exe"; fi

  echo "[BUILD] plugindev ${GOOS:-linux}/${GOARCH:-amd64} → $out"
  cd "$PROJECT_ROOT/$src"
  "$GO" build -trimpath -ldflags "-X gitcode.com/JianFeeeee/homeagent-sdk/meta.Version=${VERSION}" \
    -o "$out" .
  echo "  OK ($(du -h "$out" | cut -f1))"
  cd "$PROJECT_ROOT"
}

# 示例插件产物随 SDK 一起发。
#
# 为什么必须发：插件二进制与内核是**协议绑定**的（ProtocolVersion + 统一共享
# 内存区魔数）。SDK 升版常伴随协议变化，只发工具链不发示例产物，使用者很可能
# 拿旧产物去装，表现是握手失败（魔数不匹配）——看起来像「插件坏了」而不是
# 「版本不配套」。
#
# 用刚构建出来的那把工具链（而非 PATH 里的），保证产物与本次发版同源。
build_examples() {
  local dev="$BUILD_DIR/plugindev${SUFFIX:+_$SUFFIX}"
  [ "$GOOS" = "windows" ] && dev="${dev}.exe"
  echo "[BUILD] example plugins ${GOOS:-linux}/${GOARCH:-amd64} → $BUILD_DIR/examples"
  PLUGINDEV="$dev" VERSION="$VERSION" bash "$PROJECT_ROOT/package/build-examples.sh" "$TARGET" "$BUILD_DIR/examples"
  echo "  OK"
}

case "$COMPONENT" in
  all)
    # 工具链必须先建完：示例用它来构建（同源保证协议一致）。
    build_plugindev
    build_examples
    ;;
  plugindev) build_plugindev ;;
  examples)  build_examples ;;
  *)
    echo "Unknown component: $COMPONENT"
    exit 1
    ;;
esac
